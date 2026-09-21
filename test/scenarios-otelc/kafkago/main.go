// kafka-go scenario: writes two messages to a topic on the broker and reads
// them back through a consumer group, producing producer spans and consumer
// spans linked by the trace context in message headers.
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

	"github.com/segmentio/kafka-go"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var (
	port    = flag.String("port", "8080", "The HTTP health port")
	brokers = flag.String("brokers", "kafka-server:9092", "Kafka broker address")
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
		runKafka()
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}

func runKafka() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := kafka.DialLeader(ctx, "tcp", *brokers, "otelc-topic", 0)
	if err != nil {
		log.Printf("dial leader failed: %v", err)
		return
	}
	if err := conn.CreateTopics(kafka.TopicConfig{Topic: "otelc-topic", NumPartitions: 1, ReplicationFactor: 1}); err != nil {
		log.Printf("create topic failed (may exist): %v", err)
	}
	conn.Close()

	w := &kafka.Writer{
		Addr:     kafka.TCP(*brokers),
		Topic:    "otelc-topic",
		Balancer: &kafka.LeastBytes{},
	}
	if err := w.WriteMessages(ctx,
		kafka.Message{Key: []byte("key-1"), Value: []byte("value-1")},
		kafka.Message{Key: []byte("key-2"), Value: []byte("value-2")},
	); err != nil {
		log.Printf("write failed: %v", err)
	}
	w.Close()

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{*brokers},
		GroupID: "otelc-group",
		Topic:   "otelc-topic",
	})
	for i := 0; i < 2; i++ {
		if _, err := r.ReadMessage(ctx); err != nil {
			log.Printf("read failed: %v", err)
			break
		}
	}
	r.Close()
}
