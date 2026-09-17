//go:build goinject

//inject:net/http
package http

// Ported from go.opentelemetry.io/otelc instrumentation/net/http (client_hook.go,
// server_hook.go, response_writer.go, semconv/{client,server,util}.go, otelc.yaml
// rules client_hook + server_hook). Traces only — the upstream hooks create no
// metric instruments, so neither does this port.
//
// The semconv helpers are inlined as //inject:add declarations instead of a
// shared package: a helper package taking *http.Request arguments would import
// net/http, and injected code inside net/http must not import anything that
// imports net/http. OtelcHTTPServerSpanName is exported because the gin
// plugin (injected into a package that already imports net/http) reuses it.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	_ "unsafe"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcHTTPInstrumentationName = "go.opentelemetry.io/otelc/instrumentation/net/http"

//inject:add
const otelcHTTPInstrumentationKey = "NETHTTP"

// Bridges into otelc/httpapi: importing go.opentelemetry.io/otel or
// go.opentelemetry.io/otel/propagation from injected net/http code would close
// an import cycle (both packages transitively import net/http), so these
// body-less linkname declarations reach the helpers without an import edge.

//inject:add
//go:linkname otelcHTTPClientTracerFn github.com/kakj-go/go-inject-trace-contrib/otelc/httpapi.TracerForClient
func otelcHTTPClientTracerFn() trace.Tracer

//inject:add
//go:linkname otelcHTTPServerTracerFn github.com/kakj-go/go-inject-trace-contrib/otelc/httpapi.TracerForServer
func otelcHTTPServerTracerFn() trace.Tracer

//inject:add
//go:linkname otelcHTTPExtractFn github.com/kakj-go/go-inject-trace-contrib/otelc/httpapi.Extract
func otelcHTTPExtractFn(ctx context.Context, h Header) context.Context

//inject:add
//go:linkname otelcHTTPInjectFn github.com/kakj-go/go-inject-trace-contrib/otelc/httpapi.Inject
func otelcHTTPInjectFn(ctx context.Context, h Header)

// ---- shared method / scheme helpers (semconv/util.go) ----

//inject:add
const otelcHTTPMethodOther = "_OTHER"

//inject:add
var otelcHTTPMethodOnce sync.Once

//inject:add
var otelcHTTPMethods map[string]attribute.KeyValue

//inject:add
var otelcHTTPMethodLookup = map[string]attribute.KeyValue{
	MethodConnect: semconv.HTTPRequestMethodConnect,
	MethodDelete:  semconv.HTTPRequestMethodDelete,
	MethodGet:     semconv.HTTPRequestMethodGet,
	MethodHead:    semconv.HTTPRequestMethodHead,
	MethodOptions: semconv.HTTPRequestMethodOptions,
	MethodPatch:   semconv.HTTPRequestMethodPatch,
	MethodPost:    semconv.HTTPRequestMethodPost,
	MethodPut:     semconv.HTTPRequestMethodPut,
	MethodTrace:   semconv.HTTPRequestMethodTrace,
}

//inject:add
func otelcHTTPKnownMethods() map[string]attribute.KeyValue {
	otelcHTTPMethodOnce.Do(func() {
		otelcHTTPMethods = otelcHTTPMethodLookup
		if env := os.Getenv("OTEL_INSTRUMENTATION_HTTP_KNOWN_METHODS"); env != "" {
			out := make(map[string]attribute.KeyValue)
			for _, part := range strings.Split(env, ",") {
				method := strings.TrimSpace(part)
				if method == "" {
					continue
				}
				if attr, ok := otelcHTTPMethodLookup[method]; ok {
					out[method] = attr
					continue
				}
				out[method] = semconv.HTTPRequestMethodKey.String(method)
			}
			otelcHTTPMethods = out
		}
	})
	return otelcHTTPMethods
}

//inject:add
func otelcStandardizeHTTPMethod(method string) string {
	lookup := otelcHTTPKnownMethods()
	if _, ok := lookup[method]; ok {
		return method
	}
	upper := strings.ToUpper(method)
	if _, ok := lookup[upper]; ok {
		return upper
	}
	return otelcHTTPMethodOther
}

