//go:build goinject

//inject:github.com/gin-gonic/gin
package gin

// Ported from go.opentelemetry.io/otelc instrumentation/github.com/gin-gonic/gin
// (context_hook.go + gin.go + otelc.yaml rule gin_context_next_hook). Gin does
// not create its own span: it enriches the net/http server span with the
// matched route ("METHOD /route" + http.route) on the first Next call, and
// records accumulated c.Error() values when the middleware chain unwinds.
//
// Span lookups go through trace.SpanFromContext(c.Request.Context()), which
// the oteltrace rule makes GLS-aware. The span name helper comes from the
// net/http package additions made by the nethttp plugin (OtelcHTTPServerSpanName).

import (
	"net/http"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

// otelcGinRouteSetKey is stored on the gin.Context to prevent repeated span
// updates when multiple middleware layers call c.Next(). The keys are
// reserved by this instrumentation; user middleware must not set or read them.
//
//inject:add
const otelcGinRouteSetKey = "otel.gin.route.set"

//inject:add
const otelcGinNextDepthKey = "otel.gin.next.depth"

type Context struct{}

func (c *Context) Next() {
	if !hooksupport.Instrumented("GIN") || c == nil || c.Request == nil {
		return
	}

	if d, exists := c.Get(otelcGinNextDepthKey); exists {
		if depth, ok := d.(int); ok {
			c.Set(otelcGinNextDepthKey, depth+1)
		} else {
			c.Set(otelcGinNextDepthKey, 1)
		}
	} else {
		c.Set(otelcGinNextDepthKey, 1)
	}

	route := c.FullPath()
	if route != "" {
		if _, already := c.Get(otelcGinRouteSetKey); !already {
			if span := trace.SpanFromContext(c.Request.Context()); span.IsRecording() {
				// Set the gate only after confirming we have a recording span
				// to update; otherwise a non-recording first call would burn
				// the gate and block a later recording span on the same
				// request from being enriched.
				c.Set(otelcGinRouteSetKey, struct{}{})

				span.SetName(http.OtelcHTTPServerSpanName(c.Request.Method, route))
				span.SetAttributes(semconv.HTTPRouteKey.String(route))
			}
		}
	}

	defer func() {
		d, _ := c.Get(otelcGinNextDepthKey)
		depth, _ := d.(int)
		depth--
		c.Set(otelcGinNextDepthKey, depth)

		if depth > 0 {
			return
		}

		if len(c.Errors) == 0 {
			return
		}

		if span := trace.SpanFromContext(c.Request.Context()); span.IsRecording() {
			span.SetStatus(codes.Error, c.Errors.String())
			for _, e := range c.Errors {
				span.RecordError(e.Err)
			}
		}
	}()
}
