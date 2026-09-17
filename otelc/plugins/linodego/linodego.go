//go:build goinject

//inject:github.com/linode/linodego/v2
package linodego

// Ported from go.opentelemetry.io/otelc instrumentation/github.com/linode/linodego/v2
// (hook.go + public_method.go + semconv/client.go + otelc.yaml rules
// linodego_hook_dorequest and public_methods.otelc.yaml). Two span layers:
// doRequest CLIENT spans ("{METHOD} {endpoint}") for each HTTP call, and
// "linodego.<Method>" CLIENT spans for public API methods (with the
// linodego.client.operation.duration metric).
//
// Upstream generates per-method hook wrappers for every public method
// (public_methods_gen.go, for its linkname dispatch); go-inject matches
// templates by declaration, so each covered method is a plain template. The
// helpers below cover both layers; the templates at the bottom cover the
// methods exercised by the scenario suite — extend with more method
// templates as needed (each is three lines following beforeAPICall's
// shape: start span, replace ctx param, defer afterAPICall).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"

	hooklog "github.com/kakj-go/go-inject-trace-contrib/otelc/hooklog"
	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcLinodeInstrumentationName = "go.opentelemetry.io/otelc/instrumentation/github.com/linode/linodego/v2"

//inject:add
const otelcLinodeInstrumentationKey = "LINODEGO"

//inject:add
const otelcLinodeDefaultServerAddress = "api.linode.com"

//inject:add
const otelcLinodeMetricOperationDuration = "linodego.client.operation.duration"

//inject:add
var otelcLinodeTracer trace.Tracer

//inject:add
var otelcLinodeDuration metric.Float64Histogram

//inject:add
var otelcLinodeInitOnce sync.Once

//inject:add
func otelcLinodeInit() {
	otelcLinodeInitOnce.Do(func() {
		version := hooksupport.ModuleVersion()
		otelcLinodeTracer = otel.GetTracerProvider().Tracer(
			otelcLinodeInstrumentationName,
			trace.WithInstrumentationVersion(version),
		)
		meter := otel.GetMeterProvider().Meter(
			otelcLinodeInstrumentationName,
			metric.WithInstrumentationVersion(version),
		)
		var err error
		otelcLinodeDuration, err = meter.Float64Histogram(
			otelcLinodeMetricOperationDuration,
			metric.WithDescription("Duration of linodego public Client API operations."),
			metric.WithUnit("s"),
			metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10),
		)
		if err != nil {
			otelcLinodeDuration = nil
		}
		hooklog.Logger().Info("linodego instrumentation initialized")
	})
}

// ---- semconv helpers ----

//inject:add
func otelcLinodeSpanName(method, endpoint string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	endpoint = strings.TrimSpace(endpoint)
	if method == "" {
		method = "HTTP"
	}
	if endpoint == "" {
		return method
	}
	return method + " " + endpoint
}

//inject:add
func otelcLinodeOperationSpanName(operation string) string {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		return "linodego.operation"
	}
	return "linodego." + operation
}

//inject:add
func otelcLinodeRequestTraceAttrs(method, endpoint string) []attribute.KeyValue {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "HTTP"
	}
	endpoint = strings.TrimSpace(endpoint)

	attrs := make([]attribute.KeyValue, 0, 3)
	attrs = append(attrs, semconv.HTTPRequestMethodKey.String(method))
	if endpoint != "" {
		path := endpoint
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		attrs = append(attrs, semconv.URLPath(path))
	}
	attrs = append(attrs, semconv.ServerAddress(otelcLinodeDefaultServerAddress))
	return attrs
}

//inject:add
func otelcLinodeOperationTraceAttrs(operation string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.ServerAddress(otelcLinodeDefaultServerAddress),
	}
	if op := strings.TrimSpace(operation); op != "" {
		attrs = append(attrs, semconv.CodeFunctionName(op))
	}
	return attrs
}

