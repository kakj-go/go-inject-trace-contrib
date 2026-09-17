//go:build goinject

//inject:github.com/redis/go-redis/v9
package redis

// Ported from go.opentelemetry.io/otelc instrumentation/github.com/redis/go-redis/v9
// (client_hook.go + hook.go + semconv/client.go + otelc.yaml rules redis_*).
// Constructor after-hooks register a go-redis Hook that wraps every command
// (and pipeline) in a CLIENT span; the span body lives in ProcessHook.

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	hooklog "github.com/kakj-go/go-inject-trace-contrib/otelc/hooklog"
	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcRedisInstrumentationName = "go.opentelemetry.io/otelc/instrumentation/github.com/redis/go-redis/v9"

//inject:add
const otelcRedisInstrumentationKey = "REDIS"

//inject:add
var otelcRedisTracer trace.Tracer

//inject:add
var otelcRedisInitOnce sync.Once

//inject:add
func otelcRedisInit() {
	otelcRedisInitOnce.Do(func() {
		otelcRedisTracer = otel.GetTracerProvider().Tracer(
			otelcRedisInstrumentationName,
			trace.WithInstrumentationVersion(hooksupport.ModuleVersion()),
		)
		hooklog.Logger().Info("Redis v9 client instrumentation initialized")
	})
}

//inject:add
const otelcRedisAuthCmd = "auth"

//inject:add
const otelcRedisHelloCmd = "hello"

//inject:add
const otelcRedisSetNameOption = "setname"

//inject:add
const otelcRedisHelloAuthArgN = 2

//inject:add
const otelcRedisQueryTextRedact = "?"

// ---- semconv ----

//inject:add
func otelcRedisRequestTraceAttrs(endpoint, fullName, statement string) []attribute.KeyValue {
	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}

	attrs := []attribute.KeyValue{
		semconv.DBSystemNameRedis,
		semconv.DBOperationName(fullName),
		semconv.ServerAddress(host),
		semconv.NetworkTransportTCP,
		semconv.DBQueryText(statement),
	}

	if err == nil {
		if port, convErr := strconv.Atoi(portStr); convErr == nil && port > 0 {
			attrs = append(attrs, semconv.ServerPort(port))
		}
	}

	return attrs
}

// ---- hook (hook.go) ----

//inject:add
type otelRedisHook struct {
	Addr string
}

//inject:add
func otelcNewRedisHook(addr string) *otelRedisHook {
	return &otelRedisHook{
		Addr: addr,
	}
}

//inject:add
func (o *otelRedisHook) ProcessHook(next ProcessHook) ProcessHook {
	return func(ctx context.Context, cmd Cmder) error {
		if !hooksupport.Instrumented(otelcRedisInstrumentationKey) {
			return next(ctx, cmd)
		}
		otelcRedisInit()
		fullName := cmd.FullName()
		statement := otelcRedisStatement(cmd)

		attrs := otelcRedisRequestTraceAttrs(o.Addr, fullName, statement)

		spanName := fullName
		ctx, span := otelcRedisTracer.Start(ctx,
			spanName,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attrs...),
		)
		defer span.End()

		err := next(ctx, cmd)
		if err != nil && !errors.Is(err, Nil) {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return err
	}
}

//inject:add
func (o *otelRedisHook) ProcessPipelineHook(next ProcessPipelineHook) ProcessPipelineHook {
	return func(ctx context.Context, cmds []Cmder) error {
		if !hooksupport.Instrumented(otelcRedisInstrumentationKey) {
			return next(ctx, cmds)
		}
		otelcRedisInit()

		summary := ""
		summaryCmds := cmds
		if len(summaryCmds) > 10 {
			summaryCmds = summaryCmds[:10]
		}
		for i := range summaryCmds {
			summary += summaryCmds[i].FullName() + "/"
		}
		if len(cmds) > 10 {
			summary += "..."
		}
		cmd := NewCmd(ctx, "pipeline", summary)
		fullName := cmd.FullName()
		statement := otelcRedisStatement(cmd)

		attrs := otelcRedisRequestTraceAttrs(o.Addr, fullName, statement)

		spanName := fullName
		ctx, span := otelcRedisTracer.Start(ctx,
			spanName,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attrs...),
		)
		defer span.End()

		err := next(ctx, cmds)
		if err != nil && !errors.Is(err, Nil) {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return err
	}
}

//inject:add
func (o *otelRedisHook) DialHook(next DialHook) DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := next(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return conn, err
	}
}

