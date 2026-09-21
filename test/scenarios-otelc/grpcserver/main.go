// gRPC scenario using grpc's own health service (no codegen): the app serves
// health on :50051 plus an HTTP /health readiness endpoint, then self-drives
// one unary health.Check call through grpc.NewClient — producing a client span
// and a server span in one trace.
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

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/health"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var (
	port   = flag.String("port", "8080", "The HTTP health port")
	rpcadr = flag.String("rpcaddr", "127.0.0.1:50051", "The gRPC listen address")
)

var ready atomic.Bool

func main() {
	flag.Parse()

	lis, err := net.Listen("tcp", *rpcadr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	hs := health.NewServer()
	hs.SetServingStatus("grpcserver", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(srv, hs)
	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	go func() {
		if err := http.ListenAndServe(fmt.Sprintf(":%s", *port), nil); err != nil {
			log.Fatalf("failed to serve http: %v", err)
		}
	}()

	go func() {
		time.Sleep(300 * time.Millisecond)
		conn, err := grpc.NewClient(*rpcadr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("dial failed: %v", err)
		} else {
			client := healthpb.NewHealthClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{Service: "grpcserver"})
			if err != nil {
				log.Printf("check failed: %v", err)
			} else {
				log.Printf("check status: %v", resp.Status)
			}
			cancel()
			_ = conn.Close()
		}
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}
