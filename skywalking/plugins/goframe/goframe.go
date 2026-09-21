//go:build goinject

//inject:github.com/gogf/gf/v2/net/ghttp
package goframe

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type Server struct {
}

func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	swSpan := tracing.Span(nil)
	s, swErr := tracing.CreateEntrySpan(fmt.Sprintf("%s:%s", r.Method, r.URL.Path), func(headerKey string) (string, error) {
		return r.Header.Get(headerKey), nil
	}, tracing.WithLayer(tracing.SpanLayerHTTP),
		tracing.WithTag(tracing.TagHTTPMethod, r.Method),
		tracing.WithTag(tracing.TagURL, r.Host+r.URL.Path),
		tracing.WithComponent(5022))
	if swErr == nil {
		swSpan = s
		if swGoframeCollectParams && r.URL != nil {
			s.Tag(tracing.TagHTTPParams, r.URL.RawQuery)
		}
		if len(swGoframeCollectHeaders) > 0 {
			swGoframeCollectRequestHeaders(s, r.Header)
		}
		w = &swGoframeWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if wrapped, ok := w.(*swGoframeWriterWrapper); ok {
			swSpan.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", wrapped.statusCode))
		}
		swSpan.End()
	}()
}

//inject:add
type swGoframeWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

//inject:add
var swGoframeCollectParams = func() bool {
	if v := os.Getenv("SW_AGENT_PLUGIN_CONFIG_GOFRAME_COLLECT_REQUEST_PARAMETERS"); v != "" {
		if b, e := strconv.ParseBool(v); e == nil {
			return b
		}
	}
	return false
}()

//inject:add
var swGoframeCollectHeaders = func() []string {
	if v := os.Getenv("SW_AGENT_PLUGIN_CONFIG_GOFRAME_COLLECT_REQUEST_HEADERS"); v != "" {
		return strings.Split(v, ",")
	}
	return nil
}()

//inject:add
func swGoframeParseHeaderThreshold() int {
	if v := os.Getenv("SW_AGENT_PLUGIN_CONFIG_GOFRAME_HEADER_LENGTH_THRESHOLD"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			return n
		}
	}
	return 2048
}

//inject:add
var swGoframeHeaderLengthThreshold = swGoframeParseHeaderThreshold()

//inject:add
func swGoframeCollectRequestHeaders(span tracing.Span, requestHeaders http.Header) {
	var headerTagValues []string
	for _, header := range swGoframeCollectHeaders {
		var headerValue = requestHeaders.Get(header)
		if headerValue != "" {
			headerTagValues = append(headerTagValues, header+"="+headerValue)
		}
	}
	if len(headerTagValues) == 0 {
		return
	}
	tagValue := strings.Join(headerTagValues, "\n")
	if len(tagValue) > swGoframeHeaderLengthThreshold {
		maxLen := swGoframeHeaderLengthThreshold
		tagValue = tagValue[:maxLen]
	}
	span.Tag(tracing.TagHTTPHeaders, tagValue)
}
