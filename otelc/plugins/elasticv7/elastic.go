//go:build goinject

//inject:github.com/olivere/elastic/v7
package elastic

// Ported from go.opentelemetry.io/otelc instrumentation/github.com/olivere/elastic/v7
// (hook.go + semconv/client.go + otelc.yaml rule elastic_hook_perform_request).
// One CLIENT span per (*Client).PerformRequest, named "{operation} {index}"
// with the path parsed into semantic-convention attributes; HTTP client
// instrumentation is suppressed on the span context because the elastic
// client's inner net/http call would otherwise add a redundant child span.

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	hooklog "github.com/kakj-go/go-inject-trace-contrib/otelc/hooklog"
	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcESInstrumentationName = "go.opentelemetry.io/otelc/instrumentation/github.com/olivere/elastic/v7"

//inject:add
const otelcESInstrumentationKey = "ELASTIC"

//inject:add
var otelcESTracer trace.Tracer

//inject:add
var otelcESInitOnce sync.Once

//inject:add
func otelcESInit() {
	otelcESInitOnce.Do(func() {
		otelcESTracer = otel.GetTracerProvider().Tracer(
			otelcESInstrumentationName,
			trace.WithInstrumentationVersion(hooksupport.ModuleVersion()),
		)
		hooklog.Logger().Info("olivere/elastic v7 client instrumentation initialized")
	})
}

// ---- semconv ----

//inject:add
func otelcESSpanName(operation, index string) string {
	operation = strings.TrimSpace(operation)
	index = strings.TrimSpace(index)
	switch {
	case operation != "" && index != "":
		return operation + " " + index
	case operation != "":
		return operation
	case index != "":
		return index
	default:
		return "elasticsearch"
	}
}

//inject:add
func otelcESNormalizePath(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if u, err := url.PathUnescape(path); err == nil {
		path = u
	}
	if path == "" || path[0] != '/' {
		path = "/" + path
	}
	return path
}

//inject:add
func otelcESClientTraceAttrs(method, path, operation, index string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.DBSystemNameElasticsearch,
		semconv.NetworkTransportTCP,
	}
	if op := strings.TrimSpace(operation); op != "" {
		attrs = append(attrs, semconv.DBOperationName(op))
	}
	if idx := strings.TrimSpace(index); idx != "" {
		attrs = append(attrs, semconv.DBCollectionName(idx))
	}
	if m := strings.ToUpper(strings.TrimSpace(method)); m != "" {
		attrs = append(attrs, semconv.HTTPRequestMethodKey.String(m))
	}
	if p := otelcESNormalizePath(path); p != "" {
		attrs = append(attrs, semconv.URLPath(p))
	}
	return attrs
}

// ---- path parsing ----

//inject:add
var otelcESKnownActions = map[string]string{
	"search":          "search",
	"msearch":         "msearch",
	"bulk":            "bulk",
	"count":           "count",
	"update":          "update",
	"update_by_query": "update_by_query",
	"delete_by_query": "delete_by_query",
	"mget":            "mget",
	"refresh":         "refresh",
	"flush":           "flush",
	"mapping":         "mapping",
	"settings":        "settings",
	"aliases":         "aliases",
	"scroll":          "scroll",
	"explain":         "explain",
	"validate":        "validate",
}

//inject:add
var otelcESKnownClusterSubs = map[string]struct{}{
	"health":        {},
	"stats":         {},
	"info":          {},
	"settings":      {},
	"state":         {},
	"http":          {},
	"allocation":    {},
	"pending_tasks": {},
	"reroute":       {},
	"hot_threads":   {},
	"usage":         {},
	"nodes":         {},
	"plugins":       {},
	"ingest":        {},
	"indices":       {},
}

//inject:add
func otelcESParsePath(method, path string) (operation, index string) {
	path = otelcESUnescapePath(path)
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	segs := otelcESSplitPath(path)
	rest := segs
	if len(segs) > 0 && !strings.HasPrefix(segs[0], "_") {
		index = segs[0]
		rest = segs[1:]
	}
	return otelcESOperationFrom(method, rest), index
}

