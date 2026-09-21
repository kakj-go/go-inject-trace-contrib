//go:build goinject

//inject:github.com/go-kratos/kratos/v2/transport/http
package kratosv2

import (
	"context"
	"net/url"

	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"

	"github.com/apache/skywalking-go/plugins/core/log"
	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type Server struct {
	//inject:add
	swKratosEnhanced bool
}

type clientOptions struct {
	//inject:add
	swKratosEnhanced bool
}

func NewServer(opts ...ServerOption) (swRet *Server) {
	defer func() {
		if swRet.swKratosEnhanced {
			return
		}
		// adding the middleware to the server
		tracing.SetRuntimeContextValue(swHTTPIgnoreServerMiddlewareKey, true)
		Middleware(swHTTPServerMiddleware)(swRet)
		swRet.swKratosEnhanced = true
	}()
	return
}

func Middleware(m ...middleware.Middleware) (swRet ServerOption) {
	// ServerMiddlewareInterceptor.BeforeInvoke: when the NewServer
	// interceptor applies the middleware directly, the ignore flag is set,
	// consume it and keep the option untouched.
	swWrapped := false
	if tracing.GetRuntimeContextValue(swHTTPIgnoreServerMiddlewareKey) != nil {
		tracing.SetRuntimeContextValue(swHTTPIgnoreServerMiddlewareKey, nil)
	} else {
		m = append(m, swHTTPServerMiddleware)
		swWrapped = true
	}
	defer func() {
		if !swWrapped {
			return
		}
		opt := swRet
		// wrapper the server option, and adding the true value to the server to let the interceptor know the server has been enhanced
		swRet = func(server *Server) {
			opt(server)
			server.swKratosEnhanced = true
		}
	}()
	return
}

func NewClient(ctx context.Context, opts ...ClientOption) (swRet *Client, swErr error) {
	defer func() {
		if swRet.opts.swKratosEnhanced {
			return
		}
		// adding the middleware to the client
		tracing.SetRuntimeContextValue(swHTTPIgnoreClientMiddlewareKey, true)
		WithMiddleware(swHTTPClientMiddleware)(&swRet.opts)
		swRet.opts.swKratosEnhanced = true
	}()
	return
}

func WithMiddleware(m ...middleware.Middleware) (swRet ClientOption) {
	// ClientMiddlewareInterceptor.BeforeInvoke: mirror of the server side,
	// the ignore flag suppresses appending when NewClient applies the
	// middleware itself.
	swWrapped := false
	if tracing.GetRuntimeContextValue(swHTTPIgnoreClientMiddlewareKey) != nil {
		tracing.SetRuntimeContextValue(swHTTPIgnoreClientMiddlewareKey, nil)
	} else {
		m = append(m, swHTTPClientMiddleware)
		swWrapped = true
	}
	defer func() {
		if !swWrapped {
			return
		}
		opt := swRet
		// wrapper the client option, and adding the true value to the client to let the interceptor know the client has been enhanced
		swRet = func(o *clientOptions) {
			opt(o)
			o.swKratosEnhanced = true
		}
	}()
	return
}

//inject:add
const swHTTPIgnoreServerMiddlewareKey = "ignoreServerMiddleware"

//inject:add
const swHTTPIgnoreClientMiddlewareKey = "ignoreClientMiddleware"

//inject:add
var swHTTPServerMiddleware = func(handler middleware.Handler) middleware.Handler {
	return func(c context.Context, req interface{}) (interface{}, error) {
		if tr, ok := transport.FromServerContext(c); ok {
			span, err := tracing.CreateEntrySpan(tr.Operation(), func(key string) (string, error) {
				return tr.RequestHeader().Get(key), nil
			}, tracing.WithComponent(5010),
				tracing.WithLayer(tracing.SpanLayerRPCFramework),
				tracing.WithTag("transport", "HTTP"))
			if err != nil {
				log.Warnf("cannot create entry span: %v", err)
				return handler(c, req)
			}
			defer span.End()

			reply, err := handler(c, req)
			if err != nil {
				span.Error(err.Error())
			}
			return reply, err
		}
		return handler(c, req)
	}
}

//inject:add
var swHTTPClientMiddleware = func(handler middleware.Handler) middleware.Handler {
	return func(c context.Context, req interface{}) (interface{}, error) {
		if tr, ok := transport.FromClientContext(c); ok {
			peer := tr.Endpoint()
			if parse, _ := url.Parse(tr.Endpoint()); parse != nil {
				peer = parse.Host
			}
			span, err := tracing.CreateExitSpan(tr.Operation(), peer, func(key, value string) error {
				tr.RequestHeader().Add(key, value)
				return nil
			}, tracing.WithComponent(5010),
				tracing.WithLayer(tracing.SpanLayerRPCFramework),
				tracing.WithTag("transport", "HTTP"))
			if err != nil {
				log.Warnf("cannot create exit span: %v", err)
				return handler(c, req)
			}
			defer span.End()

			reply, err := handler(c, req)
			if err != nil {
				span.Error(err.Error())
			}
			return reply, err
		}
		return handler(c, req)
	}
}
