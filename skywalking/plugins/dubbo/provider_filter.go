//go:build goinject

//inject:dubbo.apache.org/dubbo-go/v3/filter/graceful_shutdown
package dubbo

import (
	"context"

	"dubbo.apache.org/dubbo-go/v3/common/constant"
	"dubbo.apache.org/dubbo-go/v3/protocol"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type providerGracefulShutdownFilter struct {
}

func (f *providerGracefulShutdownFilter) Invoke(ctx context.Context, invoker protocol.Invoker, invocation protocol.Invocation) (__injectResult0 protocol.Result) {
	swSpan := tracing.Span(nil)
	if url := invoker.GetURL(); url != nil {
		generateOperationName := func(invoker protocol.Invoker, inv protocol.Invocation) string {
			group := invoker.GetURL().GetParam(constant.GroupKey, "")
			if group != "" {
				group = "/" + group
			}
			return group + invoker.GetURL().Path + "/" + inv.MethodName()
		}
		s, err := tracing.CreateEntrySpan(generateOperationName(invoker, invocation), func(k string) (string, error) {
			attachment, _ := invocation.GetAttachment(k)
			return attachment, nil
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
		swSpan.End()
	}()
	return
}
