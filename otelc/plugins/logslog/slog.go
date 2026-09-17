//go:build goinject

//inject:log/slog
package slog

// Ported from go.opentelemetry.io/otelc instrumentation/log/slog (slog_hook.go
// + otelc.yaml rule hook_slog_new_record): injects trace_id/span_id as
// structured attributes on every log record created while a span is active on
// the current goroutine (via the GLS span stack).

import (
	"time"

	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcSlogTraceIDKey = "trace_id"

//inject:add
const otelcSlogSpanIDKey = "span_id"

func NewRecord(p time.Time, l Level, msg string, pc uintptr) (otelcR Record) {
	defer func() {
		if !hooksupport.Instrumented("logs/slog") {
			return
		}

		traceID, spanID := hooksupport.GetTraceAndSpanID()
		if traceID == "" {
			return
		}

		var attrs []Attr
		attrs = append(attrs, String(otelcSlogTraceIDKey, traceID))
		if spanID != "" {
			attrs = append(attrs, String(otelcSlogSpanIDKey, spanID))
		}

		otelcR.AddAttrs(attrs...)
	}()
	return
}
