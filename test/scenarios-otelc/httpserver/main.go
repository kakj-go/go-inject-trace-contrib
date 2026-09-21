// Adapted from go.opentelemetry.io/otelc test/apps/httpserver: a minimal HTTP
// server behind the default ServeMux. Self-driving: after startup the app
// requests its own traced endpoints from 127.0.0.1 (deterministic peer
// attributes), then reports /health ready for the A/B runner.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var port = flag.String("port", "8080", "The server port")

var ready atomic.Bool

func greetHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if err := json.NewEncoder(w).Encode("Hello " + name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if !ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func main() {
	flag.Parse()

	http.HandleFunc("/hello", greetHandler)
	http.HandleFunc("/health", healthHandler)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%s", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	go func() {
		if err := http.Serve(ln, nil); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	go func() {
		time.Sleep(300 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/hello?name=world", *port))
		if err != nil {
			log.Printf("self request failed: %v", err)
		} else {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}
