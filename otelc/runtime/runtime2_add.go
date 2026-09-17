//go:build goinject

//inject:runtime/runtime2.go
package runtime

// Ported from go.opentelemetry.io/otelc instrumentation/runtime (runtime_gls.go,
// rules add_gls_field + gls_linker): adds goroutine-local storage fields to the
// runtime g struct and exported accessors reading getg().m.curg.

type g struct {
	//inject:add
	otel_trace_context interface{}
	//inject:add
	otel_baggage_container interface{}
}

//inject:add
func GetTraceContextFromGLS() interface{} {
	return getg().m.curg.otel_trace_context
}

//inject:add
func GetBaggageContainerFromGLS() interface{} {
	return getg().m.curg.otel_baggage_container
}

//inject:add
func SetTraceContextToGLS(traceContext interface{}) {
	getg().m.curg.otel_trace_context = traceContext
}

//inject:add
func SetBaggageContainerToGLS(baggageContainer interface{}) {
	getg().m.curg.otel_baggage_container = baggageContainer
}

//inject:add
type OtelContextCloner interface {
	Clone() interface{}
}

//inject:add
func propagateOtelContext(context interface{}) interface{} {
	if context == nil {
		return nil
	}
	if cloner, ok := context.(OtelContextCloner); ok {
		return cloner.Clone()
	}
	return context
}
