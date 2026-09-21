//go:build goinject

//inject:github.com/apache/rocketmq-client-go/v2/consumer
package rocketmq

import (
	"context"
	"fmt"
	"strings"

	"github.com/apache/rocketmq-client-go/v2/primitive"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

//inject:add
const (
	swRMQConsumerComponentID   = 39
	swRMQConsumerPrefix        = "RocketMQ/"
	swRMQConsumerSuffix        = "/Consumer"
	swConsumerTagMQMsgID       = "mq.msg.id"
	swConsumerTagMQOffsetMsgID = "mq.offset.msg.id"
	swRMQConsumerSemicolon     = ";"
)

type pushConsumer struct {
}

func (pc *pushConsumer) consumeInner(ctx context.Context, subMsgs []*primitive.MessageExt) (result ConsumeResult, err error) {
	swSpan := tracing.Span(nil)
	{
		peer := strings.Join(pc.client.GetNameSrv().AddrList(), swRMQConsumerSemicolon)
		s, swErr := swCreateConsumerEntrySpan(subMsgs, peer)
		if swErr == nil {
			swSpan = s
		}
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.Tag(tracing.TagMQStatus, swConsumerStatusStr(result))
		if ConsumeSuccess != result {
			swSpan.Error()
		}
		swSpan.End()
	}()
	return
}

// swCreateConsumerEntrySpan creates ONE entry span for the whole batch from the
// first message and attaches every remaining message as an extra segment
// reference, mirroring the Java agent. One span per message must be avoided:
// the reuse rule would hand back the same span N times while the deferred
// AfterInvoke logic calls End only once, so the span would never be reported.
//
//inject:add
func swCreateConsumerEntrySpan(subMsgs []*primitive.MessageExt, peer string) (tracing.Span, error) {
	if len(subMsgs) == 0 {
		return nil, nil
	}
	first := subMsgs[0]
	topic := first.Topic
	msgIDs := make([]string, 0, len(subMsgs))
	offsetMsgIDs := make([]string, 0, len(subMsgs))
	for _, msg := range subMsgs {
		msgIDs = append(msgIDs, msg.MsgId)
		offsetMsgIDs = append(offsetMsgIDs, msg.OffsetMsgId)
	}

	span, err := tracing.CreateEntrySpan(swRMQConsumerPrefix+topic+swRMQConsumerSuffix, func(headerKey string) (string, error) {
		return first.GetProperty(headerKey), nil
	},
		tracing.WithLayer(tracing.SpanLayerMQ),
		tracing.WithComponent(swRMQConsumerComponentID),
		tracing.WithTag(tracing.TagMQTopic, topic),
		tracing.WithTag(swConsumerTagMQMsgID, strings.Join(msgIDs, swRMQConsumerSemicolon)),
		tracing.WithTag(swConsumerTagMQOffsetMsgID, strings.Join(offsetMsgIDs, swRMQConsumerSemicolon)),
	)
	if err != nil {
		return nil, err
	}
	for _, msg := range subMsgs[1:] {
		extractMsg := msg
		// a broken header on a single message must not lose the batch span,
		// so the error is intentionally ignored
		_ = tracing.ExtractContext(func(headerKey string) (string, error) {
			return extractMsg.GetProperty(headerKey), nil
		})
	}
	span.Tag(tracing.TagMQBroker, first.StoreHost)
	span.SetPeer(peer)
	return span, nil
}

//inject:add
func swConsumerStatusStr(status ConsumeResult) string {
	switch status {
	case ConsumeSuccess:
		return "ConsumeSuccess"
	case ConsumeRetryLater:
		return "ConsumeRetryLater"
	case Commit:
		return "Commit"
	case Rollback:
		return "Rollback"
	case SuspendCurrentQueueAMoment:
		return "SuspendCurrentQueueAMoment"
	default:
		return fmt.Sprintf("%d", status)
	}
}
