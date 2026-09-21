// Elasticsearch scenario: pings the cluster and runs one index + one search,
// producing PerformRequest spans with the parsed operation/index attributes.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/olivere/elastic/v7"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var (
	port = flag.String("port", "8080", "The HTTP health port")
	addr = flag.String("addr", "http://elasticsearch:9200", "Elasticsearch address")
)

var ready atomic.Bool

func main() {
	flag.Parse()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

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
		time.Sleep(500 * time.Millisecond)
		runElastic()
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}

func runElastic() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := elastic.NewClient(elastic.SetURL(*addr), elastic.SetSniff(false))
	if err != nil {
		log.Printf("client failed: %v", err)
		return
	}

	if _, err := client.Index().Index("otelc").Id("1").BodyJson(map[string]string{"name": "otelc"}).Do(ctx); err != nil {
		log.Printf("index failed: %v", err)
	}
	if _, err := client.Search().Index("otelc").Query(elastic.NewMatchQuery("name", "otelc")).Do(ctx); err != nil {
		log.Printf("search failed: %v", err)
	}
}
