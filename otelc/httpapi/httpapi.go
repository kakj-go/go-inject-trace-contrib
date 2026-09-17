// Package httpapi is the bridge target for code injected into net/http.
//
// The patched net/http package must not import go.opentelemetry.io/otel or
// go.opentelemetry.io/otel/propagation: both transitively import net/http
// (otel -> internal/global -> propagation -> net/http), so a plain import
// from injected code closes a cycle. The nethttp rule declares body-less
// //go:linkname functions pointing at the exported helpers below instead;
// go-inject's link bridges validate the signatures and add this package to
// the link closure without an import edge.
package httpapi

import (
	"context"
	"net/http"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/kakj-go/go-inject-trace-contrib/otelc/hooklog"
	"github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

const instrumentationName = "go.opentelemetry.io/otelc/instrumentation/net/http"

var (
	otelcOnce sync.Once
	// Upstream keeps separate lazy initializers (and startup log lines) for
	// the client and server hooks; mirror both so stdout matches 1:1.
	otelcClientOnce         sync.Once
	otelcServerOnce         sync.Once
	otelcTracerInstance     trace.Tracer
	otelcPropagatorInstance propagation.TextMapPropagator
)

func otelcInit() {
	otelcOnce.Do(func() {
		otelcTracerInstance = otel.GetTracerProvider().Tracer(
			instrumentationName,
			trace.WithInstrumentationVersion(hooksupport.ModuleVersion()),
		)
		otelcPropagatorInstance = otel.GetTextMapPropagator()
	})
}

// TracerForClient mirrors the client hook's lazy init, including its startup
// log line.
func TracerForClient() trace.Tracer {
	otelcClientOnce.Do(func() {
		otelcInit()
		hooklog.Logger().Info("HTTP client instrumentation initialized")
	})
	return otelcTracerInstance
}

// TracerForServer mirrors the server hook's lazy init, including its startup
// log line.
func TracerForServer() trace.Tracer {
	otelcServerOnce.Do(func() {
		otelcInit()
		hooklog.Logger().Info("HTTP server instrumentation initialized")
	})
	return otelcTracerInstance
}

// Propagator returns the global text-map propagator.
func Propagator() propagation.TextMapPropagator {
	otelcInit()
	return otelcPropagatorInstance
}

// Extract reads trace context values from the header map into ctx.
func Extract(ctx context.Context, h http.Header) context.Context {
	return Propagator().Extract(ctx, propagation.HeaderCarrier(h))
}

// Inject sets trace context values from ctx into the header map.
func Inject(ctx context.Context, h http.Header) {
	Propagator().Inject(ctx, propagation.HeaderCarrier(h))
}
