//go:build goinject

//inject:github.com/apache/cassandra-gocql-driver/v2
package gocql

// Ported from go.opentelemetry.io/otelc instrumentation/github.com/apache/cassandra-gocql-driver/v2
// (hook.go + semconv/cassandra.go + otelc.yaml rule gocql_newsession).
// NewSession is the single entry point: ClusterConfig.CreateSession delegates
// to it, so wrapping the observers there instruments every session. The
// observer records query/batch/connect CLIENT spans with caller-configured
// timestamps and chains through to any user observers.

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcGocqlInstrumentationName = "go.opentelemetry.io/otelc/instrumentation/github.com/apache/cassandra-gocql-driver/v2"

//inject:add
const otelcGocqlInstrumentationKey = "GOCQL"

// ---- semconv ----

//inject:add
func otelcGocqlQueryTraceAttrs(opName, statement, keyspace string, host net.IP, port int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.DBSystemNameCassandra,
		semconv.DBQueryText(statement),
	}
	if opName != "" {
		attrs = append(attrs, semconv.DBOperationName(opName))
	}
	if keyspace != "" {
		attrs = append(attrs, semconv.DBNamespace(keyspace))
	}
	if len(host) > 0 {
		attrs = append(attrs, semconv.ServerAddress(host.String()))
	}
	if port > 0 {
		attrs = append(attrs, semconv.ServerPort(port))
	}
	return attrs
}

//inject:add
func otelcGocqlBatchTraceAttrs(statement string, batchSize int, keyspace string, host net.IP, port int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.DBSystemNameCassandra,
		semconv.DBOperationName("BATCH"),
		semconv.DBQueryText(statement),
		semconv.DBOperationBatchSize(batchSize),
	}
	if keyspace != "" {
		attrs = append(attrs, semconv.DBNamespace(keyspace))
	}
	if len(host) > 0 {
		attrs = append(attrs, semconv.ServerAddress(host.String()))
	}
	if port > 0 {
		attrs = append(attrs, semconv.ServerPort(port))
	}
	return attrs
}

//inject:add
func otelcGocqlConnectTraceAttrs(host net.IP, port int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.DBSystemNameCassandra,
		semconv.DBOperationName("CONNECT"),
	}
	if len(host) > 0 {
		attrs = append(attrs, semconv.ServerAddress(host.String()))
	}
	if port > 0 {
		attrs = append(attrs, semconv.ServerPort(port))
	}
	return attrs
}

// ---- span naming and statement parsing ----

//inject:add
func otelcGocqlQuerySpanName(opName, keyspace string) string {
	if opName != "" && keyspace != "" {
		return opName + " " + keyspace
	}
	if opName != "" {
		return opName
	}
	if keyspace != "" {
		return keyspace
	}
	return "cassandra"
}

//inject:add
func otelcGocqlBatchSpanName(keyspace string) string {
	if keyspace != "" {
		return "BATCH " + keyspace
	}
	return "BATCH"
}

//inject:add
func otelcGocqlConnectSpanName(host net.IP, port int) string {
	if len(host) > 0 {
		addr := host.String()
		if port > 0 {
			return "CONNECT " + net.JoinHostPort(addr, strconv.Itoa(port))
		}
		return "CONNECT " + addr
	}
	return "CONNECT"
}

//inject:add
func otelcGocqlParseOpName(stmt string) string {
	stmt = otelcGocqlStripLeadingComments(stmt)
	if stmt == "" {
		return ""
	}
	fields := strings.Fields(stmt)
	if len(fields) > 0 {
		return strings.ToUpper(fields[0])
	}
	return ""
}

//inject:add
func otelcGocqlStripLeadingComments(stmt string) string {
	for {
		stmt = strings.TrimSpace(stmt)
		if strings.HasPrefix(stmt, "/*") {
			end := strings.Index(stmt, "*/")
			if end == -1 {
				return ""
			}
			stmt = stmt[end+2:]
			continue
		}
		if strings.HasPrefix(stmt, "--") || strings.HasPrefix(stmt, "//") {
			end := strings.IndexAny(stmt, "\r\n")
			if end == -1 {
				return ""
			}
			stmt = stmt[end:]
			continue
		}
		break
	}
	return stmt
}

// ---- observer ----

//inject:add
type otelcGocqlObserver struct {
	userQuery   QueryObserver
	userBatch   BatchObserver
	userConnect ConnectObserver
	tracer      trace.Tracer
}

//inject:add
func otelcGocqlNewObserver(userQuery QueryObserver, userBatch BatchObserver, userConnect ConnectObserver) *otelcGocqlObserver {
	return &otelcGocqlObserver{
		userQuery:   userQuery,
		userBatch:   userBatch,
		userConnect: userConnect,
		tracer:      otel.GetTracerProvider().Tracer(otelcGocqlInstrumentationName),
	}
}

