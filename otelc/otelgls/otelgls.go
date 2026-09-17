// Package otelgls is a zero-dependency registry that bridges goroutine-local
// span lookups between packages that cannot import each other: the GLS span
// stack lives in go.opentelemetry.io/otel/sdk/trace (where it is injected),
// while go.opentelemetry.io/otel/trace must reach it without importing the SDK.
// Values cross the boundary as interface{} and are type-asserted at the call
// site, so this package needs no OpenTelemetry imports at all.
//
// Ported from go.opentelemetry.io/otelc pkg/runtime (RegisterSpanFromGLSFunc /
// RegisterTraceAndSpanIDFunc), replacing the upstream go:linkname bridges with
// direct registration.
package otelgls

// Registrations happen once during package initialization of the injected
// sdk/trace code; reads happen on hot paths afterwards. Plain vars match the
// upstream semantics (set-once defaults overridden at init time).
var (
	spanFromGLSFunc    = defaultSpanFromGLS
	traceAndSpanIDFunc = defaultTraceAndSpanID
)

func defaultSpanFromGLS() interface{} { return nil }

func defaultTraceAndSpanID() (string, string) { return "", "" }

// SpanFromGLS returns the current goroutine's innermost live span as an
// interface{} holding a go.opentelemetry.io/otel/trace.Span, or nil.
func SpanFromGLS() interface{} { return spanFromGLSFunc() }

// RegisterSpanFromGLS installs the span lookup implementation. The returned
// value must be a trace.Span (API type) or nil.
func RegisterSpanFromGLS(f func() interface{}) { spanFromGLSFunc = f }

// TraceAndSpanID returns the current goroutine's trace and span IDs as strings.
func TraceAndSpanID() (string, string) { return traceAndSpanIDFunc() }

// RegisterTraceAndSpanID installs the trace/span ID lookup implementation.
func RegisterTraceAndSpanID(f func() (string, string)) { traceAndSpanIDFunc = f }
