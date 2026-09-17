// Package boot initializes the full OpenTelemetry SDK at application start.
// It is imported only from the application's main package (via otelc/bootinit)
// because its dependency closure — autoexport and the OTLP exporters — pulls
// in heavy packages such as net/http that injected hook code must not import.
//
// Ported from go.opentelemetry.io/otelc pkg/runtime (setup.go, otel_setup.go).
package boot

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	logglobal "go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/kakj-go/go-inject-trace-contrib/otelc/hooklog"
	"github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

const (
	// Default export intervals and batch sizes
	defaultTraceBatchTimeout = time.Second
	defaultTraceBatchSize    = 512
)

var (
	meterProvider         *sdkmetric.MeterProvider
	tracerProvider        *sdktrace.TracerProvider
	loggerProvider        *sdklog.LoggerProvider
	registerSignalHandler sync.Once
)

// Config holds configuration for OpenTelemetry setup
type Config struct {
	InstrumentationName    string
	InstrumentationVersion string
}

// Initialize sets up OpenTelemetry with defensive error handling
func Initialize(cfg Config) {
	// Defensive: ensure instrumentation initialization never crashes user application
	defer func() {
		if rec := recover(); rec != nil {
			hooklog.Logger().Error("panic during OpenTelemetry initialization", "panic", rec)
		}
	}()

	setupOpenTelemetry(cfg)
	setupSignalHandler()
}

// SetupOTelSDK initializes the OpenTelemetry SDK.
//
// The SDK automatically configures exporters based on environment variables
// following the OpenTelemetry specification:
//
// SDK Configuration:
//   - OTEL_SDK_DISABLED: If set to the case-insensitive string "true", the SDK
//     is disabled entirely and no providers are installed.
//
// Exporter Configuration (applies independently to each signal):
//   - OTEL_TRACES_EXPORTER: otlp (default), console, none
//   - OTEL_METRICS_EXPORTER: otlp (default), console, prometheus, none
//   - OTEL_LOGS_EXPORTER: otlp (default), console, none
//   - OTEL_EXPORTER_OTLP_ENDPOINT (+ per-signal overrides), default http://localhost:4318
//   - OTEL_EXPORTER_OTLP_PROTOCOL: grpc, http/protobuf (default), http/json
//
// Other Configuration:
//   - OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES: resource attributes
//   - OTEL_PROPAGATORS: default "tracecontext,baggage"
//   - OTEL_LOG_LEVEL: hook logger level (debug, info, warn, error)
func SetupOTelSDK() {
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") {
		hooklog.Logger().Info("OpenTelemetry SDK disabled via OTEL_SDK_DISABLED=true, skipping initialization")
		return
	}

	Initialize(Config{
		InstrumentationName:    "go.opentelemetry.io/otelc",
		InstrumentationVersion: hooksupport.ModuleVersion(),
	})
}

// setupOpenTelemetry initializes the OpenTelemetry SDK with OTLP exporters
func setupOpenTelemetry(cfg Config) {
	defer func() {
		if rec := recover(); rec != nil {
			hooklog.Logger().Error("panic during OpenTelemetry setup", "panic", rec)
		}
	}()

	// The default handler writes to the stdlib logger on stderr, which bypasses OTEL_LOG_LEVEL and does not match
	// the structured output the rest of the runtime emits.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		hooklog.Logger().Error("OpenTelemetry SDK error", "error", err)
	}))

	ctx := context.Background()

	res, err := resource.New(ctx, resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithHost(),
		resource.WithFromEnv())
	if err != nil {
		hooklog.Logger().Warn("failed to create resource", "error", err)
		res = resource.Default()
	}

	if err := setupTraceProvider(ctx, res); err != nil {
		hooklog.Logger().Warn("failed to setup trace provider", "error", err)
	}

	if err := setupMeterProvider(ctx, res); err != nil {
		hooklog.Logger().Warn("failed to setup meter provider", "error", err)
	}

	if err := setupLoggerProvider(ctx, res); err != nil {
		hooklog.Logger().Warn("failed to setup logger provider", "error", err)
	}

	// Use autoprop to select propagators from OTEL_PROPAGATORS (tracecontext,
	// baggage, b3, b3multi, jaeger, xray, ottrace, none). Defaults to W3C
	// Trace Context + Baggage when the variable is not set.
	otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())

	hooklog.Logger().Info("OpenTelemetry initialized",
		"instrumentation_name", cfg.InstrumentationName,
		"instrumentation_version", cfg.InstrumentationVersion)
}

