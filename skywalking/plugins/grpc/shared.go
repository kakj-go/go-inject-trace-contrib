//go:build goinject

//inject:google.golang.org/grpc
package grpc

import (
	"strings"
	"sync/atomic"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

//inject:add
const swRPCTypeTag = "rpc.type"

//inject:add
const swGRPCServicePrefix = "/skywalking"

//inject:add
func swGRPCFormatOperationName(service, method string) string {
	service = service[1:]
	service = strings.ReplaceAll(service, "/", ".")
	return service + method
}

// swGRPCContextData carries the async client-streaming span state between
// the NewStream interception and the per-message/finish interceptions.
//
//inject:add
type swGRPCContextData struct {
	// asyncSpan is the span that calls PrepareAsync()
	asyncSpan tracing.Span
	// continueSnapShot is the snapshot that the span has not ended,
	// When the service is in progress, it should be continued
	continueSnapShot tracing.ContextSnapshot
	// endSnapShot is the snapshot that the span has ended
	// When the service is completely finished, it should be continued
	endSnapShot tracing.ContextSnapshot
	// interceptFinish is whether to intercept finish(). RecvMsg writes it on
	// the user goroutine while Finish may consume it from a gRPC-internal
	// goroutine, so it is atomic; the CAS consumer also makes finish one-shot.
	interceptFinish atomic.Bool
}

// swGRPCFinishStreamSpan consumes the one-shot finish flag and finishes the
// async stream span. Only the first caller wins: either RecvMsg never armed
// the finish, or a concurrent Finish already consumed it - so two racing
// Finish calls can never both run the AsyncFinish.
//
//inject:add
func swGRPCFinishStreamSpan(contextdata *swGRPCContextData) bool {
	if !contextdata.interceptFinish.CompareAndSwap(true, false) {
		return false
	}
	if contextdata.asyncSpan != nil {
		contextdata.asyncSpan.AsyncFinish()
	}
	return true
}
