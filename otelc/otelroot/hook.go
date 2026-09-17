//go:build goinject

//inject:go.opentelemetry.io/otel
package otel

// Ported from go.opentelemetry.io/otelc instrumentation/go.opentelemetry.io/otel
// (hook.go + otelc.yaml rule hook_set_tracer_provider): after the first call,
// subsequent user calls to SetTracerProvider are skipped so the provider
// installed by the boot initializer survives. The early return in the template
// body suppresses the original function body for repeated calls.

import (
	"sync/atomic"

	"go.opentelemetry.io/otel/trace"
)

//inject:add
var otelcTracerProviderSet atomic.Bool

func SetTracerProvider(p trace.TracerProvider) {
	if otelcTracerProviderSet.Swap(true) {
		return
	}
}
