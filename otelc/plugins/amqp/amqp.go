//go:build goinject

//inject:github.com/rabbitmq/amqp091-go
package amqp

// Ported from go.opentelemetry.io/otelc instrumentation/github.com/rabbitmq/amqp091-go
// (hook.go + semconv + internal/propagation/carrier.go + otelc.yaml rules
// amqp_hook_*). Producer spans inject W3C trace context into a copy of
// Publishing.Headers; consumer deliveries get a span whose Acknowledger ends it
// on Ack/Nack/Reject (process spans) or immediately (auto-ack receive spans).
// PublishWithContext* stash the API context because the library drops it before
// delegating to PublishWithDeferredConfirm; without a stashed parent the GLS
// span is used instead.

import (
	"context"
	"errors"
	"strings"
	"sync"

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
const otelcAMQPInstrumentationName = "go.opentelemetry.io/otelc/instrumentation/github.com/rabbitmq/amqp091-go"

//inject:add
const otelcAMQPInstrumentationKey = "AMQP"

//inject:add
var otelcAMQPTracer trace.Tracer

//inject:add
var otelcAMQPPropagator propagation.TextMapPropagator

//inject:add
var otelcAMQPInitOnce sync.Once

//inject:add
func otelcAMQPInit() {
	otelcAMQPInitOnce.Do(func() {
		otelcAMQPTracer = otel.GetTracerProvider().Tracer(
			otelcAMQPInstrumentationName,
			trace.WithInstrumentationVersion(hooksupport.ModuleVersion()),
		)
		otelcAMQPPropagator = otel.GetTextMapPropagator()
		hooklog.Logger().Info("rabbitmq/amqp091-go instrumentation initialized")
	})
}

//inject:add
var otelcAMQPErrDeliveryNotInitialized = errors.New("delivery not initialized")

//inject:add
var otelcAMQPPublishParents sync.Map // *Channel -> context.Context

//inject:add
var otelcAMQPChannelAcks sync.Map // *Channel -> *otelcAMQPPendingAcks

// ---- semconv ----

//inject:add
const otelcAMQPOperationSend = "send"

//inject:add
const otelcAMQPOperationReceive = "receive"

//inject:add
const otelcAMQPOperationProcess = "process"

//inject:add
const otelcAMQPDefaultExchange = "(default)"

//inject:add
func otelcAMQPDestinationName(exchange, routingKey, queue, operation string) string {
	if operation == otelcAMQPOperationReceive || operation == otelcAMQPOperationProcess {
		if q := strings.TrimSpace(queue); q != "" {
			return q
		}
	}
	if ex := strings.TrimSpace(exchange); ex != "" {
		return ex
	}
	return otelcAMQPDefaultExchange
}

//inject:add
func otelcAMQPSpanName(exchange, routingKey, queue, operation string) string {
	return otelcAMQPDestinationName(exchange, routingKey, queue, operation) + " " + operation
}

//inject:add
func otelcAMQPTraceAttrs(exchange, routingKey, queue, operation string, bodySize int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.MessagingSystemRabbitMQ,
		semconv.MessagingOperationName(operation),
		semconv.MessagingDestinationName(otelcAMQPDestinationName(exchange, routingKey, queue, operation)),
	}
	switch operation {
	case otelcAMQPOperationSend:
		attrs = append(attrs, semconv.MessagingOperationTypeSend)
	case otelcAMQPOperationReceive:
		attrs = append(attrs, semconv.MessagingOperationTypeReceive)
	case otelcAMQPOperationProcess:
		attrs = append(attrs, semconv.MessagingOperationTypeProcess)
	}
	if key := strings.TrimSpace(routingKey); key != "" {
		attrs = append(attrs, semconv.MessagingRabbitMQDestinationRoutingKey(key))
	}
	if q := strings.TrimSpace(queue); q != "" && operation != otelcAMQPOperationSend {
		attrs = append(attrs, semconv.MessagingDestinationSubscriptionName(q))
	}
	if bodySize > 0 {
		attrs = append(attrs, semconv.MessagingMessageBodySize(bodySize))
	}
	return attrs
}