//inject:add
func otelcRequestMethodAttrs(method string) (attribute.KeyValue, attribute.KeyValue) {
	if method == "" {
		return semconv.HTTPRequestMethodOther, attribute.KeyValue{}
	}
	lookup := otelcHTTPKnownMethods()
	if attr, ok := lookup[method]; ok {
		return attr, attribute.KeyValue{}
	}
	if attr, ok := lookup[strings.ToUpper(method)]; ok {
		return attr, semconv.HTTPRequestMethodOriginal(method)
	}
	if method == otelcHTTPMethodOther {
		return semconv.HTTPRequestMethodOther, attribute.KeyValue{}
	}
	return semconv.HTTPRequestMethodOther, semconv.HTTPRequestMethodOriginal(method)
}

//inject:add
func OtelcSpanMethod(method string) string {
	standardized := otelcStandardizeHTTPMethod(method)
	if standardized == otelcHTTPMethodOther {
		return "HTTP"
	}
	return standardized
}

//inject:add
func OtelcSplitHostPort(hostport string) (string, int) {
	host := ""
	port := -1

	if strings.HasPrefix(hostport, "[") {
		addrEnd := strings.LastIndexByte(hostport, ']')
		if addrEnd < 0 {
			return host, port
		}
		if i := strings.LastIndexByte(hostport[addrEnd:], ':'); i < 0 {
			host = hostport[1:addrEnd]
			return host, port
		}
	} else {
		if i := strings.LastIndexByte(hostport, ':'); i < 0 {
			host = hostport
			return host, port
		}
	}

	host, pStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return host, port
	}
	p, err := strconv.ParseUint(pStr, 10, 16)
	if err != nil {
		return host, port
	}
	return host, int(p)
}

//inject:add
func otelcRequiredHTTPPort(https bool, port int) int {
	if https {
		if port > 0 && port != 443 {
			return port
		}
	} else {
		if port > 0 && port != 80 {
			return port
		}
	}
	return -1
}

//inject:add
func otelcServerClientIP(xForwardedFor string) string {
	if idx := strings.IndexByte(xForwardedFor, ','); idx >= 0 {
		xForwardedFor = xForwardedFor[:idx]
	}
	return xForwardedFor
}

//inject:add
func OtelcHTTPRoute(pattern string) string {
	if idx := strings.IndexByte(pattern, '/'); idx >= 0 {
		return pattern[idx:]
	}
	return ""
}

//inject:add
func otelcNetProtocol(proto string) (string, string) {
	name, version, _ := strings.Cut(proto, "/")
	switch name {
	case "HTTP":
		name = "http"
	case "QUIC":
		name = "quic"
	case "SPDY":
		name = "spdy"
	default:
		name = strings.ToLower(name)
	}
	return name, version
}

// ---- server span name (exported: reused by the gin plugin) ----

//inject:add
func OtelcHTTPServerSpanName(method, route string) string {
	method = OtelcSpanMethod(method)
	if route != "" {
		return method + " " + route
	}
	return method
}

// ---- server semconv (semconv/server.go) ----