//inject:add
func otelcLinodeStatusCodeFromError(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	type statusCoder interface {
		StatusCode() int
	}
	var sc statusCoder
	if errors.As(err, &sc) {
		code := sc.StatusCode()
		// linodego uses synthetic codes for non-HTTP errors; only treat
		// real HTTP status codes as response codes.
		if code >= 100 && code < 600 {
			return code, true
		}
	}
	return 0, false
}

//inject:add
func otelcLinodeErrorTraceAttrs(err error) []attribute.KeyValue {
	if err == nil {
		return nil
	}
	attrs := make([]attribute.KeyValue, 0, 2)
	if code, ok := otelcLinodeStatusCodeFromError(err); ok {
		attrs = append(attrs, semconv.HTTPResponseStatusCode(code))
		if code >= 400 {
			attrs = append(attrs, semconv.ErrorTypeKey.String(fmt.Sprintf("%d", code)))
		}
	}
	return attrs
}

//inject:add
func otelcLinodeHTTPClientStatus(code int) (codes.Code, string) {
	if code < 100 || code >= 600 {
		return codes.Error, fmt.Sprintf("Invalid HTTP status code %d", code)
	}
	if code >= 400 {
		return codes.Error, ""
	}
	return codes.Unset, ""
}

//inject:add
func otelcLinodeMetricAttrs(operation string, statusCode int) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 3)
	attrs = append(attrs, semconv.ServerAddress(otelcLinodeDefaultServerAddress))
	if op := strings.TrimSpace(operation); op != "" {
		attrs = append(attrs, semconv.CodeFunctionName(op))
	}
	if statusCode > 0 {
		attrs = append(attrs, semconv.HTTPResponseStatusCode(statusCode))
	}
	return attrs
}

// otelcLinodeFinishSpanWithError records error details on a span (shared by
// doRequest and public methods); returns the HTTP status code (0 when none).
//
//inject:add
func otelcLinodeFinishSpanWithError(span trace.Span, err error) int {
	if err == nil {
		return 0
	}
	span.RecordError(err)
	if code, ok := otelcLinodeStatusCodeFromError(err); ok {
		span.SetAttributes(otelcLinodeErrorTraceAttrs(err)...)
		if sc, desc := otelcLinodeHTTPClientStatus(code); sc != codes.Unset {
			span.SetStatus(sc, desc)
		}
		return code
	}
	span.SetStatus(codes.Error, err.Error())
	return 0
}

// otelcLinodeRecordDuration records the public-method duration metric.
//
//inject:add
func otelcLinodeRecordDuration(ctx context.Context, seconds float64, operation string, statusCode int) {
	if otelcLinodeDuration == nil {
		return
	}
	otelcLinodeDuration.Record(ctx, seconds, metric.WithAttributes(otelcLinodeMetricAttrs(operation, statusCode)...))
}

// ---- doRequest template ----

func (c *Client) doRequest(ctx context.Context, method, endpoint string, params requestParams, paginationMutator *func(*http.Request) error) (otelcErr error) {
	var otelcSpan trace.Span
	if hooksupport.Instrumented(otelcLinodeInstrumentationKey) {
		otelcLinodeInit()

		attrs := otelcLinodeRequestTraceAttrs(method, endpoint)
		spanName := otelcLinodeSpanName(method, endpoint)

		var otelcCtx context.Context
		otelcCtx, otelcSpan = otelcLinodeTracer.Start(ctx,
			spanName,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attrs...),
		)
		// Propagate the span context into the original request so nested
		// instrumentations (e.g. net/http) and public-method parents link
		// correctly.
		ctx = otelcCtx
	}
	if otelcSpan != nil {
		defer func() {
			otelcSpan.End()
			if otelcErr != nil {
				otelcSpan.RecordError(otelcErr)
				if code, ok := otelcLinodeStatusCodeFromError(otelcErr); ok {
					otelcSpan.SetAttributes(otelcLinodeErrorTraceAttrs(otelcErr)...)
					if sc, desc := otelcLinodeHTTPClientStatus(code); sc != codes.Unset {
						otelcSpan.SetStatus(sc, desc)
					}
				} else {
					otelcSpan.SetStatus(codes.Error, otelcErr.Error())
				}
			}
		}()
	}
	return
}

