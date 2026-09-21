// Redis scenario: connects to the redis container, runs SET/GET plus one
// pipeline, producing one CLIENT span per command and a pipeline span.
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

	"github.com/redis/go-redis/v9"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var (
	port    = flag.String("port", "8080", "The HTTP health port")
	redisAd = flag.String("redis", "redis-server:6379", "Redis address")
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
		time.Sleep(300 * time.Millisecond)
		runRedis()
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}

func runRedis() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{Addr: *redisAd})
	defer rdb.Close()

	if err := rdb.Set(ctx, "otelc-key", "otelc-value", 0).Err(); err != nil {
		log.Printf("set failed: %v", err)
	}
	if err := rdb.Get(ctx, "otelc-key").Err(); err != nil {
		log.Printf("get failed: %v", err)
	}

	pipe := rdb.Pipeline()
	pipe.Set(ctx, "otelc-p1", "v1", 0)
	pipe.Get(ctx, "otelc-p1")
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		log.Printf("pipeline failed: %v", err)
	}
}