//inject:add
func otelcAMQPStartSpan(ctx context.Context, exchange, routingKey, queue, operation string, bodySize int, kind trace.SpanKind) (context.Context, trace.Span) {
	return otelcAMQPTracer.Start(ctx, otelcAMQPSpanName(exchange, routingKey, queue, operation),
		trace.WithSpanKind(kind),
		trace.WithAttributes(otelcAMQPTraceAttrs(exchange, routingKey, queue, operation, bodySize)...),
	)
}

// ---- table carrier ----

//inject:add
type otelcAMQPTableCarrier struct {
	table *Table
}

//inject:add
func (c otelcAMQPTableCarrier) Get(key string) string {
	if c.table == nil || *c.table == nil {
		return ""
	}
	v, ok := (*c.table)[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

//inject:add
func (c otelcAMQPTableCarrier) Set(key, value string) {
	if c.table == nil {
		return
	}
	if *c.table == nil {
		*c.table = Table{}
	}
	(*c.table)[key] = value
}

//inject:add
func (c otelcAMQPTableCarrier) Keys() []string {
	if c.table == nil || *c.table == nil {
		return nil
	}
	keys := make([]string, 0, len(*c.table))
	for k := range *c.table {
		keys = append(keys, k)
	}
	return keys
}

// ---- shared state helpers ----

//inject:add
func otelcAMQPParentContext(ch *Channel) context.Context {
	if ch != nil {
		if v, ok := otelcAMQPPublishParents.LoadAndDelete(ch); ok {
			if ctx, ok := v.(context.Context); ok && ctx != nil {
				if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
					return ctx
				}
			}
		}
	}
	ctx := context.Background()
	if span := hooksupport.GetSpanFromGLS(); span != nil && span.SpanContext().IsValid() {
		return trace.ContextWithSpan(ctx, span)
	}
	return ctx
}

// otelcAMQPCloneTable copies top-level keys only: Inject adds string headers
// and does not mutate nested Table values, so a shallow copy does not alias
// the caller's map.
//
//inject:add
func otelcAMQPCloneTable(t Table) Table {
	out := make(Table, len(t))
	for k, v := range t {
		out[k] = v
	}
	return out
}

//inject:add
func otelcAMQPStashPublishParent(ch *Channel, ctx context.Context) {
	if !hooksupport.Instrumented(otelcAMQPInstrumentationKey) || ch == nil || ctx == nil {
		return
	}
	otelcAMQPPublishParents.Store(ch, ctx)
}

//inject:add
func otelcAMQPAcksFor(ch *Channel) *otelcAMQPPendingAcks {
	actual, _ := otelcAMQPChannelAcks.LoadOrStore(ch, &otelcAMQPPendingAcks{})
	return actual.(*otelcAMQPPendingAcks)
}

//inject:add
func otelcAMQPAcksLookup(ch *Channel) *otelcAMQPPendingAcks {
	if ch == nil {
		return nil
	}
	v, ok := otelcAMQPChannelAcks.Load(ch)
	if !ok {
		return nil
	}
	return v.(*otelcAMQPPendingAcks)
}

// ---- settlement ----

//inject:add
type otelcAMQPPendingAcks struct {
	Mu   sync.Mutex
	Acks map[uint64]*otelcAMQPSettlingAcknowledger
}

//inject:add
func (p *otelcAMQPPendingAcks) Add(tag uint64, s *otelcAMQPSettlingAcknowledger) {
	p.Mu.Lock()
	if p.Acks == nil {
		p.Acks = map[uint64]*otelcAMQPSettlingAcknowledger{}
	}
	p.Acks[tag] = s
	p.Mu.Unlock()
}

//inject:add
func (p *otelcAMQPPendingAcks) Settle(tag uint64, multiple bool, err error) {
	var toEnd []*otelcAMQPSettlingAcknowledger
	p.Mu.Lock()
	if multiple {
		for t, s := range p.Acks {
			if t <= tag {
				toEnd = append(toEnd, s)
				delete(p.Acks, t)
			}
		}
	} else if s, ok := p.Acks[tag]; ok {
		toEnd = append(toEnd, s)
		delete(p.Acks, tag)
	}
	p.Mu.Unlock()
	for _, s := range toEnd {
		s.End(err)
	}
}

//inject:add
func (p *otelcAMQPPendingAcks) EndAll() {
	p.Mu.Lock()
	left := p.Acks
	p.Acks = nil
	p.Mu.Unlock()
	for _, s := range left {
		s.End(nil)
	}
}

//inject:add
type otelcAMQPSettlingAcknowledger struct {
	inner   Acknowledger
	span    trace.Span
	once    sync.Once
	local   *otelcAMQPPendingAcks
	channel *otelcAMQPPendingAcks
}

//inject:add
func (a *otelcAMQPSettlingAcknowledger) Ack(tag uint64, multiple bool) error {
	err := a.callInner(func() error { return a.inner.Ack(tag, multiple) })
	a.finish(tag, multiple, err)
	return err
}

//inject:add
func (a *otelcAMQPSettlingAcknowledger) Nack(tag uint64, multiple, requeue bool) error {
	err := a.callInner(func() error { return a.inner.Nack(tag, multiple, requeue) })
	a.finish(tag, multiple, err)
	return err
}

//inject:add
func (a *otelcAMQPSettlingAcknowledger) Reject(tag uint64, requeue bool) error {
	err := a.callInner(func() error { return a.inner.Reject(tag, requeue) })
	a.finish(tag, false, err)
	return err
}

//inject:add
func (a *otelcAMQPSettlingAcknowledger) callInner(fn func() error) error {
	if a.inner == nil {
		return otelcAMQPErrDeliveryNotInitialized
	}
	return fn()
}

//inject:add
func (a *otelcAMQPSettlingAcknowledger) finish(tag uint64, multiple bool, err error) {
	n := 0
	if a.local != nil {
		a.local.Settle(tag, multiple, err)
		n++
	}
	if a.channel != nil {
		a.channel.Settle(tag, multiple, err)
		n++
	}
	if n == 0 {
		a.End(err)
	}
}

//inject:add
func (a *otelcAMQPSettlingAcknowledger) End(err error) {
	a.once.Do(func() {
		if a.span == nil {
			return
		}
		if err != nil {
			a.span.RecordError(err)
			a.span.SetStatus(codes.Error, err.Error())
		}
		a.span.End()
	})
}

// ---- delivery spans ----

//inject:add
func otelcAMQPWrapDeliveries(ch *Channel, queue string, autoAck bool, in <-chan Delivery) <-chan Delivery {
	// Library Consume channels are unbuffered; the wrapper is unbuffered too,
	// so the consumer still blocks the library the same way.
	out := make(chan Delivery)
	pending := &otelcAMQPPendingAcks{}
	go func() {
		defer close(out)
		defer pending.EndAll()
		for d := range in {
			out <- otelcAMQPStartDeliverySpan(ch, queue, autoAck, d, pending)
		}
	}()
	return out
}

//inject:add
func otelcAMQPStartDeliverySpan(ch *Channel, queue string, autoAck bool, d Delivery, local *otelcAMQPPendingAcks) Delivery {
	op := otelcAMQPOperationProcess
	if autoAck {
		op = otelcAMQPOperationReceive
	}
	parent := otelcAMQPPropagator.Extract(context.Background(), otelcAMQPTableCarrier{table: &d.Headers})
	_, span := otelcAMQPStartSpan(parent, d.Exchange, d.RoutingKey, queue, op, len(d.Body), trace.SpanKindConsumer)
	if autoAck {
		// Auto-ack span ends when the Delivery is read.
		span.End()
		return d
	}
	s := &otelcAMQPSettlingAcknowledger{
		inner:   d.Acknowledger,
		span:    span,
		local:   local,
		channel: nil,
	}
	if ch != nil {
		s.channel = otelcAMQPAcksFor(ch)
	}
	if local != nil {
		local.Add(d.DeliveryTag, s)
	}
	if s.channel != nil {
		s.channel.Add(d.DeliveryTag, s)
	}
	d.Acknowledger = s
	return d
}

// ---- templates ----

func (ch *Channel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg Publishing) error {
	otelcAMQPStashPublishParent(ch, ctx)
	return nil
}

func (ch *Channel) PublishWithDeferredConfirmWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg Publishing) (*DeferredConfirmation, error) {
	otelcAMQPStashPublishParent(ch, ctx)
	return nil, nil
}

