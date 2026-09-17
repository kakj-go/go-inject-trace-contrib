// Package hooksupport is the lightweight runtime API available to injected
// hook code: per-instrumentation enable/disable checks, module version lookup,
// HTTP-client suppression markers, and GLS-backed trace/span ID lookup.
//
// It deliberately has no dependency heavier than go.opentelemetry.io/otel/trace
// so that code injected into stdlib packages (net/http, log, log/slog, ...)
// can import it without creating dependency cycles — anything importing
// log/slog (the hook logger, see otelc/hooklog) or net/http (autoexport, OTLP
// exporters, see otelc/boot) must stay out.
//
// Ported from go.opentelemetry.io/otelc pkg/runtime (otel_setup.go,
// suppress.go, trace_context.go, version.go).
package hooksupport

import (
	"context"
	"os"
	runtime "runtime/debug"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel/trace"

	"github.com/kakj-go/go-inject-trace-contrib/otelc/otelgls"
)

// ModuleVersion extracts the version from the Go module system.
// Falls back to "dev" if version cannot be determined.
func ModuleVersion() string {
	if bi, ok := runtime.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

type configCache struct {
	sdkDisabled   string
	enabledRaw    string
	disabledRaw   string
	isSdkDisabled bool
	hasEnabled    bool
	enabledMap    map[string]struct{}
	disabledMap   map[string]struct{}
}

var instConfigCache atomic.Pointer[configCache]

// Instrumented checks if instrumentation is enabled via environment variables.
//
// Environment variables (following OTel JS pattern):
//   - OTEL_SDK_DISABLED: If set to the case-insensitive string "true", all instrumentations are disabled
//   - OTEL_GO_ENABLED_INSTRUMENTATIONS: comma-separated list of enabled instrumentations (e.g., "nethttp,grpc")
//   - OTEL_GO_DISABLED_INSTRUMENTATIONS: comma-separated list of disabled instrumentations (e.g., "nethttp")
//
// Logic:
//  1. If OTEL_SDK_DISABLED is "true", returns false
//  2. If OTEL_GO_ENABLED_INSTRUMENTATIONS is set, only those instrumentations are enabled
//  3. Then OTEL_GO_DISABLED_INSTRUMENTATIONS is applied to disable specific ones
//  4. If neither is set, all instrumentations are enabled by default
func Instrumented(instrumentationName string) bool {
	sdkDisabled := os.Getenv("OTEL_SDK_DISABLED")
	enabledList := os.Getenv("OTEL_GO_ENABLED_INSTRUMENTATIONS")
	disabledList := os.Getenv("OTEL_GO_DISABLED_INSTRUMENTATIONS")

	cached := instConfigCache.Load()
	if cached == nil || cached.sdkDisabled != sdkDisabled || cached.enabledRaw != enabledList ||
		cached.disabledRaw != disabledList {
		cached = buildConfigCache(sdkDisabled, enabledList, disabledList)
		instConfigCache.Store(cached)
	}

	if cached.isSdkDisabled {
		return false
	}

	name := strings.ToLower(instrumentationName)

	if cached.hasEnabled {
		if _, ok := cached.enabledMap[name]; !ok {
			return false
		}
	}

	if _, ok := cached.disabledMap[name]; ok {
		return false
	}

	return true
}

func buildConfigCache(sdkDisabled, enabledList, disabledList string) *configCache {
	c := &configCache{
		sdkDisabled:   sdkDisabled,
		enabledRaw:    enabledList,
		disabledRaw:   disabledList,
		isSdkDisabled: strings.EqualFold(sdkDisabled, "true"),
	}

	if parsed := parseInstrumentationList(enabledList); len(parsed) > 0 {
		c.hasEnabled = true
		c.enabledMap = make(map[string]struct{})
		for _, item := range parsed {
			c.enabledMap[item] = struct{}{}
		}
	}

	if disabledList != "" {
		c.disabledMap = make(map[string]struct{})
		for _, item := range parseInstrumentationList(disabledList) {
			c.disabledMap[item] = struct{}{}
		}
	}

	return c
}

// parseInstrumentationList parses a comma-separated list of instrumentation names.
func parseInstrumentationList(list string) []string {
	var result []string
	for item := range strings.SplitSeq(list, ",") {
		trimmed := strings.TrimSpace(strings.ToLower(item))
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

type contextKey struct{}

var suppressHTTPClientKey = contextKey{}

// SuppressHTTPClientInstrumentation returns a context that signals the net/http
// client hook to skip span creation. Use this from higher-level instrumentations
// (e.g., GenAI) that already create a more specific span.
func SuppressHTTPClientInstrumentation(ctx context.Context) context.Context {
	return context.WithValue(ctx, suppressHTTPClientKey, true)
}

// IsHTTPClientInstrumentationSuppressed reports whether the context carries the
// suppression flag set by SuppressHTTPClientInstrumentation.
func IsHTTPClientInstrumentationSuppressed(ctx context.Context) bool {
	v, _ := ctx.Value(suppressHTTPClientKey).(bool)
	return v
}

// GetTraceAndSpanID returns the current goroutine's trace and span IDs from
// the GLS span stack, registered by the injected sdk/trace code.
func GetTraceAndSpanID() (string, string) {
	return otelgls.TraceAndSpanID()
}

// GetSpanFromGLS returns the current goroutine's innermost live span, or nil.
func GetSpanFromGLS() trace.Span {
	if s, ok := otelgls.SpanFromGLS().(trace.Span); ok {
		return s
	}
	return nil
}
