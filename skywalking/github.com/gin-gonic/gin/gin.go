//inject:github.com/gin-gonic/gin/gin.go
package gin

import (
	"fmt"
	"github.com/gin-gonic/gin"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type Engine struct {
}

// same like https://github.com/apache/skywalking-go/blob/main/plugins/gin/intercepter.go
func (engine *Engine) handleHTTPRequest(c *gin.Context) {
	context := c
	s, err := tracing.CreateEntrySpan(
		fmt.Sprintf("%s:%s", context.Request.Method, context.Request.URL.Path), func(headerKey string) (string, error) {
			return context.Request.Header.Get(headerKey), nil
		},
		tracing.WithLayer(tracing.SpanLayerHTTP),
		tracing.WithTag(tracing.TagHTTPMethod, context.Request.Method),
		tracing.WithTag(tracing.TagURL, context.Request.Host+context.Request.URL.Path),
		tracing.WithComponent(5006))

	defer func() {
		if err == nil {
			span := s
			span.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", context.Writer.Status()))
			if len(context.Errors) > 0 {
				span.Error(context.Errors.String())
			}
			span.End()
		}
	}()
}