func (ch *Channel) PublishWithDeferredConfirm(exchange, key string, mandatory, immediate bool, msg Publishing) (otelcConf *DeferredConfirmation, otelcErr error) {
	var otelcSpan trace.Span
	if hooksupport.Instrumented(otelcAMQPInstrumentationKey) {
		otelcAMQPInit()

		var otelcCtx context.Context
		otelcCtx, otelcSpan = otelcAMQPStartSpan(otelcAMQPParentContext(ch), exchange, key, "", otelcAMQPOperationSend, len(msg.Body), trace.SpanKindProducer)
		msg.Headers = otelcAMQPCloneTable(msg.Headers)
		otelcAMQPPropagator.Inject(otelcCtx, otelcAMQPTableCarrier{table: &msg.Headers})
	}
	if otelcSpan != nil {
		defer func() {
			if otelcErr != nil {
				otelcSpan.RecordError(otelcErr)
				otelcSpan.SetStatus(codes.Error, otelcErr.Error())
			}
			otelcSpan.End()
		}()
	}
	return
}

func (ch *Channel) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args Table) (otelcDeliveries <-chan Delivery, otelcErr error) {
	var otelcStored bool
	if hooksupport.Instrumented(otelcAMQPInstrumentationKey) && ch != nil {
		otelcAMQPInit()
		otelcStored = true
	}
	if otelcStored {
		otelcCh, otelcQueue, otelcAutoAck := ch, queue, autoAck
		defer func() {
			if otelcErr != nil || otelcDeliveries == nil {
				return
			}
			otelcDeliveries = otelcAMQPWrapDeliveries(otelcCh, otelcQueue, otelcAutoAck, otelcDeliveries)
		}()
	}
	return
}

