//go:build goinject

//inject:github.com/apache/pulsar-client-go/pulsar
package pulsar

import (
	"context"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

//inject:add
const (
	swPulsarSyncPrefix      = "Pulsar/"
	swPulsarSyncSuffix      = "/Producer"
	swPulsarSyncComponentID = 73

	swPulsarAsyncPrefix      = "Pulsar/"
	swPulsarAsyncSuffix      = "/AsyncProducer"
	swPulsarCallbackSuffix   = "/Producer/Callback"
	swPulsarAsyncComponentID = 73

	swPulsarReceivePrefix      = "Pulsar/"
	swPulsarReceiveSuffix      = "/Consumer"
	swPulsarReceiveComponentID = 74
)

type partitionProducer struct {
}

func (p *partitionProducer) Send(ctx context.Context, msg *ProducerMessage) (msgID MessageID, err error) {
	swSpan := tracing.Span(nil)
	{
		topic := p.options.Topic
		lookup, swLookupErr := p.client.lookupService.Lookup(topic)
		if swLookupErr == nil {
			peer := lookup.PhysicalAddr.String()
			operationName := swPulsarSyncPrefix + topic + swPulsarSyncSuffix

			s, swErr := tracing.CreateExitSpan(operationName, peer, func(headerKey, headerValue string) error {
				if msg.Properties == nil {
					msg.Properties = map[string]string{
						headerKey: headerValue,
					}
					return nil
				}
				msg.Properties[headerKey] = headerValue
				return nil
			},
				tracing.WithLayer(tracing.SpanLayerMQ),
				tracing.WithComponent(swPulsarSyncComponentID),
				tracing.WithTag(tracing.TagMQBroker, lookup.PhysicalAddr.String()),
				tracing.WithTag(tracing.TagMQTopic, p.topic),
			)
			if swErr == nil {
				swSpan = s
			}
		}
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		if msgID != nil {
			swSpan.Tag(tracing.TagMQMsgID, msgID.String())
		}
		swSpan.End()
	}()
	return
}

func (p *partitionProducer) SendAsync(ctx context.Context, msg *ProducerMessage, callback func(MessageID, *ProducerMessage, error)) {
	swSpan := tracing.Span(nil)
	{
		topic := p.options.Topic
		lookup, swLookupErr := p.client.lookupService.Lookup(topic)
		if swLookupErr == nil {
			peer := lookup.PhysicalAddr.String()
			operationName := swPulsarAsyncPrefix + topic + swPulsarAsyncSuffix

			s, swErr := tracing.CreateExitSpan(operationName, peer, func(headerKey, headerValue string) error {
				if msg.Properties == nil {
					msg.Properties = map[string]string{
						headerKey: headerValue,
					}
					return nil
				}
				msg.Properties[headerKey] = headerValue
				return nil
			},
				tracing.WithLayer(tracing.SpanLayerMQ),
				tracing.WithComponent(swPulsarAsyncComponentID),
				tracing.WithTag(tracing.TagMQBroker, lookup.PhysicalAddr.String()),
				tracing.WithTag(tracing.TagMQTopic, p.topic),
			)
			if swErr == nil {
				swSpan = s

				swContinueSnapShot := tracing.CaptureContext()
				zuper := callback

				// enhance async callback method: the agent part is fully isolated
				// inside swTraceAsyncSendCallback (see its doc), the user callback
				// runs after it
				callbackFunc := func(id MessageID, message *ProducerMessage, err error) {
					swTraceAsyncSendCallback(swContinueSnapShot, topic, p.topic, peer, lookup.PhysicalAddr.String(), id, err)
					zuper(id, message, err)
				}

				s.SetPeer(peer)
				callback = callbackFunc
			}
		}
	}
	defer func() {
		if swSpan == nil {
			return
		}
		swSpan.End()
	}()
}

type consumer struct {
}

func (c *consumer) Receive(ctx context.Context) (message Message, err error) {
	defer func() {
		// mirror the official AfterInvoke recover: a nil message (consumer
		// closed / ctx timeout) must not panic the SDK goroutine
		defer func() { _ = recover() }()
		topic := c.options.Topic
		lookup, swLookupErr := c.client.lookupService.Lookup(topic)
		if swLookupErr != nil {
			return
		}
		peer := lookup.PhysicalAddr.String()
		operationName := swPulsarReceivePrefix + topic + swPulsarReceiveSuffix

		span, swErr := tracing.CreateEntrySpan(operationName, func(headerKey string) (string, error) {
			if message == nil {
				return "", nil
			}
			return message.Properties()[headerKey], nil
		},
			tracing.WithLayer(tracing.SpanLayerMQ),
			tracing.WithComponent(swPulsarReceiveComponentID),
			tracing.WithTag(tracing.TagMQBroker, lookup.PhysicalAddr.String()),
			tracing.WithTag(tracing.TagMQTopic, c.topic),
		)
		if swErr != nil {
			return
		}

		if mqErr, ok := err.(*Error); ok {
			span.Tag(tracing.TagMQStatus, mqErr.Error())
			span.Error(mqErr.Error())
		}
		span.SetPeer(peer)
		span.End()
	}()
	return
}

// swTraceAsyncSendCallback records the async send result on a NEW local span -
// never on the exit span, already ended when SendAsync returned. It runs on an
// SDK goroutine without framework recover, so the agent logic is fully wrapped
// in its own recover; the user callback runs outside, never swallowed.
//
//inject:add
func swTraceAsyncSendCallback(snapshot tracing.ContextSnapshot, opTopic, tagTopic, peer, broker string, id MessageID, sendErr error) {
	defer tracing.CleanContext()
	defer func() {
		// no logging channel exists on this goroutine, drop on purpose
		_ = recover()
	}()
	tracing.ContinueContext(snapshot)

	localSpan, err := tracing.CreateLocalSpan(swPulsarAsyncPrefix+opTopic+swPulsarCallbackSuffix,
		tracing.WithComponent(swPulsarAsyncComponentID),
		tracing.WithLayer(tracing.SpanLayerMQ),
		tracing.WithTag(tracing.TagMQTopic, tagTopic),
	)
	if err != nil {
		return
	}
	if sendErr != nil {
		localSpan.Error(sendErr.Error())
	}
	localSpan.Tag(tracing.TagMQBroker, broker)
	if id != nil { // nil when the send failed
		localSpan.Tag(tracing.TagMQMsgID, id.String())
	}
	localSpan.SetPeer(peer)
	localSpan.End()
}
