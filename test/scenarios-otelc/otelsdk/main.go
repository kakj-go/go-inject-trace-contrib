// Adapted from go.opentelemetry.io/otelc test/apps/otelsdk: verifies GLS-based
// span propagation through the SDK hooks (push on span creation, pop on End,
// SpanFromContext fallback, cross-goroutine clone) with manual spans only.
// net/http instrumentation is disabled on both sides via
// OTEL_GO_DISABLED_INSTRUMENTATIONS=nethttp so this scenario exercises the M0
// skeleton (runtime GLS + sdk/trace stack + otel API hooks + boot) before the
// library plugins land.
//
// Unlike the upstream one-shot app, this variant stays alive: it drives the
// same endpoint sequence once at startup, then reports /health as ready so
// the A/B runner can capture telemetry before shutdown.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	_ "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var port = flag.String("port", "8080", "The server port")

var (
	workerTasks = make(chan func())
	startWorker sync.Once
	spanToEnd   = make(chan trace.Span)
	spanEnded   = make(chan struct{})
	ready       atomic.Bool
)

// submit runs task on a single long-lived worker goroutine, created lazily on
// first use. The goroutine inherits a GLS clone from whoever calls submit
// first (the /otel handler, while its span is active), and that clone is then
// reused across requests. This models real-world worker pools, where a
// goroutine outlives the request whose GLS it inherited.
func submit(task func()) {
	startWorker.Do(func() {
		go func() {
			for task := range workerTasks {
				task()
			}
		}()
	})
	workerTasks <- task
}

// otelHandler verifies cross-goroutine propagation: the worker's GLS clone
// snapshots the active span, so SpanFromContext must return it in the task.
// The span is ended on a separate goroutine (see main) so it is only flagged
// ended, never popped from the worker's clone.
func otelHandler(w http.ResponseWriter, r *http.Request) {
	_, span := otel.Tracer("handler").Start(context.Background(), "cross-goroutine-span")
	done := make(chan struct{})
	submit(func() {
		defer close(done)
		span := trace.SpanFromContext(context.Background())
		sc := span.SpanContext()

		if sc.IsValid() {
			fmt.Printf("OTEL_SDK_TEST: span valid, traceID=%s spanID=%s\n",
				sc.TraceID().String(), sc.SpanID().String())
		} else {
			fmt.Println("OTEL_SDK_TEST: span invalid")
		}
	})
	<-done
	spanToEnd <- span
	<-spanEnded

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// workerHandler reuses the worker after the /otel span ended. Its GLS clone
// still holds that span, so this checks SpanFromContext skips ended entries
// (stale span=false) and worker-span starts as root, not as its child.
func workerHandler(w http.ResponseWriter, r *http.Request) {
	done := make(chan struct{})
	submit(func() {
		defer close(done)
		stale := trace.SpanFromContext(context.Background()).SpanContext().IsValid()
		fmt.Printf("OTEL_SDK_WORKER: stale span=%t\n", stale)
		_, span := otel.Tracer("worker").Start(context.Background(), "worker-span")
		span.End()
	})
	<-done

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// compactHandler verifies ended spans below the active span don't consume the
// GLS limit. With OTEL_GLS_MAX_SPANS=3 the stack [server-span, lower, top] is
// full; ending lower on another goroutine only flags it, so replacement-span
// hits the limit and must compact the stack to be admitted (admitted=true).
func compactHandler(w http.ResponseWriter, r *http.Request) {
	_, lower := otel.Tracer("compact").Start(context.Background(), "lower-span")
	_, top := otel.Tracer("compact").Start(context.Background(), "top-span")
	done := make(chan struct{})
	go func() {
		lower.End()
		close(done)
	}()
	<-done

	_, replacement := otel.Tracer("compact").Start(context.Background(), "replacement-span")
	active := trace.SpanFromContext(context.Background()).SpanContext()
	fmt.Printf("OTEL_SDK_COMPACT: admitted=%t\n", active.SpanID() == replacement.SpanContext().SpanID())
	replacement.End()
	top.End()

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if !ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func main() {
	flag.Parse()
	addr := fmt.Sprintf(":%s", *port)

	http.HandleFunc("/otel", otelHandler)       // cross-goroutine propagation via GLS clone
	http.HandleFunc("/worker", workerHandler)   // ended span is invisible on a reused goroutine
	http.HandleFunc("/compact", compactHandler) // ended spans don't count against OTEL_GLS_MAX_SPANS
	http.HandleFunc("/health", healthHandler)

	// End the /otel span on a goroutine that never held it in GLS, so removal
	// happens only through the shared ended flag (see otelHandler).
	go func() {
		span := <-spanToEnd
		span.End()
		close(spanEnded)
	}()

	ln, err := net.Listen("tcp", addr)
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
		for _, path := range []string{"otel", "worker", "compact"} {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/%s", *port, path))
			if err != nil {
				log.Printf("request to %s failed: %v", path, err)
				continue
			}
			resp.Body.Close()
		}
		// Give simple-processor span export a moment before reporting ready.
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}
