// AWS SDK v2 scenario against an in-process mock DynamoDB endpoint (mirrors
// the upstream integration test): LoadDefaultConfig gets the otelaws
// middlewares appended by the injected hook, and one ListTables call produces
// the dynamodb.ListTables CLIENT span plus nested HTTP/RPC spans.
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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var port = flag.String("port", "8080", "The HTTP health port")

var ready atomic.Bool

func main() {
	flag.Parse()

	// Mock DynamoDB endpoint on a second port: ListTables returns an empty
	// table list, which is all the SDK needs for a successful round trip.
	ln, err := net.Listen("tcp", fmt.Sprintf(":%s", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{"TableNames":[]}`))
	})
	go func() {
		if err := http.Serve(ln, mux); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	go func() {
		time.Sleep(500 * time.Millisecond)
		runAWS(fmt.Sprintf("http://127.0.0.1:%s", *port))
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}

func runAWS(endpoint string) {
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		log.Printf("load config failed: %v", err)
		return
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	out, err := client.ListTables(ctx, &dynamodb.ListTablesInput{})
	if err != nil {
		log.Printf("list tables failed: %v", err)
		return
	}
	log.Printf("tables: %d", len(out.TableNames))
}
