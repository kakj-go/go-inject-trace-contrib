//go:build goinject

//inject:dubbo.apache.org/dubbo-go/v3/filter/graceful_shutdown
package dubbo

import (
	"context"

	"dubbo.apache.org/dubbo-go/v3/common/constant"
	"dubbo.apache.org/dubbo-go/v3/protocol"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type consumerGracefulShutdownFilter struct {
}

func (f *consumerGracefulShutdownFilter) Invoke(ctx context.Context, invoker protocol.Invoker, invocation protocol.Invocation) (__injectResult0 protocol.Result) {
	swSpan := tracing.Span(nil)
	if url := invoker.GetURL(); url != nil {
		generateOperationName := func(invoker protocol.Invoker, inv protocol.Invocation) string {
			group := invoker.GetURL().GetParam(constant.GroupKey, "")
			if group != "" {
				group = "/" + group
			}
			return group + invoker.GetURL().Path + "/" + inv.MethodName()
		}
		s, err := tracing.CreateExitSpan(generateOperationName(invoker, invocation), url.Location, func(k, v string) error {
			invocation.SetAttachment(k, v)
			return nil
		}, tracing.WithLayer(tracing.SpanLayerRPCFramework),
			tracing.WithTag(tracing.TagURL, url.String()),
			tracing.WithComponent(3))
		if err == nil {
			swSpan = s
		}
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if res, ok := __injectResult0.(*protocol.RPCResult); ok && res.Error() != nil {
			swSpan.Error(res.Error().Error())
		}
		swSpan.End()
	}()
	return
}
