//go:build goinject

//inject:google.golang.org/grpc
//inject:id grpc-server
//inject:version >=v1.70.0
package grpc

import (
	"context"

	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/internal/transport"
	"google.golang.org/grpc/metadata"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type Server struct {
}

func (s *Server) handleStream(t transport.ServerTransport, stream *transport.ServerStream) {
	swSpan := tracing.Span(nil)
	swMethod := stream.Method()
	md, _ := metadata.FromIncomingContext(stream.Context())
	s2, swErr := tracing.CreateEntrySpan(swGRPCFormatOperationName(swMethod, ""), func(headerKey string) (string, error) {
		value := ""
		vals := md.Get(headerKey)
		if len(vals) > 0 {
			value = vals[0]
		}
		return value, nil
	}, tracing.WithLayer(tracing.SpanLayerRPCFramework),
		tracing.WithTag(tracing.TagURL, swMethod),
		tracing.WithComponent(23),
	)
	if swErr == nil {
		swSpan = s2
	}
	defer func() {
		if swSpan == nil {
			return
		}
		swSpan.End()
	}()
}

func (s *Server) sendResponse(ctx context.Context, stream *transport.ServerStream, msg any, cp Compressor, opts *transport.WriteOptions, comp encoding.Compressor) (err error) {
	swSpan := tracing.Span(nil)
	if tracing.ActiveSpan() != nil {
		s2, swErr := tracing.CreateLocalSpan(swGRPCFormatOperationName(stream.Method(), "/Server/Response/SendResponse"),
			tracing.WithLayer(tracing.SpanLayerRPCFramework),
			tracing.WithTag(tracing.TagURL, stream.Method()),
			tracing.WithComponent(23),
		)
		if swErr == nil {
			swSpan = s2
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

func (s *Server) processUnaryRPC(ctx context.Context, stream *transport.ServerStream, info *serviceInfo, md *MethodDesc, trInfo *traceInfo) (err error) {
	if activeSpan := tracing.ActiveSpan(); activeSpan != nil {
		activeSpan.Tag(swRPCTypeTag, "Unary")
	}
	return
}

func (s *Server) processStreamingRPC(ctx context.Context, stream *transport.ServerStream, info *serviceInfo, sd *StreamDesc, trInfo *traceInfo) (err error) {
	if activeSpan := tracing.ActiveSpan(); activeSpan != nil {
		activeSpan.Tag(swRPCTypeTag, "Streaming")
	}
	return
}