//inject:add
func otelcESUnescapePath(path string) string {
	if u, err := url.PathUnescape(path); err == nil {
		return u
	}
	return path
}

//inject:add
func otelcESSplitPath(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	raw := strings.Split(path, "/")
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

//inject:add
func otelcESOperationFrom(method string, segs []string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if op := otelcESLastKnownAction(segs); op != "" {
		return op
	}
	if otelcESHasDocType(segs) {
		return otelcESDocOperation(method, segs)
	}
	if clusterOp := otelcESClusterStyleOp(segs); clusterOp != "" {
		return clusterOp
	}
	return otelcESMethodFallback(method)
}

//inject:add
func otelcESLastKnownAction(segs []string) string {
	var found string
	for _, s := range segs {
		if !strings.HasPrefix(s, "_") {
			continue
		}
		if op, ok := otelcESKnownActions[strings.TrimPrefix(s, "_")]; ok {
			found = op
		}
	}
	return found
}

//inject:add
func otelcESHasDocType(segs []string) bool {
	for _, s := range segs {
		if s == "_doc" || s == "_create" {
			return true
		}
	}
	return false
}

//inject:add
func otelcESDocOperation(method string, segs []string) string {
	for _, s := range segs {
		if s == "_create" {
			return "create"
		}
	}
	switch method {
	case "GET", "HEAD":
		return "get"
	case "DELETE":
		return "delete"
	default:
		// PUT is a full index. POST is index with an auto-id.
		return "index"
	}
}

//inject:add
func otelcESClusterStyleOp(segs []string) string {
	if len(segs) == 0 || !strings.HasPrefix(segs[0], "_") {
		return ""
	}
	head := strings.TrimPrefix(segs[0], "_")
	if head == "" {
		return ""
	}
	if sub := otelcESKnownClusterSub(segs[1:]); sub != "" {
		return head + "." + sub
	}
	return head
}

//inject:add
func otelcESKnownClusterSub(segs []string) string {
	for _, s := range segs {
		name := strings.TrimPrefix(s, "_")
		if _, ok := otelcESKnownClusterSubs[name]; ok {
			return name
		}
	}
	return ""
}

//inject:add
func otelcESMethodFallback(method string) string {
	switch method {
	case "HEAD":
		return "exists"
	case "PUT":
		return "create"
	case "DELETE":
		return "delete"
	case "GET":
		return "get"
	case "POST":
		return "index"
	case "":
		return ""
	default:
		return strings.ToLower(method)
	}
}

//inject:add
func otelcESContains(list []int, v int) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// ---- template ----

func (c *Client) PerformRequest(ctx context.Context, opt PerformRequestOptions) (otelcResp *Response, otelcErr error) {
	var otelcSpan trace.Span
	var otelcIgnoreErrors []int
	if hooksupport.Instrumented(otelcESInstrumentationKey) {
		otelcESInit()

		if ctx == nil {
			ctx = context.Background()
		}

		operation, index := otelcESParsePath(opt.Method, opt.Path)

		var otelcCtx context.Context
		otelcCtx, otelcSpan = otelcESTracer.Start(ctx,
			otelcESSpanName(operation, index),
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(otelcESClientTraceAttrs(opt.Method, opt.Path, operation, index)...),
		)
		otelcCtx = hooksupport.SuppressHTTPClientInstrumentation(otelcCtx)
		ctx = otelcCtx
		otelcIgnoreErrors = opt.IgnoreErrors
	}
	if otelcSpan != nil {
		defer func() {
			span := otelcSpan
			defer span.End()

			status := 0
			if otelcResp != nil && otelcResp.StatusCode > 0 {
				status = otelcResp.StatusCode
			}

			if otelcErr != nil {
				span.RecordError(otelcErr)
				span.SetStatus(codes.Error, otelcErr.Error())
				var esErr *Error
				if errors.As(otelcErr, &esErr) && esErr.Status > 0 {
					status = esErr.Status
				}
			} else if status >= 400 && !otelcESContains(otelcIgnoreErrors, status) {
				span.SetStatus(codes.Error, "")
			}

			if status > 0 {
				span.SetAttributes(semconv.DBResponseStatusCode(strconv.Itoa(status)))
			}
		}()
	}
	return
}
