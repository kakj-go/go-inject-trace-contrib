//go:build goinject

//inject:github.com/segmentio/kafka-go
package kafka

// Ported from go.opentelemetry.io/otelc instrumentation/github.com/segmentio/kafka-go
// (consumer_hook.go + producer_hook.go + semconv/client.go + internal/propagation/
// carrier.go + otelc.yaml rules kafka_reader_readmessage/fetchmessage and
// kafka_writer_writemessages). Consumer spans link to the producer through the
// trace context carried in message headers; ReadMessage's nested FetchMessage
// call is recognized via a context marker and skipped to avoid duplicate spans.

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	hooklog "github.com/kakj-go/go-inject-trace-contrib/otelc/hooklog"
	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcKafkaInstrumentationName = "go.opentelemetry.io/otelc/instrumentation/github.com/segmentio/kafka-go"

//inject:add
const otelcKafkaInstrumentationKey = "KAFKA"

//inject:add
var otelcKafkaTracer trace.Tracer

//inject:add
var otelcKafkaPropagator propagation.TextMapPropagator

//inject:add
var otelcKafkaInitOnce sync.Once

//inject:add
func otelcKafkaInit() {
	otelcKafkaInitOnce.Do(func() {
		otelcKafkaTracer = otel.GetTracerProvider().Tracer(
			otelcKafkaInstrumentationName,
			trace.WithInstrumentationVersion(hooksupport.ModuleVersion()),
		)
		otelcKafkaPropagator = otel.GetTextMapPropagator()
		hooklog.Logger().Info("Kafka (segmentio/kafka-go) consumer instrumentation initialized")
		hooklog.Logger().Info("Kafka (segmentio/kafka-go) producer instrumentation initialized")
	})
}

// ---- header carrier (internal/propagation/carrier.go) ----

// otelcKafkaHeaderCarrier adapts Kafka message headers to the TextMapCarrier
// interface so trace context can be propagated through Kafka messages.
//inject:add
type otelcKafkaHeaderCarrier struct {
	headers *[]Header
}

//inject:add
func (c otelcKafkaHeaderCarrier) Get(key string) string {
	for _, h := range *c.headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

//inject:add
func (c otelcKafkaHeaderCarrier) Set(key, value string) {
	for i := range *c.headers {
		if (*c.headers)[i].Key == key {
			(*c.headers)[i].Value = []byte(value)
			return
		}
	}
	*c.headers = append(*c.headers, Header{Key: key, Value: []byte(value)})
}

//inject:add
func (c otelcKafkaHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(*c.headers))
	for _, h := range *c.headers {
		keys = append(keys, h.Key)
	}
	return keys
}

// ---- semconv (semconv/client.go) ----

//inject:add
const otelcKafkaOperationSend = "send"

//inject:add
const otelcKafkaOperationReceive = "receive"

// messagingKafkaAsyncKey has no OpenTelemetry semantic convention yet; it marks
// producer spans for kafka.Writer.Async writes (see upstream comment).
//inject:add
const otelcKafkaAsyncKey = attribute.Key("messaging.kafka.async")

//inject:add
func otelcKafkaMessageKey(key []byte) string {
	return strings.ToValidUTF8(string(key), "\ufffd")
}

//inject:add
func otelcKafkaRequestTraceAttrs(endpoint, destination, operation, groupID, messageKey string, bodySize int, partition int, offset int64, hasPartition, hasOffset, async bool) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.MessagingSystemKafka,
		semconv.MessagingOperationName(operation),
		semconv.MessagingDestinationName(destination),
	}

	switch operation {
	case otelcKafkaOperationSend:
		attrs = append(attrs, semconv.MessagingOperationTypeSend)
		if async {
			attrs = append(attrs, otelcKafkaAsyncKey.Bool(true))
		}
	case otelcKafkaOperationReceive:
		attrs = append(attrs, semconv.MessagingOperationTypeReceive)
	}

	if endpoint != "" {
		host, portStr, err := net.SplitHostPort(endpoint)
		if err != nil {
			attrs = append(attrs, semconv.ServerAddress(endpoint))
		} else {
			attrs = append(attrs, semconv.ServerAddress(host))
			if port, convErr := strconv.Atoi(portStr); convErr == nil && port > 0 {
				attrs = append(attrs, semconv.ServerPort(port))
			}
		}
	}

	if groupID != "" {
		attrs = append(attrs, semconv.MessagingConsumerGroupName(groupID))
	}
	if messageKey != "" {
		attrs = append(attrs, semconv.MessagingKafkaMessageKey(messageKey))
	}
	if bodySize > 0 {
		attrs = append(attrs, semconv.MessagingMessageBodySize(bodySize))
	}
	if hasPartition {
		attrs = append(attrs, semconv.MessagingDestinationPartitionID(strconv.Itoa(partition)))
	}
	if hasOffset {
		attrs = append(attrs, semconv.MessagingKafkaOffset(int(offset)))
	}

	return attrs
}

// ---- consumer templates ----

// otelcKafkaFetchCallKey marks a context as coming from ReadMessage's own call
// into FetchMessage so the nested hook can skip it.
//inject:add
type otelcKafkaFetchCallKey struct{}

