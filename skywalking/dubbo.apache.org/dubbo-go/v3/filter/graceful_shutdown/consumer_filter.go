//inject:dubbo.apache.org/dubbo-go/v3/filter/graceful_shutdown/consumer_filter.go
package graceful_shutdown

import (
	"context"
	"dubbo.apache.org/dubbo-go/v3/common/constant"
	"dubbo.apache.org/dubbo-go/v3/protocol"
	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type consumerGracefulShutdownFilter struct {
}

func (f *consumerGracefulShutdownFilter) Invoke(ctx context.Context, invoker protocol.Invoker, invocation protocol.Invocation) (__injectResult0 protocol.Result) {
	generateOperationName := func(invoker protocol.Invoker, inv protocol.Invocation) string {
		group := invoker.GetURL().GetParam(constant.GroupKey, "")
		if group != "" {
			group = "/" + group
		}
		return group + invoker.GetURL().Path + "/" + inv.MethodName()
	}

	filterInvoker := invoker
	dubboInv := invocation
	url := filterInvoker.GetURL()
	if url != nil {
		s, err := tracing.CreateExitSpan(generateOperationName(filterInvoker, dubboInv), url.Location, func(k, v string) error {
			dubboInv.SetAttachment(k, v)
			return nil
		}, tracing.WithLayer(tracing.SpanLayerRPCFramework),
			tracing.WithTag(tracing.TagURL, url.String()),
			tracing.WithComponent(3))

		defer func() {
			if err == nil {
				return
			}
			span := s
			if res, ok := __injectResult0.(*protocol.RPCResult); ok && res.Error() != nil {
				span.Error(res.Error().Error())
			}
			span.End()
		}()
	}

	return nil
}
