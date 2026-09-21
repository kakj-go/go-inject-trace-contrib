//go:build goinject

//inject:github.com/apache/rocketmq-client-go/v2/producer
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
	swRMQSyncSendPrefix    = "RocketMQ/"
	swRMQSyncSuffix        = "/Producer"
	swRMQSyncComponentID   = 38
	swRMQSyncSemicolon     = ";"
	swSyncTagMQOffsetMsgID = "mq.offset.msg.id"

	swRMQASyncSendPrefix    = "RocketMQ/"
	swRMQASyncSuffix        = "/AsyncProducer"
	swRMQCallbackSuffix     = "/Producer/Callback"
	swRMQASyncComponentID   = 38
	swASyncSemicolon        = ";"
	swASyncTagMQOffsetMsgID = "mq.offset.msg.id"
)

type defaultProducer struct {
}

// swGeneralProducerBeforeInvoke mirrors GeneralProducerBeforeInvoke of the
// official plugin: the exit span is created from the name server address list
// and the sw8 headers are attached to every message as properties.
//
//inject:add
func swGeneralProducerBeforeInvoke(p *defaultProducer, msgList []*primitive.Message) tracing.Span {
	peer := strings.Join(p.client.GetNameSrv().AddrList(), swRMQSyncSemicolon)
	topic := msgList[0].Topic
	operationName := swRMQSyncSendPrefix + topic + swRMQSyncSuffix

	span, err := tracing.CreateExitSpan(operationName, peer, func(headerKey, headerValue string) error {
		for _, message := range msgList {
			message.WithProperty(headerKey, headerValue)
		}
		return nil
	},
		tracing.WithLayer(tracing.SpanLayerMQ),
		tracing.WithComponent(swRMQSyncComponentID),
		tracing.WithTag(tracing.TagMQTopic, topic),
	)
	if err != nil {
		return nil
	}
	return span
}

func (p *defaultProducer) SendSync(ctx context.Context, msgs ...*primitive.Message) (result *primitive.SendResult, err error) {
	swSpan := swGeneralProducerBeforeInvoke(p, msgs)
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		if result != nil {
			swSpan.Tag(tracing.TagMQStatus, swSendStatusStr(result.Status))
			swSpan.Tag(tracing.TagMQQueue, fmt.Sprintf("%d", result.MessageQueue.QueueId))
			swSpan.Tag(tracing.TagMQBroker, p.client.GetNameSrv().
				FindBrokerAddrByName(result.MessageQueue.BrokerName))
			swSpan.Tag(tracing.TagMQMsgID, result.MsgID)
			swSpan.Tag(swSyncTagMQOffsetMsgID, result.OffsetMsgID)
		}
		swSpan.End()
	}()
	return
}

func (p *defaultProducer) SendOneWay(ctx context.Context, msgs ...*primitive.Message) (err error) {
	swSpan := swGeneralProducerBeforeInvoke(p, msgs)
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

func (p *defaultProducer) SendAsync(ctx context.Context, callback func(context.Context, *primitive.SendResult, error), msgs ...*primitive.Message) (err error) {
	swSpan := tracing.Span(nil)
	{
		peer := strings.Join(p.client.GetNameSrv().AddrList(), swASyncSemicolon)
		msgList := msgs
		topic := msgList[0].Topic
		operationName := swRMQASyncSendPrefix + topic + swRMQASyncSuffix

		s, swErr := tracing.CreateExitSpan(operationName, peer, func(headerKey, headerValue string) error {
			for _, message := range msgList {
				message.WithProperty(headerKey, headerValue)
			}
			return nil
		},
			tracing.WithLayer(tracing.SpanLayerMQ),
			tracing.WithComponent(swRMQASyncComponentID),
			tracing.WithTag(tracing.TagMQTopic, topic),
		)
		if swErr == nil {
			swSpan = s

			swContinueSnapShot := tracing.CaptureContext()
			zuper := callback
			// enhance async callback method: the agent part is fully isolated inside
			// swTraceAsyncSendCallback (see its doc), the user callback runs after it
			callbackFunc := func(ctx context.Context, sendResult *primitive.SendResult, err error) {
				swTraceAsyncSendCallback(swContinueSnapShot, topic, peer, sendResult, err, func(brokerName string) string {
					return p.client.GetNameSrv().FindBrokerAddrByName(brokerName)
				})
				zuper(ctx, sendResult, err)
			}

			s.SetPeer(peer)
			callback = callbackFunc
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

// swTraceAsyncSendCallback records the async send result on a NEW local span -
// never on the exit span, already ended when SendAsync returned. It runs on an
// SDK goroutine without framework recover, so the agent logic is fully wrapped
// in its own recover; the user callback runs outside, never swallowed.
//
//inject:add
func swTraceAsyncSendCallback(snapshot tracing.ContextSnapshot, topic, peer string,
	sendResult *primitive.SendResult, sendErr error, brokerAddr func(brokerName string) string) {
	defer tracing.CleanContext()
	defer func() {
		// no logging channel exists on this goroutine, drop on purpose
		_ = recover()
	}()
	tracing.ContinueContext(snapshot)

	localSpan, err := tracing.CreateLocalSpan(swRMQASyncSendPrefix+topic+swRMQCallbackSuffix,
		tracing.WithComponent(swRMQASyncComponentID),
		tracing.WithLayer(tracing.SpanLayerMQ),
		tracing.WithTag(tracing.TagMQTopic, topic),
	)
	if err != nil {
		return
	}
	if sendErr != nil {
		localSpan.Error(sendErr.Error())
	}
	if sendResult != nil { // nil when the send failed
		localSpan.Tag(tracing.TagMQStatus, swSendStatusStr(sendResult.Status))
		if sendResult.MessageQueue != nil {
			localSpan.Tag(tracing.TagMQQueue, fmt.Sprintf("%d", sendResult.MessageQueue.QueueId))
			localSpan.Tag(tracing.TagMQBroker, brokerAddr(sendResult.MessageQueue.BrokerName))
		}
		localSpan.Tag(tracing.TagMQMsgID, sendResult.MsgID)
		localSpan.Tag(swASyncTagMQOffsetMsgID, sendResult.OffsetMsgID)
	}
	localSpan.SetPeer(peer)
	localSpan.End()
}

//inject:add
func swSendStatusStr(status primitive.SendStatus) string {
	switch status {
	case primitive.SendOK:
		return "SendOK"
	case primitive.SendFlushDiskTimeout:
		return "SendFlushDiskTimeout"
	case primitive.SendFlushSlaveTimeout:
		return "SendFlushSlaveTimeout"
	case primitive.SendSlaveNotAvailable:
		return "SendSlaveNotAvailable"
	case primitive.SendUnknownError:
		return "SendUnknownError"
	default:
		return fmt.Sprintf("%d", status)
	}
}