// otelcKafkaRead starts the consumer capture; the deferred half builds the
// consumer span once the original body returns the message.
func (r *Reader) ReadMessage(ctx context.Context) (msg Message, err error) {
	var otelcStart time.Time
	var otelcEndpoint, otelcTopic, otelcGroupID string
	var otelcInstrumented bool
	if hooksupport.Instrumented(otelcKafkaInstrumentationKey) && r != nil {
		otelcKafkaInit()

		if ctx == nil {
			ctx = context.Background()
		}
		ctx = context.WithValue(ctx, otelcKafkaFetchCallKey{}, struct{}{})

		cfg := r.Config()
		if len(cfg.Brokers) > 0 {
			otelcEndpoint = cfg.Brokers[0]
		}
		otelcTopic = cfg.Topic
		otelcGroupID = cfg.GroupID
		otelcStart = time.Now()
		otelcInstrumented = true
	}
	if otelcInstrumented {
		otelcCtx := ctx
		defer func() {
			topic := msg.Topic
			if topic == "" {
				topic = otelcTopic
			}

			parent := otelcCtx
			if parent == nil {
				parent = context.Background()
			}
			parent = otelcKafkaPropagator.Extract(parent, otelcKafkaHeaderCarrier{headers: &msg.Headers})

			_, span := otelcKafkaTracer.Start(parent, topic+" receive",
				trace.WithSpanKind(trace.SpanKindConsumer),
				trace.WithTimestamp(otelcStart),
				trace.WithAttributes(otelcKafkaRequestTraceAttrs(
					otelcEndpoint, topic, otelcKafkaOperationReceive, otelcGroupID,
					otelcKafkaMessageKey(msg.Key), len(msg.Value),
					msg.Partition, msg.Offset, err == nil, err == nil, false,
				)...),
			)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}
	return
}

func (r *Reader) FetchMessage(ctx context.Context) (msg Message, err error) {
	var otelcStart time.Time
	var otelcEndpoint, otelcTopic, otelcGroupID string
	var otelcInstrumented bool
	if ctx != nil && ctx.Value(otelcKafkaFetchCallKey{}) != nil {
		// ReadMessage's own nested FetchMessage call: already instrumented.
	} else if hooksupport.Instrumented(otelcKafkaInstrumentationKey) && r != nil {
		otelcKafkaInit()

		cfg := r.Config()
		if len(cfg.Brokers) > 0 {
			otelcEndpoint = cfg.Brokers[0]
		}
		otelcTopic = cfg.Topic
		otelcGroupID = cfg.GroupID
		otelcStart = time.Now()
		otelcInstrumented = true
	}
	if otelcInstrumented {
		otelcCtx := ctx
		defer func() {
			topic := msg.Topic
			if topic == "" {
				topic = otelcTopic
			}

			parent := otelcCtx
			if parent == nil {
				parent = context.Background()
			}
			parent = otelcKafkaPropagator.Extract(parent, otelcKafkaHeaderCarrier{headers: &msg.Headers})

			_, span := otelcKafkaTracer.Start(parent, topic+" receive",
				trace.WithSpanKind(trace.SpanKindConsumer),
				trace.WithTimestamp(otelcStart),
				trace.WithAttributes(otelcKafkaRequestTraceAttrs(
					otelcEndpoint, topic, otelcKafkaOperationReceive, otelcGroupID,
					otelcKafkaMessageKey(msg.Key), len(msg.Value),
					msg.Partition, msg.Offset, err == nil, err == nil, false,
				)...),
			)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}
	return
}

// ---- producer template ----

// otelcKafkaAsyncCompletionOnce tracks, per *kafka.Writer, the sync.Once that
// guards installing the logging wrapper on Completion.
//inject:add
var otelcKafkaAsyncCompletionOnce sync.Map

//inject:add
func otelcKafkaEnsureAsyncFailureLogging(w *Writer) {
	onceIface, _ := otelcKafkaAsyncCompletionOnce.LoadOrStore(w, new(sync.Once))
	once, ok := onceIface.(*sync.Once)
	if !ok {
		return
	}
	once.Do(func() {
		original := w.Completion
		w.Completion = func(msgs []Message, err error) {
			if err != nil {
				hooklog.Logger().Error("kafka async write failed after WriteMessages returned; the producer span(s) for the affected message(s) were already ended without this outcome",
					"error", err, "messageCount", len(msgs))
			}
			if original != nil {
				original(msgs, err)
			}
		}
	})
}

func (w *Writer) WriteMessages(ctx context.Context, msgs ...Message) (err error) {
	var otelcSpans []trace.Span
	if hooksupport.Instrumented(otelcKafkaInstrumentationKey) && w != nil && len(msgs) > 0 {
		otelcKafkaInit()

		if w.Async {
			otelcKafkaEnsureAsyncFailureLogging(w)
		}

		otelcEndpoint := ""
		if w.Addr != nil {
			otelcEndpoint = w.Addr.String()
		}

		otelcSpans = make([]trace.Span, len(msgs))
		for i := range msgs {
			topic := msgs[i].Topic
			if topic == "" {
				topic = w.Topic
			}
			msgCtx, span := otelcKafkaTracer.Start(ctx, topic+" send",
				trace.WithSpanKind(trace.SpanKindProducer),
				trace.WithAttributes(otelcKafkaRequestTraceAttrs(
					otelcEndpoint, topic, otelcKafkaOperationSend, "",
					otelcKafkaMessageKey(msgs[i].Key), len(msgs[i].Value),
					0, 0, false, false, w.Async,
				)...),
			)
			otelcKafkaPropagator.Inject(msgCtx, otelcKafkaHeaderCarrier{headers: &msgs[i].Headers})
			otelcSpans[i] = span
		}
	}
	if otelcSpans != nil {
		defer func() {
			var writeErrs WriteErrors
			isWriteErrors := errors.As(err, &writeErrs)

			for i, span := range otelcSpans {
				if span == nil {
					continue
				}
				if isWriteErrors {
					if i < len(writeErrs) && writeErrs[i] != nil {
						span.RecordError(writeErrs[i])
						span.SetStatus(codes.Error, writeErrs[i].Error())
					}
				} else if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
				}
				span.End()
			}
		}()
	}
	return
}
