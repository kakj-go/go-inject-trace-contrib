//go:build goinject

//inject:github.com/labstack/echo/v4
package echov4

import (
	"fmt"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type Echo struct {
}

func New() (e *Echo) {
	defer func() {
		// the official plugin fails when the result is not *Echo, which
		// cannot happen here because New always returns *Echo
		e.Use(swEchoMiddleware())
	}()
}

//inject:add
func swEchoMiddleware() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			request := c.Request()
			span, err := tracing.CreateEntrySpan(
				request.Method+":"+c.Path(),
				func(headerKey string) (string, error) {
					return request.Header.Get(headerKey), nil
				},
				tracing.WithLayer(tracing.SpanLayerHTTP),
				tracing.WithTag(tracing.TagHTTPMethod, request.Method),
				tracing.WithTag(tracing.TagURL, request.Host+request.URL.Path),
				tracing.WithComponent(5015))
			if err != nil {
				return err
			}
			// serve the request to the next middleware
			if err = next(c); err != nil {
				span.Error(err.Error())
				// invokes the registered HTTP error handler
				c.Error(err)
			}
			span.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", c.Response().Status))
			span.End()
			return nil
		}
	}
}
