//go:build goinject

//inject:net/http
package http

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type Transport struct {
}

func (t *Transport) RoundTrip(req *Request) (resp *Response, err error) {
	swSpan := tracing.Span(nil)
	swHost := req.Host
	if swHost == "" && req.URL != nil {
		swHost = req.URL.Host
	}
	s, swErr := tracing.CreateExitSpan(fmt.Sprintf("%s:%s", req.Method, req.URL.Path), swHost, func(headerKey, headerValue string) error {
		req.Header.Add(headerKey, headerValue)
		return nil
	}, tracing.WithLayer(tracing.SpanLayerHTTP),
		tracing.WithTag(tracing.TagHTTPMethod, req.Method),
		tracing.WithTag(tracing.TagURL, swHost+req.URL.Path),
		tracing.WithComponent(5005))
	if swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if resp != nil {
			swSpan.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", resp.StatusCode))
			if resp.StatusCode >= 400 {
				swSpan.ErrorOccured()
			}
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

type ServeMux struct {
}

func (mux *ServeMux) ServeHTTP(w ResponseWriter, r *Request) {
	swSpan := tracing.Span(nil)
	s, swErr := tracing.CreateEntrySpan(fmt.Sprintf("%s:%s", r.Method, r.URL.Path), func(headerKey string) (string, error) {
		return r.Header.Get(headerKey), nil
	}, tracing.WithLayer(tracing.SpanLayerHTTP),
		tracing.WithTag(tracing.TagHTTPMethod, r.Method),
		tracing.WithTag(tracing.TagURL, r.Host+r.URL.Path),
		tracing.WithComponent(5004))
	if swErr == nil {
		swSpan = s
		if swHTTPCollectParams && r.URL != nil {
			s.Tag(tracing.TagHTTPParams, r.URL.RawQuery)
		}
	}
	w = &swResponseWriterWrapper{ResponseWriter: w, statusCode: StatusOK}
	defer func() {
		if swSpan == nil {
			return
		}
		if wrapped, ok := w.(*swResponseWriterWrapper); ok {
			swSpan.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", wrapped.statusCode))
			if wrapped.statusCode >= 400 {
				swSpan.ErrorOccured()
			}
		}
		swSpan.End()
	}()
}

//inject:add
type swResponseWriterWrapper struct {
	ResponseWriter
	statusCode int
}

//inject:add
func (w *swResponseWriterWrapper) WriteHeader(statusCode int) {
	// cache the status code
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

//inject:add
func (w *swResponseWriterWrapper) Hijack() (rwc net.Conn, buf *bufio.ReadWriter, err error) {
	if h, ok := w.ResponseWriter.(Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("responseWriter does not implement Hijacker")
}

//inject:add
var swHTTPCollectParams = func() bool {
	if v := os.Getenv("SW_AGENT_PLUGIN_CONFIG_HTTP_SERVER_COLLECT_PARAMETERS"); v != "" {
		if b, e := strconv.ParseBool(v); e == nil {
			return b
		}
	}
	return false
}()
