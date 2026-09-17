//go:build goinject && go1.24 && !go1.28

//inject:runtime/proc.go
//inject:id otelc-gls-propagation
package runtime

// Ported from go.opentelemetry.io/otelc instrumentation/runtime (rule
// goroutine_propagate): every go statement propagates the parent goroutine's
// GLS trace context and baggage into the new goroutine, cloning the span
// stack snapshot at fork time. The final top-level return is dropped by
// go-inject; the original newproc1 body runs and fills the named result,
// which the deferred closure then reads.
func newproc1(fn *funcval, callergp *g, callerpc uintptr, parked bool, waitreason waitReason) (newg *g) {
	defer func() {
		newg.otel_trace_context = propagateOtelContext(callergp.otel_trace_context)
		newg.otel_baggage_container = propagateOtelContext(callergp.otel_baggage_container)
	}()
	return
}