func (ch *Channel) ConsumeWithContext(ctx context.Context, queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args Table) (otelcDeliveries <-chan Delivery, otelcErr error) {
	var otelcStored bool
	if hooksupport.Instrumented(otelcAMQPInstrumentationKey) && ch != nil {
		otelcAMQPInit()
		otelcStored = true
	}
	if otelcStored {
		otelcCh, otelcQueue, otelcAutoAck := ch, queue, autoAck
		defer func() {
			if otelcErr != nil || otelcDeliveries == nil {
				return
			}
			otelcDeliveries = otelcAMQPWrapDeliveries(otelcCh, otelcQueue, otelcAutoAck, otelcDeliveries)
		}()
	}
	return
}

func (ch *Channel) Get(queue string, autoAck bool) (otelcMsg Delivery, otelcOK bool, otelcErr error) {
	var otelcStored bool
	if hooksupport.Instrumented(otelcAMQPInstrumentationKey) && ch != nil {
		otelcAMQPInit()
		otelcStored = true
	}
	if otelcStored {
		otelcCh, otelcQueue, otelcAutoAck := ch, queue, autoAck
		defer func() {
			if otelcErr != nil || !otelcOK {
				return
			}
			otelcMsg = otelcAMQPStartDeliverySpan(otelcCh, otelcQueue, otelcAutoAck, otelcMsg, nil)
		}()
	}
	return
}

func (ch *Channel) Ack(tag uint64, multiple bool) (otelcErr error) {
	var otelcStored bool
	if hooksupport.Instrumented(otelcAMQPInstrumentationKey) {
		otelcStored = true
	}
	if otelcStored {
		otelcCh, otelcTag, otelcMultiple := ch, tag, multiple
		defer func() {
			if p := otelcAMQPAcksLookup(otelcCh); p != nil {
				p.Settle(otelcTag, otelcMultiple, otelcErr)
			}
		}()
	}
	return
}

func (ch *Channel) Nack(tag uint64, multiple, requeue bool) (otelcErr error) {
	var otelcStored bool
	if hooksupport.Instrumented(otelcAMQPInstrumentationKey) {
		otelcStored = true
	}
	if otelcStored {
		otelcCh, otelcTag, otelcMultiple := ch, tag, multiple
		defer func() {
			if p := otelcAMQPAcksLookup(otelcCh); p != nil {
				p.Settle(otelcTag, otelcMultiple, otelcErr)
			}
		}()
	}
	return
}

func (ch *Channel) Reject(tag uint64, requeue bool) (otelcErr error) {
	var otelcStored bool
	if hooksupport.Instrumented(otelcAMQPInstrumentationKey) {
		otelcStored = true
	}
	if otelcStored {
		otelcCh, otelcTag := ch, tag
		defer func() {
			if p := otelcAMQPAcksLookup(otelcCh); p != nil {
				p.Settle(otelcTag, false, otelcErr)
			}
		}()
	}
	return
}
