//go:build goinject

//inject:github.com/gofiber/fiber/v2
package fiber

import (
	"fmt"

	"github.com/valyala/fasthttp"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type App struct {
}

func (app *App) handler(ctx *fasthttp.RequestCtx) {
	swSpan := tracing.Span(nil)
	s, swErr := tracing.CreateEntrySpan(fmt.Sprintf("%s:%s", string(ctx.Method()), string(ctx.Request.URI().Path())),
		func(headerKey string) (string, error) {
			return string(ctx.Request.Header.Peek(headerKey)), nil
		},
		tracing.WithLayer(tracing.SpanLayerHTTP),
		tracing.WithTag(tracing.TagHTTPMethod, string(ctx.Method())),
		tracing.WithTag(tracing.TagURL, string(ctx.Request.URI().Host())+string(ctx.Request.URI().Path())),
		tracing.WithComponent(5021))
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
