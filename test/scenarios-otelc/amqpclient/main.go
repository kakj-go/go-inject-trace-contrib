// AMQP scenario: publishes two messages and consumes them with manual ack,
// exercising the send spans, the process spans, and the Ack settlement chain.
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

	amqp "github.com/rabbitmq/amqp091-go"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var (
	port = flag.String("port", "8080", "The HTTP health port")
	url  = flag.String("amqp-url", "amqp://admin:123456@amqp-server:5672/", "AMQP URL")
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
		runAMQP()
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}

func runAMQP() {
	conn, err := amqp.Dial(*url)
	if err != nil {
		log.Printf("dial failed: %v", err)
		return
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Printf("channel failed: %v", err)
		return
	}
	defer ch.Close()

	q, err := ch.QueueDeclare("orders", true, false, false, false, nil)
	if err != nil {
		log.Printf("declare failed: %v", err)
		return
	}

	for i := 0; i < 2; i++ {
		body := fmt.Sprintf("message-%d", i)
		if err := ch.PublishWithContext(
			context.Background(),
			"", q.Name, false, false,
			amqp.Publishing{ContentType: "text/plain", Body: []byte(body)},
		); err != nil {
			log.Printf("publish %d failed: %v", i, err)
		}
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Printf("consume failed: %v", err)
		return
	}
	for i := 0; i < 2; i++ {
		msg, ok := <-msgs
		if !ok {
			break
		}
		if err := msg.Ack(false); err != nil {
			log.Printf("ack failed: %v", err)
		}
	}
}