// setupTraceProvider creates and configures the trace provider
func setupTraceProvider(ctx context.Context, res *resource.Resource) error {
	traceExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return err
	}

	// OTEL_TRACES_EXPORTER=none: skip building a provider/processor entirely
	// rather than running a batch processor that will never export anything.
	if autoexport.IsNoneSpanExporter(traceExporter) {
		hooklog.Logger().Debug("trace exporter disabled via OTEL_TRACES_EXPORTER=none, skipping trace provider setup")
		return nil
	}

	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(newSpanProcessor(traceExporter)),
	)

	otel.SetTracerProvider(tracerProvider)

	hooklog.Logger().Info("trace provider initialized with auto-export")
	return nil
}

func newSpanProcessor(traceExporter sdktrace.SpanExporter) sdktrace.SpanProcessor {
	if useSimpleSpanProcessor() {
		hooklog.Logger().Debug("using SimpleSpanProcessor for immediate span export")
		return sdktrace.NewSimpleSpanProcessor(traceExporter)
	}

	return sdktrace.NewBatchSpanProcessor(traceExporter,
		sdktrace.WithBatchTimeout(defaultTraceBatchTimeout),
		sdktrace.WithMaxExportBatchSize(defaultTraceBatchSize),
	)
}

func useSimpleSpanProcessor() bool {
	return strings.EqualFold(os.Getenv("OTEL_GO_SIMPLE_SPAN_PROCESSOR"), "true")
}

// setupMeterProvider creates and configures the meter provider
func setupMeterProvider(ctx context.Context, res *resource.Resource) error {
	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return err
	}

	if autoexport.IsNoneMetricReader(metricReader) {
		hooklog.Logger().Debug("metric exporter disabled via OTEL_METRICS_EXPORTER=none, skipping meter provider setup")
		return nil
	}

	meterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(metricReader),
	)

	otel.SetMeterProvider(meterProvider)

	hooklog.Logger().Info("meter provider initialized with auto-export")
	return nil
}

// setupLoggerProvider creates and configures the logger provider
func setupLoggerProvider(ctx context.Context, res *resource.Resource) error {
	logExporter, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return err
	}

	if autoexport.IsNoneLogExporter(logExporter) {
		hooklog.Logger().Debug("log exporter disabled via OTEL_LOGS_EXPORTER=none, skipping logger provider setup")
		return nil
	}

	loggerProvider = sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
	)

	logglobal.SetLoggerProvider(loggerProvider)

	hooklog.Logger().Info("logger provider initialized with auto-export")
	return nil
}

// Shutdown gracefully shuts down the OpenTelemetry SDK
func Shutdown(ctx context.Context) error {
	var err error

	if tracerProvider != nil {
		if shutdownErr := tracerProvider.Shutdown(ctx); shutdownErr != nil {
			hooklog.Logger().Error("failed to shutdown tracer provider", "error", shutdownErr)
			err = shutdownErr
		}
	}

	if meterProvider != nil {
		if shutdownErr := meterProvider.Shutdown(ctx); shutdownErr != nil {
			hooklog.Logger().Error("failed to shutdown meter provider", "error", shutdownErr)
			err = shutdownErr
		}
	}

	if loggerProvider != nil {
		if shutdownErr := loggerProvider.Shutdown(ctx); shutdownErr != nil {
			hooklog.Logger().Error("failed to shutdown logger provider", "error", shutdownErr)
			err = shutdownErr
		}
	}

	return err
}

// StartRuntimeMetrics enables Go runtime metrics collection.
// This follows the same enable/disable pattern as other instrumentations via
// OTEL_GO_ENABLED_INSTRUMENTATIONS and OTEL_GO_DISABLED_INSTRUMENTATIONS.
func StartRuntimeMetrics() {
	if !hooksupport.Instrumented("runtimemetrics") {
		hooklog.Logger().Debug("runtime metrics disabled via environment variable")
		return
	}

	mp := otel.GetMeterProvider()

	if err := runtime.Start(runtime.WithMeterProvider(mp)); err != nil {
		hooklog.Logger().Warn("failed to start runtime metrics", "error", err)
		return
	}

	hooklog.Logger().Info("runtime metrics enabled")
}

// setupSignalHandler flushes the OTel SDK on SIGINT/SIGTERM so buffered telemetry
// survives shutdown, then steps aside. It never exits or re-raises the signal:
// the application owns its own exit path and exit code.
func setupSignalHandler() {
	registerSignalHandler.Do(func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		go handleShutdownSignal(sigCh)
	})
}

func handleShutdownSignal(sigCh chan os.Signal) {
	sig := <-sigCh

	// Stop listening now so a repeated signal isn't swallowed by our buffered
	// channel but reaches the app or the default disposition, keeping the
	// "press again to force quit" behavior.
	signal.Stop(sigCh)

	hooklog.Logger().Info("received signal, flushing telemetry", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := Shutdown(ctx); err != nil {
		hooklog.Logger().Error("error flushing telemetry during shutdown", "error", err)
	} else {
		hooklog.Logger().Info("OpenTelemetry SDK shutdown completed successfully")
	}
}
