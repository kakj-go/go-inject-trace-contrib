//go:build goinject

//inject:google.golang.org/grpc
//inject:id grpc-client
//inject:version >=v1.70.0
package grpc

import (
	"context"
	"strings"

	"google.golang.org/grpc/metadata"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type ClientConn struct {
}

func (cc *ClientConn) Invoke(ctx context.Context, method string, args, reply any, opts ...CallOption) (err error) {
	swSpan := tracing.Span(nil)
	if !strings.HasPrefix(method, swGRPCServicePrefix) {
		s, swErr := tracing.CreateExitSpan(swGRPCFormatOperationName(method, ""), cc.Target(), func(headerKey, headerValue string) error {
			ctx = metadata.AppendToOutgoingContext(ctx, headerKey, headerValue)
			return nil
		},
			tracing.WithLayer(tracing.SpanLayerRPCFramework),
			tracing.WithTag(tracing.TagURL, method),
			tracing.WithComponent(23),
		)
		if swErr == nil {
			swSpan = s
			s.Tag(swRPCTypeTag, "Unary")
		}
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

func (cc *ClientConn) NewStream(ctx context.Context, desc *StreamDesc, method string, opts ...CallOption) (cs ClientStream, err error) {
	swSpan := tracing.Span(nil)
	if !strings.HasPrefix(method, swGRPCServicePrefix) {
		s, swErr := tracing.CreateExitSpan(swGRPCFormatOperationName(method, ""), cc.Target(), func(headerKey, headerValue string) error {
			ctx = metadata.AppendToOutgoingContext(ctx, headerKey, headerValue)
			return nil
		},
			tracing.WithLayer(tracing.SpanLayerRPCFramework),
			tracing.WithTag(tracing.TagURL, method),
			tracing.WithComponent(23),
		)
		if swErr == nil {
			swSpan = s
			s.Tag(swRPCTypeTag, "Streaming")
		}
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.PrepareAsync()
		swContinueSnapShot := tracing.CaptureContext()
		swSpan.End()
		if realCS, ok := cs.(*clientStream); ok {
			realCS.swGRPCContextData = &swGRPCContextData{
				asyncSpan:        swSpan,
				continueSnapShot: swContinueSnapShot,
				endSnapShot:      tracing.CaptureContext(),
			}
		}
	}()
	return
}
