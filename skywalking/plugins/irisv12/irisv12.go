//go:build goinject

//inject:github.com/kataras/iris/v12/core/router
package irisv12

import (
	"fmt"

	"github.com/kataras/iris/v12/context"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type routerHandler struct {
}

func (h *routerHandler) HandleRequest(ctx *context.Context) {
	swSpan := tracing.Span(nil)
	s, swErr := tracing.CreateEntrySpan(
		fmt.Sprintf("%s:%s", ctx.Method(), ctx.Path()), func(headerKey string) (string, error) {
			return ctx.GetHeader(headerKey), nil
		},
		tracing.WithLayer(tracing.SpanLayerHTTP),
		tracing.WithTag(tracing.TagHTTPMethod, ctx.Method()),
		tracing.WithTag(tracing.TagURL, ctx.Host()+ctx.Path()),
		tracing.WithComponent(5018))
	if swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		swSpan.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", ctx.ResponseWriter().StatusCode()))
		swSpan.End()
	}()
}