// ---- public method templates ----
// Each covers one (*Client) method: start the "linodego.<Method>" span,
// replace the ctx parameter, and end + record the duration metric on return.
// New methods follow the same three-line shape.

//inject:add
func otelcLinodeStartAPICall(ctx context.Context, operation string) (context.Context, trace.Span, time.Time) {
	otelcLinodeInit()
	attrs := otelcLinodeOperationTraceAttrs(operation)
	otelcCtx, span := otelcLinodeTracer.Start(ctx,
		otelcLinodeOperationSpanName(operation),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
	return otelcCtx, span, time.Now()
}

//inject:add
func otelcLinodeEndAPICall(span trace.Span, start time.Time, operation string, ctx context.Context, err error) {
	if span == nil {
		return
	}
	defer span.End()
	statusCode := otelcLinodeFinishSpanWithError(span, err)
	if !start.IsZero() {
		otelcLinodeRecordDuration(ctx, time.Since(start).Seconds(), operation, statusCode)
	}
}

func (c *Client) GetAccount(ctx context.Context) (otelcObj *Account, otelcErr error) {
	if hooksupport.Instrumented(otelcLinodeInstrumentationKey) {
		otelcCtx, otelcSpan, otelcStart := otelcLinodeStartAPICall(ctx, "GetAccount")
		ctx = otelcCtx
		defer func() { otelcLinodeEndAPICall(otelcSpan, otelcStart, "GetAccount", otelcCtx, otelcErr) }()
	}
	return
}

func (c *Client) GetInstance(ctx context.Context, linodeID int) (otelcObj *Instance, otelcErr error) {
	if hooksupport.Instrumented(otelcLinodeInstrumentationKey) {
		otelcCtx, otelcSpan, otelcStart := otelcLinodeStartAPICall(ctx, "GetInstance")
		ctx = otelcCtx
		defer func() { otelcLinodeEndAPICall(otelcSpan, otelcStart, "GetInstance", otelcCtx, otelcErr) }()
	}
	return
}

func (c *Client) ListRegions(ctx context.Context, opts *ListOptions) (otelcObjs []Region, otelcErr error) {
	if hooksupport.Instrumented(otelcLinodeInstrumentationKey) {
		otelcCtx, otelcSpan, otelcStart := otelcLinodeStartAPICall(ctx, "ListRegions")
		ctx = otelcCtx
		defer func() { otelcLinodeEndAPICall(otelcSpan, otelcStart, "ListRegions", otelcCtx, otelcErr) }()
	}
	return
}

func (c *Client) ListVolumes(ctx context.Context, opts *ListOptions) (otelcObjs []Volume, otelcErr error) {
	if hooksupport.Instrumented(otelcLinodeInstrumentationKey) {
		otelcCtx, otelcSpan, otelcStart := otelcLinodeStartAPICall(ctx, "ListVolumes")
		ctx = otelcCtx
		defer func() { otelcLinodeEndAPICall(otelcSpan, otelcStart, "ListVolumes", otelcCtx, otelcErr) }()
	}
	return
}

func (c *Client) GetVolume(ctx context.Context, volumeID int) (otelcObj *Volume, otelcErr error) {
	if hooksupport.Instrumented(otelcLinodeInstrumentationKey) {
		otelcCtx, otelcSpan, otelcStart := otelcLinodeStartAPICall(ctx, "GetVolume")
		ctx = otelcCtx
		defer func() { otelcLinodeEndAPICall(otelcSpan, otelcStart, "GetVolume", otelcCtx, otelcErr) }()
	}
	return
}

func (c *Client) ListInstances(ctx context.Context, opts *ListOptions) (otelcObjs []Instance, otelcErr error) {
	if hooksupport.Instrumented(otelcLinodeInstrumentationKey) {
		otelcCtx, otelcSpan, otelcStart := otelcLinodeStartAPICall(ctx, "ListInstances")
		ctx = otelcCtx
		defer func() { otelcLinodeEndAPICall(otelcSpan, otelcStart, "ListInstances", otelcCtx, otelcErr) }()
	}
	return
}
