//go:build goinject

//inject:google.golang.org/grpc
//inject:id grpc-stream
//inject:version >=v1.70.0
package grpc

import (
	"io"
	"strings"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type clientStream struct {
	//inject:add
	swGRPCContextData *swGRPCContextData
}

type serverStream struct {
}

func (cs *clientStream) SendMsg(m any) (err error) {
	if !strings.HasPrefix(cs.callHdr.Method, swGRPCServicePrefix) {
		if cs.swGRPCContextData != nil {
			tracing.ContinueContext(cs.swGRPCContextData.continueSnapShot)
		}
		swSpan := tracing.Span(nil)
		s, swErr := tracing.CreateLocalSpan(swGRPCFormatOperationName(cs.callHdr.Method, "/Client/Request/SendMsg"),
			tracing.WithLayer(tracing.SpanLayerRPCFramework),
			tracing.WithTag(tracing.TagURL, cs.callHdr.Method),
			tracing.WithComponent(23),
		)
		if swErr == nil {
			swSpan = s
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
	}
	return
}

func (cs *clientStream) RecvMsg(m any) (err error) {
	if !strings.HasPrefix(cs.callHdr.Method, swGRPCServicePrefix) {
		if cs.swGRPCContextData != nil {
			tracing.ContinueContext(cs.swGRPCContextData.continueSnapShot)
			cs.swGRPCContextData.interceptFinish.Store(true)
		}
		swSpan := tracing.Span(nil)
		s, swErr := tracing.CreateLocalSpan(swGRPCFormatOperationName(cs.callHdr.Method, "/Client/Response/RecvMsg"),
			tracing.WithLayer(tracing.SpanLayerRPCFramework),
			tracing.WithTag(tracing.TagURL, cs.callHdr.Method),
			tracing.WithComponent(23),
		)
		if swErr == nil {
			swSpan = s
		}
		defer func() {
			if swSpan == nil {
				return
			}
			if err != nil && err != io.EOF {
				swSpan.Error(err.Error())
			}
			if err == io.EOF {
				swSpan.SetOperationName(swGRPCFormatOperationName(cs.callHdr.Method, "/Client/Response/CloseRecv"))
			}
			swSpan.End()
			if cs.swGRPCContextData != nil {
				tracing.ContinueContext(cs.swGRPCContextData.endSnapShot)
			}
		}()
	}
	return
}

func (cs *clientStream) CloseSend() (err error) {
	if !strings.HasPrefix(cs.callHdr.Method, swGRPCServicePrefix) {
		swSpan := tracing.Span(nil)
		s, swErr := tracing.CreateLocalSpan(swGRPCFormatOperationName(cs.callHdr.Method, "/Client/Response/CloseSend"),
			tracing.WithLayer(tracing.SpanLayerRPCFramework),
			tracing.WithTag(tracing.TagURL, cs.callHdr.Method),
			tracing.WithComponent(23),
		)
		if swErr == nil {
			swSpan = s
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
	}
	return
}

func (cs *clientStream) finish(err error) {
	if cs.swGRPCContextData == nil {
		return
	}
	if !swGRPCFinishStreamSpan(cs.swGRPCContextData) {
		return
	}
	activeSpan := tracing.ActiveSpan()
	if activeSpan == nil {
		return
	}
	activeSpan.SetOperationName(swGRPCFormatOperationName(cs.callHdr.Method, "/Client/Response/CloseRecv"))
}

func (ss *serverStream) SendMsg(m any) (err error) {
	if !strings.HasPrefix(ss.s.Method(), swGRPCServicePrefix) {
		swSpan := tracing.Span(nil)
		s, swErr := tracing.CreateLocalSpan(swGRPCFormatOperationName(ss.s.Method(), "/Server/Request/SendMsg"),
			tracing.WithLayer(tracing.SpanLayerRPCFramework),
			tracing.WithTag(tracing.TagURL, ss.s.Method()),
			tracing.WithComponent(23),
		)
		if swErr == nil {
			swSpan = s
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
	}
	return
}

func (ss *serverStream) RecvMsg(m any) (err error) {
	if !strings.HasPrefix(ss.s.Method(), swGRPCServicePrefix) {
		swSpan := tracing.Span(nil)
		s, swErr := tracing.CreateLocalSpan(swGRPCFormatOperationName(ss.s.Method(), "/Server/Response/RecvMsg"),
			tracing.WithLayer(tracing.SpanLayerRPCFramework),
			tracing.WithTag(tracing.TagURL, ss.s.Method()),
			tracing.WithComponent(23),
		)
		if swErr == nil {
			swSpan = s
		}
		defer func() {
			if swSpan == nil {
				return
			}
			if err != nil && err != io.EOF {
				swSpan.Error(err.Error())
			}
			if err == io.EOF {
				swSpan.SetOperationName(swGRPCFormatOperationName(ss.s.Method(), "/Server/Response/CloseRecv"))
			}
			swSpan.End()
		}()
	}
	return
}
