//go:build goinject

//inject:log/log.go
package log

// Ported from go.opentelemetry.io/otelc instrumentation/log (log_hook.go +
// otelc.yaml rule hook_log_output): appends " trace_id=... span_id=..." to
// each output line (before the trailing newline) when a span is active. The
// appendOutput callback parameter is replaced with a wrapping closure; the
// original output body then calls the wrapper.

import (
	"strings"

	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcLogTraceIDKey = "trace_id"

//inject:add
const otelcLogSpanIDKey = "span_id"

func (l *Logger) output(pc uintptr, calldepth int, appendOutput func([]byte) []byte) (otelcErr error) {
	if hooksupport.Instrumented("logs/log") {
		otelcInner := appendOutput
		appendOutput = func(b []byte) []byte {
			b = otelcInner(b)
			if len(b) == 0 {
				return b
			}

			if strings.Contains(string(b), otelcLogTraceIDKey) {
				return b
			}

			traceID, spanID := hooksupport.GetTraceAndSpanID()
			if traceID == "" {
				return b
			}

			var sb strings.Builder
			sb.WriteString(" ")
			sb.WriteString(otelcLogTraceIDKey)
			sb.WriteString("=")
			sb.WriteString(traceID)

			if spanID != "" {
				sb.WriteString(" ")
				sb.WriteString(otelcLogSpanIDKey)
				sb.WriteString("=")
				sb.WriteString(spanID)
			}

			otelcSuffix := sb.String()

			idx := len(b)
			for idx > 0 && (b[idx-1] == '\n' || b[idx-1] == '\r') {
				idx--
			}

			b = append(b[:idx], append([]byte(otelcSuffix), b[idx:]...)...)
			return b
		}
	}
	return
}
