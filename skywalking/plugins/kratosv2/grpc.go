//go:build goinject

//inject:github.com/go-kratos/kratos/v2/transport/grpc
package kratosv2

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/selector"
	"github.com/go-kratos/kratos/v2/transport"
	"google.golang.org/grpc"

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
		tracing.SetRuntimeContextValue(swGRPCIgnoreServerMiddlewareKey, true)
		Middleware(swGRPCServerMiddleware)(swRet)
		swRet.swKratosEnhanced = true
	}()
	return
}

func Middleware(m ...middleware.Middleware) (swRet ServerOption) {
	// MiddlewareInterceptor.BeforeInvoke: when the NewServer interceptor
	// applies the middleware directly, the ignore flag is set, consume it
	// and keep the option untouched.
	swWrapped := false
	if tracing.GetRuntimeContextValue(swGRPCIgnoreServerMiddlewareKey) != nil {
		tracing.SetRuntimeContextValue(swGRPCIgnoreServerMiddlewareKey, nil)
	} else {
		m = append(m, swGRPCServerMiddleware)
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

func unaryClientInterceptor(ms []middleware.Middleware, timeout time.Duration, filters []selector.NodeFilter) grpc.UnaryClientInterceptor {
	// UnaryClientInterceptor.BeforeInvoke: the returned interceptor closure
	// captures ms, so appending here injects the tracing middleware.
	ms = append(ms, swGRPCClientMiddleware)
}

//inject:add
const swGRPCIgnoreServerMiddlewareKey = "ignoreServerMiddleware"

//inject:add
var swGRPCServerMiddleware = func(handler middleware.Handler) middleware.Handler {
	return func(c context.Context, req interface{}) (interface{}, error) {
		if tr, ok := transport.FromServerContext(c); ok {
			span, err := tracing.CreateEntrySpan(tr.Operation(), func(key string) (string, error) {
				return tr.RequestHeader().Get(key), nil
			}, tracing.WithComponent(5010),
				tracing.WithLayer(tracing.SpanLayerRPCFramework),
				tracing.WithTag("transport", "gRPC"))
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
var swGRPCClientMiddleware = func(handler middleware.Handler) middleware.Handler {
	return func(c context.Context, req interface{}) (interface{}, error) {
		if tr, ok := transport.FromClientContext(c); ok {
			span, err := tracing.CreateExitSpan(tr.Operation(), tr.Endpoint(), func(key, value string) error {
				tr.RequestHeader().Add(key, value)
				return nil
			}, tracing.WithComponent(5010),
				tracing.WithLayer(tracing.SpanLayerRPCFramework),
				tracing.WithTag("transport", "gRPC"))
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
