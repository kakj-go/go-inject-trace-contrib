//go:build goinject

//inject:go.opentelemetry.io/otel/trace
package trace

// Ported from go.opentelemetry.io/otelc instrumentation/go.opentelemetry.io/otel/trace
// (hook.go + otelc.yaml rule hook_span_from_context): when the caller's
// context carries no valid span, fall back to the current goroutine's GLS
// span so implicit propagation resolves. The lookup crosses into the SDK's
// span stack through the zero-dependency otelgls registry because importing
// the SDK (or any package importing this one) from here would be a cycle.

import (
	"context"

	"github.com/kakj-go/go-inject-trace-contrib/otelc/otelgls"
)

func SpanFromContext(ctx context.Context) (otelcSpan Span) {
	defer func() {
		if otelcSpan.SpanContext().IsValid() {
			return
		}
		glsSpan := otelgls.SpanFromGLS()
		if glsSpan == nil {
			return
		}
		if s, ok := glsSpan.(Span); ok {
			otelcSpan = s
		}
	}()
	return
}
