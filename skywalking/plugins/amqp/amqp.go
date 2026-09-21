//go:build goinject

//inject:github.com/rabbitmq/amqp091-go
package amqp

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

//inject:add
const (
	swProducerComponentID = 144
	swConsumerComponentID = 145

	swAMQPSendPrefix     = "AMQP"
	swAMQPSendSuffix     = "/Producer"
	swAMQPSendDelimiter  = "/"
	swAMQPConsumerPrefix = "AMQP/"
	swAMQPConsumerSuffix = "/Consumer"

	swTagMQExchange      = "mq.exchange"
	swTagMQRoutingKey    = "mq.routing_key"
	swTagMQConsumerTag   = "mq.consumer_tag"
	swTagMQReplyTo       = "mq.reply_to"
	swTagMQCorrelationID = "mq.correlation_id"
	swTagMQArgs          = "mq.args"

	swConsumerTagLengthMax = 0xFF
)

//inject:add
var swConsumerSeq uint64

// swQueueConsumerTagMapping is touched from three goroutines (Consume writes,
// the SDK delivery dispatch reads, Close deletes) - unsynchronized access is
// a fatal concurrent map read/write, so it only goes through the accessors.
//
//inject:add
var (
	swQueueConsumerTagMapping = make(map[string]string)
	swQueueConsumerTagLock    sync.RWMutex
)

//inject:add
func swRegisterConsumerQueue(consumerTag, queue string) {
	swQueueConsumerTagLock.Lock()
	defer swQueueConsumerTagLock.Unlock()
	swQueueConsumerTagMapping[consumerTag] = queue
}

//inject:add
func swConsumerQueue(consumerTag string) string {
	swQueueConsumerTagLock.RLock()
	defer swQueueConsumerTagLock.RUnlock()
	return swQueueConsumerTagMapping[consumerTag]
}

//inject:add
func swRemoveConsumerQueue(consumerTag string) {
	swQueueConsumerTagLock.Lock()
	defer swQueueConsumerTagLock.Unlock()
	delete(swQueueConsumerTagMapping, consumerTag)
}

//inject:add
func swUniqueConsumerTag() string {
	return swCommandNameBasedUniqueConsumerTag(os.Args[0])
}

//inject:add
func swCommandNameBasedUniqueConsumerTag(commandName string) string {
	tagPrefix := "ctag-"
	tagInfix := commandName
	tagSuffix := "-" + strconv.FormatUint(atomic.AddUint64(&swConsumerSeq, 1), 10)

	if len(tagPrefix)+len(tagInfix)+len(tagSuffix) > swConsumerTagLengthMax {
		tagInfix = "streadway/amqp"
	}

	return tagPrefix + tagInfix + tagSuffix
}

// swParseURI keeps only the "host:port" of the dialed uri, stored on the
// Connection as the peer of every span created from its channels.
//
//inject:add
func swParseURI(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s:%s", u.Hostname(), u.Port())
}

// swGetPeerInfo reads the peer address stored by the DialConfig
// AfterInvoke logic, mirroring getPeerInfo of the official plugin.
//
//inject:add
func swGetPeerInfo(conn *Connection) string {
	return conn.swSkywalkingData.(string)
}

// Connection carries the peer address ("host:port") of the broker it dialed,
// replacing the SkyWalkingDynamicField of the official struct enhancement.
type Connection struct {
	//inject:add
	swSkywalkingData interface{}
}

func DialConfig(url string, config Config) (connection *Connection, err error) {
	defer func() {
		if connection != nil {
			connection.swSkywalkingData = swParseURI(url)
		}
	}()
	return
}

type Channel struct {
}

func (ch *Channel) PublishWithDeferredConfirmWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg Publishing) (confirmation *DeferredConfirmation, err error) {
	swSpan := tracing.Span(nil)
	{
		peer := swGetPeerInfo(ch.connection)
		routingKey := key
		operationName := swAMQPSendPrefix
		if exchange != "" {
			operationName += swAMQPSendDelimiter + exchange
		}
		if routingKey != "" {
			operationName += swAMQPSendDelimiter + routingKey
		}
		publishing := msg
		operationName += swAMQPSendSuffix

		s, swErr := tracing.CreateExitSpan(operationName, peer, func(headerKey, headerValue string) error {
			if publishing.Headers == nil {
				publishing.Headers = Table{
					headerKey: headerValue,
				}
				return nil
			}
			publishing.Headers[headerKey] = headerValue
			return nil
		}, tracing.WithLayer(tracing.SpanLayerMQ),
			tracing.WithComponent(swProducerComponentID),
			tracing.WithTag(tracing.TagMQBroker, peer),
			tracing.WithTag(swTagMQExchange, exchange),
			tracing.WithTag(swTagMQRoutingKey, routingKey),
		)
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
		swSpan.End()
	}()
	return
}

func (ch *Channel) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args Table) (deliveries <-chan Delivery, err error) {
	{
		consumerTag := consumer
		if consumerTag == "" {
			consumerTag = swUniqueConsumerTag()
		}
		swRegisterConsumerQueue(consumerTag, queue)
	}
	return
}

type consumers struct {
}

func (subs *consumers) send(tag string, msg *Delivery) (foundConsumer bool) {
	defer func() {
		if !foundConsumer {
			return
		}
		consumerTag := tag
		delivery := msg
		queue := swConsumerQueue(consumerTag)
		operationName := swAMQPConsumerPrefix + queue + "/" + consumerTag + swAMQPConsumerSuffix
		channel, _ := delivery.Acknowledger.(*Channel)
		peer := swGetPeerInfo(channel.connection)

		span, swErr := tracing.CreateEntrySpan(operationName, func(headerKey string) (string, error) {
			header, _ := delivery.Headers[headerKey].(string)
			return header, nil
		}, tracing.WithLayer(tracing.SpanLayerMQ),
			tracing.WithComponent(swConsumerComponentID),
			tracing.WithTag(tracing.TagMQBroker, peer),
			tracing.WithTag(tracing.TagMQQueue, queue),
			tracing.WithTag(tracing.TagMQMsgID, delivery.MessageId),
			tracing.WithTag(swTagMQConsumerTag, consumerTag),
			tracing.WithTag(swTagMQCorrelationID, delivery.CorrelationId),
			tracing.WithTag(swTagMQReplyTo, delivery.ReplyTo),
			tracing.WithTag(swTagMQArgs, fmt.Sprintf("%v", delivery.Headers)),
		)
		if swErr != nil {
			return
		}
		span.SetPeer(peer)
		span.End()
	}()
	return
}

func (subs *consumers) close() {
	// use an immediate closure so the lock is released before the original
	// body's own subs.Lock() — a plain defer here would deadlock since the
	// template defer wraps the entire woven function including the original body
	func() {
		subs.Lock()
		defer subs.Unlock()
		for consumerTag := range subs.chans {
			swRemoveConsumerQueue(consumerTag)
		}
	}()
}
