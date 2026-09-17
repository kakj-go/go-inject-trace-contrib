//go:build goinject

//inject:github.com/sirupsen/logrus
package logrus

// Ported from go.opentelemetry.io/otelc instrumentation/github.com/sirupsen/logrus
// (logrus_hook.go + otelc.yaml rules hook_logrus_new/_with_field/_set_formatter):
// registers a logrus hook that copies trace_id/span_id from the GLS span stack
// into entry.Data. The lazy map + mutex initialization is load-bearing: logrus
// runs `var std = New()` during its own package init, so the injected after-hook
// can fire before this package's var initializers — nil maps must never be
// assigned through.

import (
	"sync"

	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcLogrusTraceIDKey = "trace_id"

//inject:add
const otelcLogrusSpanIDKey = "span_id"

//inject:add
var otelcLogrusMu sync.Mutex

//inject:add
var otelcLogrusHookMap map[*Logger]bool

//inject:add
var otelcLogrusFieldMap map[*Logger]bool

//inject:add
var otelcLogrusFormatterInit bool

//inject:add
type otelcTraceHook struct{}

//inject:add
func (h *otelcTraceHook) Levels() []Level {
	return AllLevels
}

//inject:add
func (h *otelcTraceHook) Fire(entry *Entry) error {
	if !hooksupport.Instrumented("logs/logrus") {
		return nil
	}

	traceID, spanID := hooksupport.GetTraceAndSpanID()
	if traceID != "" {
		entry.Data[otelcLogrusTraceIDKey] = traceID
	}
	if spanID != "" {
		entry.Data[otelcLogrusSpanIDKey] = spanID
	}
	return nil
}

//inject:add
func otelcLogrusAddHookLocked(logger *Logger) {
	if otelcLogrusHookMap == nil {
		otelcLogrusHookMap = make(map[*Logger]bool)
	}
	if otelcLogrusHookMap[logger] {
		return
	}

	logger.AddHook(&otelcTraceHook{})
	otelcLogrusHookMap[logger] = true
}

func New() (otelcLogger *Logger) {
	defer func() {
		if !hooksupport.Instrumented("logs/logrus") || otelcLogger == nil {
			return
		}

		otelcLogrusMu.Lock()
		defer otelcLogrusMu.Unlock()

		otelcLogrusAddHookLocked(otelcLogger)
	}()
	return
}

func (entry *Entry) WithField(otelcKey string, otelcValue interface{}) (otelcEntry *Entry) {
	defer func() {
		if !hooksupport.Instrumented("logs/logrus") || otelcEntry == nil || otelcEntry.Logger == nil {
			return
		}

		otelcLogrusMu.Lock()
		defer otelcLogrusMu.Unlock()

		if otelcLogrusFieldMap == nil {
			otelcLogrusFieldMap = make(map[*Logger]bool)
		}
		if otelcLogrusFieldMap[otelcEntry.Logger] {
			return
		}

		if otelcEntry.Logger.Hooks == nil {
			otelcEntry.Logger.Hooks = make(LevelHooks)
		}

		otelcEntry.Logger.AddHook(&otelcTraceHook{})
		otelcLogrusFieldMap[otelcEntry.Logger] = true
	}()
	return
}

func (logger *Logger) SetFormatter(formatter Formatter) {
	if hooksupport.Instrumented("logs/logrus") {
		otelcLogrusMu.Lock()
		if !otelcLogrusFormatterInit {
			std := StandardLogger()
			otelcLogrusAddHookLocked(std)
			otelcLogrusFormatterInit = true
		}
		otelcLogrusMu.Unlock()
	}
}
