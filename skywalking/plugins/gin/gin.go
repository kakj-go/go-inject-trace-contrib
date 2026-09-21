//go:build goinject

//inject:github.com/gin-gonic/gin
package gin

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type Context struct {
}

func (c *Context) Next() {
	swSpan := tracing.Span(nil)
	if c.index < 0 {
		swFullPath := c.FullPath()
		if swFullPath == "" {
			swFullPath = c.Request.URL.Path
		}
		s, err := tracing.CreateEntrySpan(
			fmt.Sprintf("%s:%s", c.Request.Method, swFullPath), func(headerKey string) (string, error) {
				return c.Request.Header.Get(headerKey), nil
			},
			tracing.WithLayer(tracing.SpanLayerHTTP),
			tracing.WithTag(tracing.TagHTTPMethod, c.Request.Method),
			tracing.WithTag(tracing.TagURL, c.Request.Host+c.Request.URL.Path),
			tracing.WithComponent(5006))
		if err == nil {
			swSpan = s
			if len(swGinCollectHeaders) > 0 {
				swGinCollectRequestHeaders(swSpan, c.Request.Header)
			}
		}
	}
	defer func() {
		if swSpan == nil {
			return
		}
		swSpan.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", c.Writer.Status()))
		if c.Writer.Status() >= 400 {
			swSpan.ErrorOccured()
		}
		if len(c.Errors) > 0 {
			swSpan.Error(c.Errors.String())
		}
		swSpan.End()
	}()
}

//inject:add
var swGinCollectHeaders = func() []string {
	if v := os.Getenv("SW_AGENT_PLUGIN_CONFIG_GIN_COLLECT_REQUEST_HEADERS"); v != "" {
		return strings.Split(v, ",")
	}
	return nil
}()

//inject:add
func swGinParseHeaderThreshold() int {
	if v := os.Getenv("SW_AGENT_PLUGIN_CONFIG_GIN_HEADER_LENGTH_THRESHOLD"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			return n
		}
	}
	return 2048
}

//inject:add
var swGinHeaderLengthThreshold = swGinParseHeaderThreshold()

//inject:add
func swGinCollectRequestHeaders(span tracing.Span, requestHeaders http.Header) {
	var headerTagValues []string
	for _, header := range swGinCollectHeaders {
		headerValue := requestHeaders.Get(header)
		if headerValue != "" {
			headerTagValues = append(headerTagValues, header+"="+headerValue)
		}
	}
	if len(headerTagValues) == 0 {
		return
	}
	tagValue := strings.Join(headerTagValues, "\n")
	if len(tagValue) > swGinHeaderLengthThreshold {
		tagValue = tagValue[:swGinHeaderLengthThreshold]
	}
	span.Tag(tracing.TagHTTPHeaders, tagValue)
}
