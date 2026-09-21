//go:build goinject

//inject:github.com/segmentio/kafka-go
package segmentiokafka

import (
	"context"
	"strings"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

//inject:add
const (
	swKafkaWriterPrefix      = "Kafka/"
	swKafkaWriterSuffix      = "/Producer"
	swKafkaWriterComponentID = 40

	swKafkaReaderPrefix      = "Kafka/"
	swKafkaReaderSuffix      = "/Consumer"
	swKafkaReaderComponentID = 41

	swKafkaSemicolon = ";"
)

//inject:add
var swInternalReporterContextKey = context.Background()

type Writer struct {
}

func (w *Writer) WriteMessages(ctx context.Context, msgs ...Message) (err error) {
	swSpan := tracing.Span(nil)
	addr, topic := w.Addr.String(), w.Topic
	// the agent internal reporting traffic must not be traced, which mirrors
	// the early return of the official BeforeInvoke
	if internal, ok := ctx.Value(swInternalReporterContextKey).(bool); !ok || !internal {
		messageList := msgs
		operationName := swKafkaWriterPrefix + topic + swKafkaWriterSuffix

		s, swErr := tracing.CreateExitSpan(operationName, addr, func(headerKey, headerValue string) error {
			for idx := range messageList {
				if len(messageList[idx].Headers) == 0 {
					messageList[idx].Headers = []Header{
						{Key: headerKey, Value: []byte(headerValue)},
					}
				} else {
					messageList[idx].Headers = append(messageList[idx].Headers,
						Header{Key: headerKey, Value: []byte(headerValue)})
				}
			}
			return nil
		},
			tracing.WithLayer(tracing.SpanLayerMQ),
			tracing.WithComponent(swKafkaWriterComponentID),
			tracing.WithTag(tracing.TagMQBroker, addr),
			tracing.WithTag(tracing.TagMQTopic, topic),
		)
		if swErr == nil {
			swSpan = s
			swSpan.SetPeer(addr)
		}
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Tag(tracing.TagMQStatus, err.Error())
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

type Reader struct {
}

func (r *Reader) ReadMessage(ctx context.Context) (msg Message, err error) {
	defer func() {
		brokers := strings.Join(r.Config().Brokers, swKafkaSemicolon)
		topic := msg.Topic
		operationName := swKafkaReaderPrefix + topic + swKafkaReaderSuffix

		span, spanErr := tracing.CreateEntrySpan(operationName, func(headerKey string) (string, error) {
			for _, header := range msg.Headers {
				if header.Key == headerKey {
					return string(header.Value), nil
				}
			}
			return "", nil
		},
			tracing.WithLayer(tracing.SpanLayerMQ),
			tracing.WithComponent(swKafkaReaderComponentID),
			tracing.WithTag(tracing.TagMQBroker, brokers),
			tracing.WithTag(tracing.TagMQTopic, topic),
		)
		if spanErr != nil {
			return
		}

		if err != nil {
			span.Tag(tracing.TagMQStatus, err.Error())
			span.Error(err.Error())
		}
		span.SetPeer(brokers)
		span.End()
	}()
	return
}
