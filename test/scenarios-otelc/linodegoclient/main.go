// linodego scenario: embeds a mock Linode API (mirrors the upstream smoke
// test) and self-drives the catalog/account/volumes/instances calls,
// producing "linodego.<Method>" operation spans over "{METHOD} {endpoint}"
// doRequest spans, plus the linodego.client.operation.duration metric.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/linode/linodego/v2"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var (
	port        = flag.String("port", "8080", "The mock+health server port")
	resourceID  = flag.Int("id", 123, "Resource ID used by get operations")
)

var ready atomic.Bool

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func paginated(data any) map[string]any {
	n := 1
	switch v := data.(type) {
	case []map[string]any:
		n = len(v)
	}
	return map[string]any{
		"data":    data,
		"page":    1,
		"pages":   1,
		"results": n,
	}
}

func main() {
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v4/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v4")
		if path == "" {
			path = "/"
		}
		switch {
		case path == "/regions":
			writeJSON(w, 200, paginated([]map[string]any{
				{
					"id":          "us-east",
					"label":       "Newark, NJ",
					"country":     "us",
					"status":      "ok",
					"capabilities": []string{"Linodes", "NodeBalancers", "Block Storage"},
					"resolvers": map[string]string{
						"ipv4": "1.1.1.1",
						"ipv6": "2600::",
					},
					"site_type": "core",
				},
			}))
		case path == "/account":
			writeJSON(w, 200, map[string]any{
				"first_name": "Test",
				"last_name":  "User",
				"email":      "test@example.com",
				"company":    "OTel",
				"balance":    0,
				"euuid":      "E123",
			})
		case path == "/volumes":
			writeJSON(w, 200, paginated([]map[string]any{
				{"id": float64(*resourceID), "label": "otelc-vol", "status": "active", "size": 10, "region": "us-east"},
			}))
		case strings.HasPrefix(path, "/volumes/"):
			writeJSON(w, 200, map[string]any{
				"id": float64(*resourceID), "label": "otelc-vol", "status": "active", "size": 10, "region": "us-east",
			})
		case path == "/linode/instances":
			writeJSON(w, 200, paginated([]map[string]any{
				{"id": float64(*resourceID), "label": "otelc-instance", "region": "us-east", "status": "running"},
			}))
		case strings.HasPrefix(path, "/linode/instances/"):
			writeJSON(w, 200, map[string]any{
				"id": float64(*resourceID), "label": "otelc-instance", "region": "us-east", "status": "running",
			})
		default:
			writeJSON(w, 404, map[string]any{"errors": []map[string]any{{"reason": "not found"}}})
		}
	})

	ln, err := net.Listen("tcp", fmt.Sprintf(":%s", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	go func() {
		if err := http.Serve(ln, mux); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	go func() {
		time.Sleep(500 * time.Millisecond)
		runClient(fmt.Sprintf("http://127.0.0.1:%s", *port))
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}

func runClient(baseURL string) {
	client, err := linodego.NewClient(http.DefaultClient)
	if err != nil {
		log.Printf("failed to create client: %v", err)
		return
	}
	client.SetToken("test-token")
	client.UseCache(false)
	if _, err := client.UseURL(baseURL + "/v4"); err != nil {
		log.Printf("failed to set API URL: %v", err)
		return
	}

	ctx := context.Background()

	if _, err := client.ListRegions(ctx, nil); err != nil {
		log.Printf("ListRegions failed: %v", err)
	}
	if _, err := client.GetAccount(ctx); err != nil {
		log.Printf("GetAccount failed: %v", err)
	}
	if _, err := client.ListVolumes(ctx, nil); err != nil {
		log.Printf("ListVolumes failed: %v", err)
	}
	if _, err := client.GetVolume(ctx, *resourceID); err != nil {
		log.Printf("GetVolume failed: %v", err)
	}
	if _, err := client.ListInstances(ctx, nil); err != nil {
		log.Printf("ListInstances failed: %v", err)
	}
	if _, err := client.GetInstance(ctx, *resourceID); err != nil {
		log.Printf("GetInstance failed: %v", err)
	}
}