//inject:add
func otelcHTTPServerRequestTraceAttrs(server string, req *Request) []attribute.KeyValue {
	count := 3 // ServerAddress, Method, Scheme

	var host string
	var p int
	if server == "" {
		host, p = OtelcSplitHostPort(req.Host)
	} else {
		host, p = OtelcSplitHostPort(server)
		if p < 0 {
			_, p = OtelcSplitHostPort(req.Host)
		}
	}

	hostPort := otelcRequiredHTTPPort(req.TLS != nil, p)
	if hostPort > 0 {
		count++
	}

	method, methodOriginal := otelcRequestMethodAttrs(req.Method)
	if methodOriginal != (attribute.KeyValue{}) {
		count++
	}

	scheme := otelcHTTPServerScheme(req.TLS != nil)

	peer, peerPort := OtelcSplitHostPort(req.RemoteAddr)
	if peer != "" {
		count++
		if peerPort > 0 {
			count++
		}
	}

	useragent := req.UserAgent()
	if useragent != "" {
		count++
	}

	clientIP := otelcServerClientIP(req.Header.Get("X-Forwarded-For"))
	if clientIP == "" {
		clientIP = peer
	}
	if clientIP != "" {
		count++
	}

	if req.URL != nil && req.URL.Path != "" {
		count++
	}
	if req.URL != nil && req.URL.RawQuery != "" {
		count++
	}

	protoName, protoVersion := otelcNetProtocol(req.Proto)
	if protoName != "" && protoName != "http" {
		count++
	}
	if protoVersion != "" {
		count++
	}

	route := OtelcHTTPRoute(req.Pattern)
	if route != "" {
		count++
	}

	attrs := make([]attribute.KeyValue, 0, count)
	attrs = append(attrs,
		semconv.ServerAddress(host),
		method,
		scheme,
	)

	if hostPort > 0 {
		attrs = append(attrs, semconv.ServerPort(hostPort))
	}
	if methodOriginal != (attribute.KeyValue{}) {
		attrs = append(attrs, methodOriginal)
	}
	if peer != "" {
		attrs = append(attrs, semconv.NetworkPeerAddress(peer))
		if peerPort > 0 {
			attrs = append(attrs, semconv.NetworkPeerPort(peerPort))
		}
	}
	if useragent != "" {
		attrs = append(attrs, semconv.UserAgentOriginal(useragent))
	}
	if clientIP != "" {
		attrs = append(attrs, semconv.ClientAddress(clientIP))
	}
	if req.URL != nil && req.URL.Path != "" {
		attrs = append(attrs, semconv.URLPath(req.URL.Path))
	}
	if req.URL != nil && req.URL.RawQuery != "" {
		attrs = append(attrs, semconv.URLQuery(req.URL.RawQuery))
	}
	if protoName != "" && protoName != "http" {
		attrs = append(attrs, semconv.NetworkProtocolName(protoName))
	}
	if protoVersion != "" {
		attrs = append(attrs, semconv.NetworkProtocolVersion(protoVersion))
	}
	if route != "" {
		attrs = append(attrs, semconv.HTTPRoute(route))
	}

	return attrs
}

//inject:add
func otelcHTTPServerScheme(https bool) attribute.KeyValue {
	if https {
		return semconv.URLScheme("https")
	}
	return semconv.URLScheme("http")
}

//inject:add
func otelcHTTPServerResponseTraceAttrs(statusCode int, writeBytes int64) []attribute.KeyValue {
	var count int
	if writeBytes > 0 {
		count++
	}
	if statusCode > 0 {
		count++
	}
	if statusCode >= 500 && statusCode < 600 {
		count++
	}

	attributes := make([]attribute.KeyValue, 0, count)
	if writeBytes > 0 {
		attributes = append(attributes, semconv.HTTPResponseBodySize(int(writeBytes)))
	}
	if statusCode > 0 {
		attributes = append(attributes, semconv.HTTPResponseStatusCode(statusCode))
	}
	if statusCode >= 500 && statusCode < 600 {
		attributes = append(attributes, semconv.ErrorTypeKey.String(strconv.Itoa(statusCode)))
	}
	return attributes
}

//inject:add
func otelcHTTPServerStatus(code int) (codes.Code, string) {
	if code < 100 || code >= 600 {
		return codes.Error, fmt.Sprintf("Invalid HTTP status code %d", code)
	}
	if code >= 500 {
		return codes.Error, ""
	}
	return codes.Unset, ""
}

// ---- client semconv (semconv/client.go) ----

//inject:add
func otelcHTTPClientRequestTraceAttrs(req *Request) []attribute.KeyValue {
	numOfAttributes := 4 // URL, server address, method, and scheme.

	var urlHost string
	if req.URL != nil {
		urlHost = req.URL.Host
	}
	var requestHost string
	var requestPort int
	for _, hostport := range []string{urlHost, req.Header.Get("Host")} {
		requestHost, requestPort = OtelcSplitHostPort(hostport)
		if requestHost != "" || requestPort > 0 {
			break
		}
	}

	eligiblePort := otelcRequiredHTTPPort(req.URL != nil && req.URL.Scheme == "https", requestPort)
	if eligiblePort > 0 {
		numOfAttributes++
	}

	protoName, protoVersion := otelcNetProtocol(req.Proto)
	if protoName != "" && protoName != "http" {
		numOfAttributes++
	}
	if protoVersion != "" {
		numOfAttributes++
	}

	method, originalMethod := otelcRequestMethodAttrs(req.Method)
	if originalMethod != (attribute.KeyValue{}) {
		numOfAttributes++
	}

	useragent := req.UserAgent()
	if useragent != "" {
		numOfAttributes++
	}

	attrs := make([]attribute.KeyValue, 0, numOfAttributes)

	attrs = append(attrs, method)
	if originalMethod != (attribute.KeyValue{}) {
		attrs = append(attrs, originalMethod)
	}

	var u string
	if req.URL != nil {
		userinfo := req.URL.User
		req.URL.User = nil
		u = req.URL.String()
		req.URL.User = userinfo
	}
	attrs = append(attrs, semconv.URLFull(u))

	attrs = append(attrs, semconv.ServerAddress(requestHost))
	if eligiblePort > 0 {
		attrs = append(attrs, semconv.ServerPort(eligiblePort))
	}

	if req.URL != nil && req.URL.Scheme != "" {
		attrs = append(attrs, semconv.URLScheme(req.URL.Scheme))
	} else {
		attrs = append(attrs, semconv.URLScheme("http"))
	}

	if protoName != "" && protoName != "http" {
		attrs = append(attrs, semconv.NetworkProtocolName(protoName))
	}
	if protoVersion != "" {
		attrs = append(attrs, semconv.NetworkProtocolVersion(protoVersion))
	}

	if useragent != "" {
		attrs = append(attrs, semconv.UserAgentOriginal(useragent))
	}

	return attrs
}

