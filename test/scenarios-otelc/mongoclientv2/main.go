// MongoDB v2 scenario: same command flow as the v1 scenario on the v2 driver
// (Connect has no context parameter there).
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

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var (
	port   = flag.String("port", "8080", "The HTTP health port")
	mongoU = flag.String("mongo", "mongodb://mongo-server:27017", "MongoDB URI")
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
		runMongo()
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}

func runMongo() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(*mongoU))
	if err != nil {
		log.Printf("connect failed: %v", err)
		return
	}
	defer client.Disconnect(ctx)

	if err := client.Ping(ctx, nil); err != nil {
		log.Printf("ping failed: %v", err)
		return
	}

	coll := client.Database("otelc").Collection("items")
	if _, err := coll.InsertOne(ctx, bson.M{"name": "otelc"}); err != nil {
		log.Printf("insert failed: %v", err)
	}
	res := coll.FindOne(ctx, bson.M{"name": "otelc"})
	var doc bson.M
	if err := res.Decode(&doc); err != nil {
		log.Printf("find failed: %v", err)
	}
}
