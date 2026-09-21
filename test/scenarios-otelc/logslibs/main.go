// Log-bridge scenario: each logging library (log/slog, stdlib log, logrus,
// zap) emits one record inside an active manual span; the injected bridges
// append trace_id/span_id to the library output. Validation compares the
// app's stdout (normalized) between the official otelc build and the
// go-inject build, plus the manual span via mockcol.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"go.uber.org/zap"

	"go.opentelemetry.io/otel"
	_ "go.opentelemetry.io/otel/sdk/trace"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var port = flag.String("port", "8080", "The server port")

var ready atomic.Bool

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if !ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func main() {
	flag.Parse()

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

		// One manual span; each library logs inside it so every bridge has a
		// live trace context to inject.
		tracer := otel.Tracer("logslibs")
		_, span := tracer.Start(context.Background(), "log-span")
		func() {
			defer span.End()

			slog.Info("hello from slog", "key", "value")
			log.Println("hello from log")
			logrus.WithField("key", "value").Info("hello from logrus")
			zap.L().Info("hello from zap", zap.String("key", "value"))
		}()

		// And one record outside any span: bridges must stay silent.
		slog.Info("outside span slog")
		log.Println("outside span log")

		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}