//inject:add
func (o *otelcGocqlObserver) ObserveQuery(ctx context.Context, q ObservedQuery) {
	if hooksupport.Instrumented(otelcGocqlInstrumentationKey) {
		o.recordQuerySpan(ctx, q)
	}
	if o.userQuery != nil {
		o.userQuery.ObserveQuery(ctx, q)
	}
}

//inject:add
func (o *otelcGocqlObserver) ObserveBatch(ctx context.Context, b ObservedBatch) {
	if hooksupport.Instrumented(otelcGocqlInstrumentationKey) {
		o.recordBatchSpan(ctx, b)
	}
	if o.userBatch != nil {
		o.userBatch.ObserveBatch(ctx, b)
	}
}

//inject:add
func (o *otelcGocqlObserver) ObserveConnect(oc ObservedConnect) {
	if hooksupport.Instrumented(otelcGocqlInstrumentationKey) {
		o.recordConnectSpan(oc)
	}
	if o.userConnect != nil {
		o.userConnect.ObserveConnect(oc)
	}
}

//inject:add
func (o *otelcGocqlObserver) recordQuerySpan(ctx context.Context, q ObservedQuery) {
	if ctx == nil {
		ctx = context.Background()
	}
	opName := otelcGocqlParseOpName(q.Statement)
	spanName := otelcGocqlQuerySpanName(opName, q.Keyspace)

	startTime := q.Start
	if startTime.IsZero() {
		startTime = time.Now()
	}
	endTime := q.End
	if endTime.IsZero() {
		endTime = time.Now()
	}

	_, span := o.tracer.Start(ctx, spanName,
		trace.WithTimestamp(startTime),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End(trace.WithTimestamp(endTime))

	var host net.IP
	port := 0
	if q.Host != nil {
		host = q.Host.ConnectAddress()
		port = q.Host.Port()
	}
	span.SetAttributes(otelcGocqlQueryTraceAttrs(opName, q.Statement, q.Keyspace, host, port)...)

	if q.Err != nil {
		span.RecordError(q.Err)
		span.SetStatus(codes.Error, q.Err.Error())
	}
}

//inject:add
func (o *otelcGocqlObserver) recordBatchSpan(ctx context.Context, b ObservedBatch) {
	if ctx == nil {
		ctx = context.Background()
	}
	spanName := otelcGocqlBatchSpanName(b.Keyspace)

	startTime := b.Start
	if startTime.IsZero() {
		startTime = time.Now()
	}
	endTime := b.End
	if endTime.IsZero() {
		endTime = time.Now()
	}

	_, span := o.tracer.Start(ctx, spanName,
		trace.WithTimestamp(startTime),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End(trace.WithTimestamp(endTime))

	var host net.IP
	port := 0
	if b.Host != nil {
		host = b.Host.ConnectAddress()
		port = b.Host.Port()
	}
	span.SetAttributes(otelcGocqlBatchTraceAttrs(strings.Join(b.Statements, "; "), len(b.Statements), b.Keyspace, host, port)...)

	if b.Err != nil {
		span.RecordError(b.Err)
		span.SetStatus(codes.Error, b.Err.Error())
	}
}

//inject:add
func (o *otelcGocqlObserver) recordConnectSpan(oc ObservedConnect) {
	startTime := oc.Start
	if startTime.IsZero() {
		startTime = time.Now()
	}
	endTime := oc.End
	if endTime.IsZero() {
		endTime = time.Now()
	}

	var host net.IP
	port := 0
	if oc.Host != nil {
		host = oc.Host.ConnectAddress()
		port = oc.Host.Port()
	}

	spanName := otelcGocqlConnectSpanName(host, port)

	_, span := o.tracer.Start(context.Background(), spanName,
		trace.WithTimestamp(startTime),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End(trace.WithTimestamp(endTime))

	span.SetAttributes(otelcGocqlConnectTraceAttrs(host, port)...)

	if oc.Err != nil {
		span.RecordError(oc.Err)
		span.SetStatus(codes.Error, oc.Err.Error())
	}
}

// ---- template ----

func NewSession(cfg ClusterConfig) (*Session, error) {
	if hooksupport.Instrumented(otelcGocqlInstrumentationKey) {
		if _, ok := cfg.QueryObserver.(*otelcGocqlObserver); !ok {
			obs := otelcGocqlNewObserver(cfg.QueryObserver, cfg.BatchObserver, cfg.ConnectObserver)
			cfg.QueryObserver = obs
			cfg.BatchObserver = obs
			cfg.ConnectObserver = obs
		}
	}
	return nil, nil
}