//inject:add
func otelcHTTPClientResponseTraceAttrs(resp *Response) []attribute.KeyValue {
	var count int
	if resp.StatusCode > 0 {
		count++
	}
	if otelcIsErrorStatusCode(resp.StatusCode) {
		count++
	}

	attrs := make([]attribute.KeyValue, 0, count)
	if resp.StatusCode > 0 {
		attrs = append(attrs, semconv.HTTPResponseStatusCode(resp.StatusCode))
	}
	if otelcIsErrorStatusCode(resp.StatusCode) {
		attrs = append(attrs, semconv.ErrorTypeKey.String(strconv.Itoa(resp.StatusCode)))
	}
	return attrs
}

//inject:add
func otelcIsErrorStatusCode(code int) bool {
	return code >= 400 || code < 100
}

//inject:add
func otelcHTTPClientStatus(code int) (codes.Code, string) {
	if code < 100 || code >= 600 {
		return codes.Error, fmt.Sprintf("Invalid HTTP status code %d", code)
	}
	if code >= 400 {
		return codes.Error, ""
	}
	return codes.Unset, ""
}

//inject:add
func otelcHTTPClientErrorType(err error) attribute.KeyValue {
	t := reflect.TypeOf(err)
	var value string
	if t.PkgPath() == "" && t.Name() == "" {
		value = t.String()
	} else {
		value = fmt.Sprintf("%s.%s", t.PkgPath(), t.Name())
	}

	if value == "" {
		return semconv.ErrorTypeOther
	}

	return semconv.ErrorTypeKey.String(value)
}

//inject:add
func otelcHTTPClientSpanName(method string) string {
	return OtelcSpanMethod(method)
}

//inject:add
func otelcIsOTelExporterRequest(req *Request) bool {
	ua := req.Header.Get("User-Agent")
	return strings.HasPrefix(ua, "OTel OTLP Exporter Go") || strings.HasPrefix(ua, "OTel Go OTLP") ||
		strings.HasPrefix(ua, "OTel-Go-OTLP")
}

// ---- response writer wrapper (response_writer.go) ----

// otelcWriteOnly hides every method of the wrapped value except Write.
//inject:add
type otelcWriteOnly struct {
	io.Writer
}

//inject:add
type otelcWriterWrapper struct {
	ResponseWriter
	otelcStatusCode  int
	otelcWroteHeader bool
}

