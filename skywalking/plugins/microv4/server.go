//go:build goinject

//inject:go-micro.dev/v4/server
package microv4

import (
	"context"

	"go-micro.dev/v4/metadata"
	"go-micro.dev/v4/transport"
	"go-micro.dev/v4/util/socket"

	"github.com/apache/skywalking-go/plugins/core/tools"
	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type router struct {
}

func (r *router) ServeRequest(ctx context.Context, req Request, rsp Response) (swErr error) {
	// ServeRequestInterceptor.BeforeInvoke: build the entry span (or adopt
	// the connection span stored by the socket Accept rule). On span
	// creation failure the interceptor skips the original call and returns
	// the zero error.
	swSpan, swSpanErr := swCreatingSpan(ctx, req)
	if swSpanErr != nil {
		return
	}
	defer func() {
		if swSpan == nil {
			return
		}
		swSpan.End()
	}()
	return
}

//inject:add
var swMicroComponentID int32 = 5009

//inject:add
func swCreatingSpan(ctx context.Context, req Request) (tracing.Span, error) {
	endpoint := req.Service() + "." + req.Endpoint()
	if s := swGetExistingSpan(req); s != nil {
		s.SetOperationName(endpoint)
		s.SetSpanLayer(tracing.SpanLayerRPCFramework)
		s.SetComponent(swMicroComponentID)
		return s, nil
	}
	return tracing.CreateEntrySpan(endpoint, func(headerKey string) (string, error) {
		al, _ := metadata.Get(ctx, headerKey)
		return al, nil
	}, tracing.WithComponent(swMicroComponentID),
		tracing.WithLayer(tracing.SpanLayerRPCFramework))
}

//inject:add
func swGetExistingSpan(req Request) tracing.Span {
	// find the transport.Socket field inside the request (the connection
	// the request was received on) and reuse its injected span, if any
	socketVal := tools.GetInstanceValueByType(req, tools.WithInterfaceType((*transport.Socket)(nil)))
	if socketVal == nil {
		return nil
	}
	sock, ok := socketVal.(*socket.Socket)
	if !ok || sock.SwInjectData == nil {
		return nil
	}
	tracing.ContinueContext(sock.SwInjectData.Snapshot)
	return sock.SwInjectData.Span
}