//inject:add
func otelcRedisStatement(cmd Cmder) string {
	args := cmd.Args()
	redactStart, redactEnd := otelcRedisCredentialRedactRange(cmd.Name(), args)

	b := make([]byte, 0, 64)
	for i, arg := range args {
		if i > 0 {
			b = append(b, ' ')
		}
		if i >= redactStart && i < redactEnd {
			b = append(b, otelcRedisQueryTextRedact...)
			continue
		}
		b = otelcRedisAppendArg(b, arg)
	}

	if err := cmd.Err(); err != nil && !errors.Is(err, Nil) {
		b = append(b, ": "...)
		b = append(b, err.Error()...)
	}

	return string(b)
}

//inject:add
func otelcRedisCredentialRedactRange(name string, args []interface{}) (start, end int) {
	switch name {
	case otelcRedisAuthCmd:
		if len(args) > 1 {
			return 1, len(args)
		}
	case otelcRedisHelloCmd:
		if i := otelcRedisHelloAuthIndex(args); i >= 0 {
			return i + 1, min(i+1+otelcRedisHelloAuthArgN, len(args))
		}
	}
	return 0, 0
}

//inject:add
func otelcRedisHelloAuthIndex(args []interface{}) int {
	for i := 1; i < len(args); i++ {
		if !otelcRedisArgEqualFold(args[i], otelcRedisAuthCmd) {
			continue
		}
		if i > 1 && otelcRedisArgEqualFold(args[i-1], otelcRedisSetNameOption) {
			continue
		}
		return i
	}
	return -1
}

//inject:add
func otelcRedisArgEqualFold(v interface{}, s string) bool {
	switch a := v.(type) {
	case string:
		return strings.EqualFold(a, s)
	case []byte:
		return strings.EqualFold(string(a), s)
	default:
		return false
	}
}

//inject:add
func otelcRedisAppendArg(b []byte, v interface{}) []byte {
	switch v := v.(type) {
	case nil:
		return append(b, "<nil>"...)
	case string:
		if utf8.ValidString(v) {
			return append(b, v...)
		}
		return append(b, "<string>"...)
	case []byte:
		if utf8.Valid(v) {
			return append(b, v...)
		}
		return append(b, "<byte>"...)
	case int:
		return strconv.AppendInt(b, int64(v), 10)
	case int8:
		return strconv.AppendInt(b, int64(v), 10)
	case int16:
		return strconv.AppendInt(b, int64(v), 10)
	case int32:
		return strconv.AppendInt(b, int64(v), 10)
	case int64:
		return strconv.AppendInt(b, v, 10)
	case uint:
		return strconv.AppendUint(b, uint64(v), 10)
	case uint8:
		return strconv.AppendUint(b, uint64(v), 10)
	case uint16:
		return strconv.AppendUint(b, uint64(v), 10)
	case uint32:
		return strconv.AppendUint(b, uint64(v), 10)
	case uint64:
		return strconv.AppendUint(b, v, 10)
	case float32:
		return strconv.AppendFloat(b, float64(v), 'f', -1, 64)
	case float64:
		return strconv.AppendFloat(b, v, 'f', -1, 64)
	case bool:
		if v {
			return append(b, "true"...)
		}
		return append(b, "false"...)
	case time.Time:
		return v.AppendFormat(b, time.RFC3339Nano)
	default:
		return append(b, "not_support_type"...)
	}
}

// ---- constructor templates ----

func NewClient(opt *Options) (otelcClient *Client) {
	defer func() {
		if otelcClient != nil {
			otelcClient.AddHook(otelcNewRedisHook(otelcClient.Options().Addr))
		}
	}()
	return
}

func NewFailoverClient(failoverOpt *FailoverOptions) (otelcClient *Client) {
	defer func() {
		if otelcClient != nil {
			otelcClient.AddHook(otelcNewRedisHook(otelcClient.Options().Addr))
		}
	}()
	return
}

func NewRing(opt *RingOptions) (otelcClient *Ring) {
	defer func() {
		if otelcClient != nil {
			otelcClient.OnNewNode(func(rdb *Client) {
				rdb.AddHook(otelcNewRedisHook(rdb.Options().Addr))
			})
		}
	}()
	return
}

func NewClusterClient(opt *ClusterOptions) (otelcClient *ClusterClient) {
	defer func() {
		if otelcClient != nil {
			otelcClient.OnNewNode(func(rdb *Client) {
				rdb.AddHook(otelcNewRedisHook(rdb.Options().Addr))
			})
		}
	}()
	return
}

func NewSentinelClient(opt *Options) (otelcClient *SentinelClient) {
	defer func() {
		if otelcClient != nil {
			otelcClient.AddHook(otelcNewRedisHook(otelcClient.String()))
		}
	}()
	return
}

func (c *Client) Conn() (otelcConn *Conn) {
	defer func() {
		if otelcConn != nil {
			otelcConn.AddHook(otelcNewRedisHook(otelcConn.String()))
		}
	}()
	return
}
