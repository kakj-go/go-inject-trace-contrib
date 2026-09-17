//go:build goinject

//inject:go.uber.org/zap/zapcore
package zapcore

// Ported from go.opentelemetry.io/otelc instrumentation/go.uber.org/zap
// (zap_hook.go + otelc.yaml rule hook_zapcore_checked_entry_write): appends
// trace_id/span_id zapcore fields to every checked-entry write while a span
// is active. The variadic fields parameter is replaced before the original
// Write body consumes it.

import (
	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcZapTraceIDKey = "trace_id"

//inject:add
const otelcZapSpanIDKey = "span_id"

func (ce *CheckedEntry) Write(fields ...Field) {
	if hooksupport.Instrumented("logs/zap") && ce != nil {
		traceID, spanID := hooksupport.GetTraceAndSpanID()
		if traceID != "" {
			// Copy so append cannot write into spare capacity on a reused caller slice.
			otelcOut := make([]Field, len(fields), len(fields)+2)
			copy(otelcOut, fields)
			otelcOut = append(otelcOut, Field{Key: otelcZapTraceIDKey, Type: StringType, String: traceID})
			if spanID != "" {
				otelcOut = append(otelcOut, Field{Key: otelcZapSpanIDKey, Type: StringType, String: spanID})
			}
			fields = otelcOut
		}
	}
}
