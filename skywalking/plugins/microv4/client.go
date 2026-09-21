//go:build goinject

//inject:go-micro.dev/v4/client
package microv4

import (
	"context"
	"fmt"

	"go-micro.dev/v4/metadata"
	"go-micro.dev/v4/registry"
	"go-micro.dev/v4/selector"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type rpcClient struct {
}

func (r *rpcClient) next(request Request, opts CallOptions) (swNext selector.Next, swErr error) {
	defer func() {
		// NextInterceptor.AfterInvoke: tag the node address as the peer of
		// the active (client call) span each time the selector is invoked.
		swSpan := tracing.ActiveSpan()
		if swSpan == nil {
			return
		}
		if swErr != nil {
			return
		}
		if swNext != nil {
			nextSelector := swNext
			swNext = func() (*registry.Node, error) {
				node, tmp := nextSelector()
				if node != nil {
					swSpan.SetPeer(node.Address)
				}
				return node, tmp
			}
		}
	}()
	return
}

// NewClientWrapper is the public client wrapper injected into the client
// package, referenced by the go-micro.dev/v4 NewService rule.
//
//inject:add
func NewClientWrapper(cli Client) Client {
	return &swClientWrapper{cli}
}

//inject:add
type swClientWrapper struct {
	Client
}

// Call is used for client calls
//
//inject:add
func (s *swClientWrapper) Call(ctx context.Context, req Request, rsp interface{}, opts ...CallOption) error {
	span, err := tracing.CreateExitSpan(fmt.Sprintf("%s.%s", req.Service(), req.Endpoint()), req.Service(), func(k, v string) error {
		mda, _ := metadata.FromContext(ctx)
		md := metadata.Copy(mda)
		md[k] = v
		ctx = metadata.NewContext(ctx, md)
		return nil
	}, tracing.WithComponent(5008),
		tracing.WithLayer(tracing.SpanLayerRPCFramework))
	if err != nil {
		return err
	}

	defer span.End()
	if err = s.Client.Call(ctx, req, rsp, opts...); err != nil {
		span.Error(err.Error())
	}
	return err
}

// Stream is used streaming
//
//inject:add
func (s *swClientWrapper) Stream(ctx context.Context, req Request, opts ...CallOption) (Stream, error) {
	span, err := tracing.CreateExitSpan(fmt.Sprintf("%s.%s", req.Service(), req.Endpoint()), req.Service(), func(k, v string) error {
		mda, _ := metadata.FromContext(ctx)
		md := metadata.Copy(mda)
		md[k] = v
		ctx = metadata.NewContext(ctx, md)
		return nil
	}, tracing.WithComponent(5008),
		tracing.WithLayer(tracing.SpanLayerRPCFramework))
	if err != nil {
		return nil, err
	}

	defer span.End()
	stream, err := s.Client.Stream(ctx, req, opts...)
	if err != nil {
		span.Error(err.Error())
	}
	return stream, err
}

// Publish is used publish message to subscriber
//
//inject:add
func (s *swClientWrapper) Publish(ctx context.Context, p Message, opts ...PublishOption) error {
	span, err := tracing.CreateExitSpan(fmt.Sprintf("Pub to %s", p.Topic()), p.ContentType(), func(k, v string) error {
		mda, _ := metadata.FromContext(ctx)
		md := metadata.Copy(mda)
		md[k] = v
		ctx = metadata.NewContext(ctx, md)
		return nil
	}, tracing.WithComponent(5008),
		tracing.WithLayer(tracing.SpanLayerRPCFramework))
	if err != nil {
		return err
	}

	defer span.End()
	if err = s.Client.Publish(ctx, p, opts...); err != nil {
		span.Error(err.Error())
	}
	return err
}
