//go:build goinject

//inject:github.com/fasthttp/router
package fasthttp

import (
	"fmt"

	"github.com/valyala/fasthttp"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type Router struct {
}

func (r *Router) Handler(ctx *fasthttp.RequestCtx) {
	swSpan := tracing.Span(nil)
	s, swErr := tracing.CreateEntrySpan(fmt.Sprintf("%s:%s", string(ctx.Method()), ctx.URI().String()),
		func(headerKey string) (string, error) {
			return string(ctx.Request.Header.Peek(headerKey)), nil
		}, tracing.WithLayer(tracing.SpanLayerHTTP),
		tracing.WithTag(tracing.TagHTTPMethod, string(ctx.Method())),
		tracing.WithTag(tracing.TagURL, ctx.URI().String()),
		tracing.WithComponent(5020))
	if swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if ctx.Response.StatusCode() >= 400 {
			swSpan.Error()
		}
		swSpan.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", ctx.Response.StatusCode()))
		swSpan.End()
	}()
}
