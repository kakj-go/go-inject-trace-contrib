// Adapted from go.opentelemetry.io/otelc test/apps/httpclient: exercises the
// net/http client and server hooks together with cross-hook propagation. The
// /dorequest endpoint (self-driven from 127.0.0.1) issues one client request
// to this same server, producing server-entry -> client -> server-entry spans
// in one trace.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var port = flag.String("port", "8080", "The server port")

var ready atomic.Bool

func helloHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = io.WriteString(w, "Hello world")
}

func doRequestHandler(w http.ResponseWriter, r *http.Request) {
	url := fmt.Sprintf("http://127.0.0.1:%s/hello?name=world", *port)
	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(body)
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

	http.HandleFunc("/hello", helloHandler)
	http.HandleFunc("/dorequest", doRequestHandler)
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
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/dorequest", *port))
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
