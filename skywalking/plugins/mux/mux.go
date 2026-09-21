//go:build goinject

//inject:github.com/gorilla/mux
package mux

import (
	"fmt"
	"net/http"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type Router struct {
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	swSpan := tracing.Span(nil)
	s, swErr := tracing.CreateEntrySpan(fmt.Sprintf("%s:%s", req.Method, req.RequestURI), func(headerKey string) (string, error) {
		return req.Header.Get(headerKey), nil
	}, tracing.WithLayer(tracing.SpanLayerHTTP),
		tracing.WithComponent(5017),
		tracing.WithTag(tracing.TagHTTPMethod, req.Method),
		tracing.WithTag(tracing.TagURL, req.Host+req.URL.Path))
	if swErr == nil {
		swSpan = s
		w = swNewResponseWriter(w)
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if wrapped, ok := w.(*swWriterWrapper); ok {
			swSpan.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", wrapped.statusCode))
		}
		if wrapped, ok := w.(*swWriterWrapperWithHijacker); ok {
			swSpan.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", wrapped.writer.statusCode))
		}
		swSpan.End()
	}()
}

func (r *Router) Match(req *http.Request, match *RouteMatch) (matched bool) {
	defer func() {
		// only process with matched route
		if !matched {
			return
		}
		if match == nil || match.Route == nil || req == nil {
			return
		}

		span := tracing.ActiveSpan()
		if span == nil {
			return
		}

		// find matched template
		var routePrefix, routePath string
		for _, matcher := range match.Route.matchers {
			if regexp, ok := matcher.(*routeRegexp); ok && regexp != nil {
				if regexp.regexpType == 2 {
					routePrefix = regexp.template
				} else if regexp.regexpType == 0 {
					routePath = regexp.template
				}
			}
		}

		opName := routePrefix
		if routePath != "" {
			opName = routePath
		}

		// re-set the operation name if route path/prefix not empty
		if opName != "" {
			span.SetOperationName(req.Method + ":" + opName)
		}
	}()
	return
}

//inject:add
func swNewResponseWriter(val http.ResponseWriter) http.ResponseWriter {
	var rw http.ResponseWriter
	sourceWriter := val.(http.ResponseWriter)
	switch val.(type) {
	case http.Hijacker:
		rw = swNewWriterWrapperWithHijacker(sourceWriter, sourceWriter.(http.Hijacker))
	default:
		rw = swNewWriterWrapper(sourceWriter)
	}
	return rw
}

//inject:add
func swNewWriterWrapper(writer http.ResponseWriter) *swWriterWrapper {
	return &swWriterWrapper{
		ResponseWriter: writer,
		statusCode:     http.StatusOK,
	}
}

//inject:add
type swWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

//inject:add
func (w *swWriterWrapper) WriteHeader(statusCode int) {
	// cache the status code
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

//inject:add
func swNewWriterWrapperWithHijacker(writer http.ResponseWriter, hijacker http.Hijacker) *swWriterWrapperWithHijacker {
	wrapper := swNewWriterWrapper(writer)
	return &swWriterWrapperWithHijacker{
		ResponseWriter: wrapper,
		writer:         wrapper,
		Hijacker:       hijacker,
	}
}

//inject:add
type swWriterWrapperWithHijacker struct {
	http.ResponseWriter
	writer *swWriterWrapper // status code cache
	http.Hijacker
}