//inject:add
func (w *otelcWriterWrapper) WriteHeader(statusCode int) {
	if w.otelcWroteHeader {
		return
	}
	w.otelcStatusCode = statusCode
	w.otelcWroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

//inject:add
func (w *otelcWriterWrapper) Write(b []byte) (int, error) {
	if !w.otelcWroteHeader {
		w.WriteHeader(StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

//inject:add
func (w *otelcWriterWrapper) ReadFrom(src io.Reader) (int64, error) {
	if !w.otelcWroteHeader {
		w.WriteHeader(StatusOK)
	}

	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}

	return io.Copy(otelcWriteOnly{w}, src)
}

//inject:add
func (w *otelcWriterWrapper) WriteString(s string) (int, error) {
	if !w.otelcWroteHeader {
		w.WriteHeader(StatusOK)
	}

	if sw, ok := w.ResponseWriter.(io.StringWriter); ok {
		return sw.WriteString(s)
	}

	return w.ResponseWriter.Write([]byte(s))
}

//inject:add
func (w *otelcWriterWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("responseWriter does not implement http.Hijacker")
}

//inject:add
func (w *otelcWriterWrapper) Flush() {
	if f, ok := w.ResponseWriter.(Flusher); ok {
		f.Flush()
	}
}

//inject:add
func (w *otelcWriterWrapper) Push(target string, opts *PushOptions) error {
	if pusher, ok := w.ResponseWriter.(Pusher); ok {
		return pusher.Push(target, opts)
	}
	return ErrNotSupported
}

//inject:add
func (w *otelcWriterWrapper) Unwrap() ResponseWriter {
	return w.ResponseWriter
}

// ---- interception templates ----

// Empty projections anchor the intercepted methods to the real types.
type Transport struct{}

type serverHandler struct{}

// Port of BeforeRoundTrip/AfterRoundTrip: guards only wrap span creation, so
// every early exit still runs the original transport logic; the deferred
// closure reads the named results after the original body fills them.
func (t *Transport) RoundTrip(req *Request) (resp *Response, err error) {
	var otelcSpan trace.Span
	if hooksupport.Instrumented(otelcHTTPInstrumentationKey) && req != nil &&
		!hooksupport.IsHTTPClientInstrumentationSuppressed(req.Context()) &&
		!otelcIsOTelExporterRequest(req) {
		ctx := otelcHTTPExtractFn(req.Context(), req.Header)

		attrs := otelcHTTPClientRequestTraceAttrs(req)

		ctx, otelcSpan = otelcHTTPClientTracerFn().Start(ctx,
			otelcHTTPClientSpanName(req.Method),
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attrs...),
		)

		if req.Header == nil {
			req.Header = make(Header)
		}

		otelcHTTPInjectFn(ctx, req.Header)

		req = req.WithContext(ctx)
	}
	if otelcSpan != nil {
		defer func() {
			span := otelcSpan
			defer span.End()

			if resp != nil {
				span.SetAttributes(otelcHTTPClientResponseTraceAttrs(resp)...)

				code, desc := otelcHTTPClientStatus(resp.StatusCode)
				if code != codes.Unset {
					span.SetStatus(code, desc)
				}
			}

			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				span.SetAttributes(otelcHTTPClientErrorType(err))
			}
		}()
	}
	return
}

// Port of BeforeServeHTTP/AfterServeHTTP: wraps the writer to capture the
// status code, threads the span context through the request, and renames the
// span to "METHOD /route" once ServeMux fills r.Pattern (Go 1.22+) during the
// original body.
func (sh serverHandler) ServeHTTP(w ResponseWriter, r *Request) {
	var otelcSpan trace.Span
	if hooksupport.Instrumented(otelcHTTPInstrumentationKey) {
		ctx := otelcHTTPExtractFn(r.Context(), r.Header)

		attrs := otelcHTTPServerRequestTraceAttrs("", r)

		ctx, otelcSpan = otelcHTTPServerTracerFn().Start(ctx,
			OtelcHTTPServerSpanName(r.Method, ""),
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attrs...),
		)

		w = &otelcWriterWrapper{
			ResponseWriter: w,
			otelcStatusCode: StatusOK,
		}

		r = r.WithContext(ctx)
	}
	if otelcSpan != nil {
		otelcW, otelcR := w, r
		defer func() {
			span := otelcSpan
			defer span.End()

			if otelcR != nil && span.IsRecording() {
				if route := OtelcHTTPRoute(otelcR.Pattern); route != "" {
					span.SetName(OtelcHTTPServerSpanName(otelcR.Method, route))
					span.SetAttributes(semconv.HTTPRoute(route))
				}
			}

			statusCode := StatusOK
			if wrapper, ok := otelcW.(*otelcWriterWrapper); ok {
				statusCode = wrapper.otelcStatusCode
			}

			span.SetAttributes(otelcHTTPServerResponseTraceAttrs(statusCode, 0)...)

			code, desc := otelcHTTPServerStatus(statusCode)
			if code != codes.Unset {
				span.SetStatus(code, desc)
			}
		}()
	}
}
