//go:build goinject

//inject:google.golang.org/grpc
package grpc

// Ported from go.opentelemetry.io/otelc instrumentation/google.golang.org/grpc
// (client/client_hook.go + server/server_hook.go + semconv/{grpc,semconv,util}.go
// + otelc.yaml rules client_hook_newclient/_dialcontext and server_hook). The
// constructor hooks prepend a stats.Handler to the dial/server options; the
// handler creates CLIENT/SERVER spans per RPC, propagates trace context through
// gRPC metadata, and records the rpc.* metric family via rpcconv.

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/semconv/v1.37.0/rpcconv"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"

	hooklog "github.com/kakj-go/go-inject-trace-contrib/otelc/hooklog"
	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcGRPCInstrumentationName = "go.opentelemetry.io/otelc/instrumentation/google.golang.org/grpc"

//inject:add
const otelcGRPCInstrumentationKey = "GRPC"

// ---- shared state (client and server keep separate lazy initializers and
// startup log lines, mirroring the two upstream hook packages) ----

//inject:add
var otelcGRPCClientOnce sync.Once

//inject:add
var otelcGRPCServerOnce sync.Once

//inject:add
var otelcGRPCTracer trace.Tracer

//inject:add
var otelcGRPCPropagator propagation.TextMapPropagator

//inject:add
var otelcGRPCClientMeter metric.Meter

//inject:add
var otelcGRPCServerMeter metric.Meter

//inject:add
var otelcGRPCClientDuration rpcconv.ClientDuration

//inject:add
var otelcGRPCClientRequestSize rpcconv.ClientRequestSize

//inject:add
var otelcGRPCClientResponseSize rpcconv.ClientResponseSize

//inject:add
var otelcGRPCClientRequestsPerRPC rpcconv.ClientRequestsPerRPC

//inject:add
var otelcGRPCClientResponsesPerRPC rpcconv.ClientResponsesPerRPC

//inject:add
var otelcGRPCServerDuration rpcconv.ServerDuration

//inject:add
var otelcGRPCServerRequestSize rpcconv.ServerRequestSize

//inject:add
var otelcGRPCServerResponseSize rpcconv.ServerResponseSize

//inject:add
var otelcGRPCServerRequestsPerRPC rpcconv.ServerRequestsPerRPC

//inject:add
var otelcGRPCServerResponsesPerRPC rpcconv.ServerResponsesPerRPC

//inject:add
func otelcGRPCCommonInit() {
	otelcGRPCTracer = otel.GetTracerProvider().Tracer(
		otelcGRPCInstrumentationName,
		trace.WithInstrumentationVersion(hooksupport.ModuleVersion()),
	)
	otelcGRPCPropagator = otel.GetTextMapPropagator()
}

//inject:add
func otelcGRPCClientInit() {
	otelcGRPCClientOnce.Do(func() {
		otelcGRPCCommonInit()
		otelcGRPCClientMeter = otel.GetMeterProvider().Meter(
			otelcGRPCInstrumentationName,
			metric.WithInstrumentationVersion(hooksupport.ModuleVersion()),
			metric.WithSchemaURL(semconv.SchemaURL),
		)

		var err error
		otelcGRPCClientDuration, err = rpcconv.NewClientDuration(otelcGRPCClientMeter)
		if err != nil {
			hooklog.Logger().Error("failed to create client duration metric", "error", err)
		}
		otelcGRPCClientRequestSize, err = rpcconv.NewClientRequestSize(otelcGRPCClientMeter)
		if err != nil {
			hooklog.Logger().Error("failed to create client request size metric", "error", err)
		}
		otelcGRPCClientResponseSize, err = rpcconv.NewClientResponseSize(otelcGRPCClientMeter)
		if err != nil {
			hooklog.Logger().Error("failed to create client response size metric", "error", err)
		}
		otelcGRPCClientRequestsPerRPC, err = rpcconv.NewClientRequestsPerRPC(otelcGRPCClientMeter)
		if err != nil {
			hooklog.Logger().Error("failed to create client requests per rpc metric", "error", err)
		}
		otelcGRPCClientResponsesPerRPC, err = rpcconv.NewClientResponsesPerRPC(otelcGRPCClientMeter)
		if err != nil {
			hooklog.Logger().Error("failed to create client responses per rpc metric", "error", err)
		}

		hooklog.Logger().Info("gRPC client instrumentation initialized")
	})
}

//inject:add
func otelcGRPCServerInit() {
	otelcGRPCServerOnce.Do(func() {
		otelcGRPCCommonInit()
		otelcGRPCServerMeter = otel.GetMeterProvider().Meter(
			otelcGRPCInstrumentationName,
			metric.WithInstrumentationVersion(hooksupport.ModuleVersion()),
			metric.WithSchemaURL(semconv.SchemaURL),
		)

		var err error
		otelcGRPCServerDuration, err = rpcconv.NewServerDuration(otelcGRPCServerMeter)
		if err != nil {
			hooklog.Logger().Error("failed to create server duration metric", "error", err)
		}
		otelcGRPCServerRequestSize, err = rpcconv.NewServerRequestSize(otelcGRPCServerMeter)
		if err != nil {
			hooklog.Logger().Error("failed to create server request size metric", "error", err)
		}
		otelcGRPCServerResponseSize, err = rpcconv.NewServerResponseSize(otelcGRPCServerMeter)
		if err != nil {
			hooklog.Logger().Error("failed to create server response size metric", "error", err)
		}
		otelcGRPCServerRequestsPerRPC, err = rpcconv.NewServerRequestsPerRPC(otelcGRPCServerMeter)
		if err != nil {
			hooklog.Logger().Error("failed to create server requests per rpc metric", "error", err)
		}
		otelcGRPCServerResponsesPerRPC, err = rpcconv.NewServerResponsesPerRPC(otelcGRPCServerMeter)
		if err != nil {
			hooklog.Logger().Error("failed to create server responses per rpc metric", "error", err)
		}

		hooklog.Logger().Info("gRPC server instrumentation initialized")
	})
}

// ---- semconv helpers ----

//inject:add
const otelcGRPCExporterTracePath = "/opentelemetry.proto.collector.trace.v1.TraceService/Export"

//inject:add
const otelcGRPCExporterMetricPath = "/opentelemetry.proto.collector.metrics.v1.MetricsService/Export"

//inject:add
const otelcGRPCExporterLogPath = "/opentelemetry.proto.collector.logs.v1.LogsService/Export"

//inject:add
func otelcGRPCIsExporterPath(fullMethod string) bool {
	return fullMethod == otelcGRPCExporterTracePath ||
		fullMethod == otelcGRPCExporterMetricPath ||
		fullMethod == otelcGRPCExporterLogPath
}

//inject:add
func otelcGRPCParseFullMethod(fullMethod string) (string, []attribute.KeyValue) {
	if !strings.HasPrefix(fullMethod, "/") {
		return fullMethod, []attribute.KeyValue{semconv.RPCSystemGRPC}
	}
	name := fullMethod[1:]
	pos := strings.LastIndex(name, "/")
	if pos < 0 {
		return name, []attribute.KeyValue{semconv.RPCSystemGRPC}
	}
	service, method := name[:pos], name[pos+1:]

	attrs := []attribute.KeyValue{semconv.RPCSystemGRPC}
	if service != "" {
		attrs = append(attrs, semconv.RPCService(service))
	}
	if method != "" {
		attrs = append(attrs, semconv.RPCMethod(method))
	}
	return name, attrs
}

//inject:add
func otelcGRPCStatusCodeAttr(code int) attribute.KeyValue {
	return semconv.RPCGRPCStatusCodeKey.Int(code)
}

//inject:add
func otelcGRPCServerStatus(s *status.Status) (codes.Code, string) {
	switch s.Code() {
	case 0, 1, 3, 5, 6, 7, 8, 9, 10, 11, 16:
		return codes.Unset, ""
	case 2, 4, 12, 13, 14, 15:
		return codes.Error, s.Message()
	default:
		return codes.Error, s.Message()
	}
}

//inject:add
func otelcGRPCClientStatus(s *status.Status) (codes.Code, string) {
	if s.Code() == 0 {
		return codes.Unset, ""
	}
	return codes.Error, s.Message()
}

//inject:add
func otelcGRPCSplitHostPort(hostport string) (string, int) {
	host := ""
	port := -1

	if strings.HasPrefix(hostport, "[") {
		addrEnd := strings.LastIndexByte(hostport, ']')
		if addrEnd < 0 {
			return "", port
		}
		if i := strings.LastIndexByte(hostport[addrEnd:], ':'); i < 0 {
			return hostport[1:addrEnd], port
		}
	} else {
		if i := strings.LastIndexByte(hostport, ':'); i < 0 {
			return hostport, port
		}
	}

	host, pStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return "", port
	}
	p, err := strconv.ParseUint(pStr, 10, 16)
	if err != nil {
		return host, port
	}
	return host, int(p)
}

//inject:add
func otelcGRPCServerAddrAttrs(addr string) []attribute.KeyValue {
	host, port := otelcGRPCSplitHostPort(addr)
	var attrs []attribute.KeyValue
	if host != "" {
		attrs = append(attrs, semconv.ServerAddress(host))
	}
	if port > 0 {
		attrs = append(attrs, semconv.ServerPort(port))
	}
	return attrs
}

//inject:add
func otelcGRPCClientAddrAttrs(addr string) []attribute.KeyValue {
	host, port := otelcGRPCSplitHostPort(addr)
	var attrs []attribute.KeyValue
	if host != "" {
		attrs = append(attrs, semconv.ClientAddress(host))
	}
	if port > 0 {
		attrs = append(attrs, semconv.ClientPort(port))
	}
	return attrs
}

// ---- metadata propagation (semconv/semconv.go) ----

// otelcGRPCMetadataSupplier is a TextMapCarrier for gRPC metadata.
//
//inject:add
type otelcGRPCMetadataSupplier struct {
	metadata *metadata.MD
}

//inject:add
func (s otelcGRPCMetadataSupplier) Get(key string) string {
	values := s.metadata.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

//inject:add
func (s otelcGRPCMetadataSupplier) Set(key, value string) {
	s.metadata.Set(key, value)
}

//inject:add
func (s otelcGRPCMetadataSupplier) Keys() []string {
	out := make([]string, 0, len(*s.metadata))
	for key := range *s.metadata {
		out = append(out, key)
	}
	return out
}

//inject:add
func otelcGRPCInject(ctx context.Context, propagators propagation.TextMapPropagator) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.MD{}
	}
	propagators.Inject(ctx, otelcGRPCMetadataSupplier{metadata: &md})
	return metadata.NewOutgoingContext(ctx, md)
}

//inject:add
func otelcGRPCExtract(ctx context.Context, propagators propagation.TextMapPropagator) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.MD{}
	}
	return propagators.Extract(ctx, otelcGRPCMetadataSupplier{metadata: &md})
}

// ---- per-RPC context ----

//inject:add
type otelcGRPCContextKey struct{}

//inject:add
type otelcGRPCContext struct {
	inMessages    int64
	outMessages   int64
	metricAttrs   []attribute.KeyValue
	metricAttrSet attribute.Set
}

// ---- stats handlers ----

//inject:add
type otelcGRPCClientStatsHandler struct{}

//inject:add
func (h *otelcGRPCClientStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	if otelcGRPCIsExporterPath(info.FullMethodName) {
		return ctx
	}

	name, attrs := otelcGRPCParseFullMethod(info.FullMethodName)

	ctx, _ = otelcGRPCTracer.Start(
		ctx,
		name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)

	ctx = otelcGRPCInject(ctx, otelcGRPCPropagator)

	gctx := &otelcGRPCContext{
		metricAttrs:   attrs,
		metricAttrSet: attribute.NewSet(attrs...),
	}

	return context.WithValue(ctx, otelcGRPCContextKey{}, gctx)
}

//inject:add
func (h *otelcGRPCClientStatsHandler) HandleRPC(ctx context.Context, rs stats.RPCStats) {
	gctx, _ := ctx.Value(otelcGRPCContextKey{}).(*otelcGRPCContext)
	if gctx == nil {
		return
	}

	span := trace.SpanFromContext(ctx)

	switch rs := rs.(type) {
	case *stats.Begin:
		// RPC started
	case *stats.OutPayload:
		atomic.AddInt64(&gctx.outMessages, 1)
		if otelcGRPCClientRequestSize.Inst() != nil {
			otelcGRPCClientRequestSize.Inst().Record(ctx, int64(rs.Length), metric.WithAttributeSet(gctx.metricAttrSet))
		}
	case *stats.InPayload:
		atomic.AddInt64(&gctx.inMessages, 1)
		if otelcGRPCClientResponseSize.Inst() != nil {
			otelcGRPCClientResponseSize.Inst().Record(ctx, int64(rs.Length), metric.WithAttributeSet(gctx.metricAttrSet))
		}
	case *stats.OutHeader:
		if span.IsRecording() {
			if p, ok := peer.FromContext(ctx); ok {
				span.SetAttributes(otelcGRPCServerAddrAttrs(p.Addr.String())...)
			}
		}
	case *stats.End:
		var s *status.Status
		var statusAttr attribute.KeyValue
		if rs.Error != nil {
			s, _ = status.FromError(rs.Error)
			statusAttr = otelcGRPCStatusCodeAttr(int(s.Code()))
		} else {
			s = status.New(0, "")
			statusAttr = otelcGRPCStatusCodeAttr(0)
		}

		if span.IsRecording() {
			if s != nil {
				code, msg := otelcGRPCClientStatus(s)
				span.SetStatus(code, msg)
			}
			if rs.Error != nil {
				span.RecordError(rs.Error)
			}
			span.SetAttributes(statusAttr)
			span.End()
		}

		metricAttrs := make([]attribute.KeyValue, 0, len(gctx.metricAttrs)+1)
		metricAttrs = append(metricAttrs, gctx.metricAttrs...)
		metricAttrs = append(metricAttrs, statusAttr)
		recordOpts := []metric.RecordOption{metric.WithAttributeSet(attribute.NewSet(metricAttrs...))}

		duration := float64(rs.EndTime.Sub(rs.BeginTime)) / float64(time.Millisecond)

		if otelcGRPCClientDuration.Inst() != nil {
			otelcGRPCClientDuration.Inst().Record(ctx, duration, recordOpts...)
		}
		if otelcGRPCClientRequestsPerRPC.Inst() != nil {
			otelcGRPCClientRequestsPerRPC.Inst().Record(ctx, atomic.LoadInt64(&gctx.outMessages), recordOpts...)
		}
		if otelcGRPCClientResponsesPerRPC.Inst() != nil {
			otelcGRPCClientResponsesPerRPC.Inst().Record(ctx, atomic.LoadInt64(&gctx.inMessages), recordOpts...)
		}
	}
}

//inject:add
func (h *otelcGRPCClientStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

//inject:add
func (h *otelcGRPCClientStatsHandler) HandleConn(context.Context, stats.ConnStats) {
	// no-op
}

//inject:add
type otelcGRPCServerStatsHandler struct{}

//inject:add
func (h *otelcGRPCServerStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	if otelcGRPCIsExporterPath(info.FullMethodName) {
		return ctx
	}

	ctx = otelcGRPCExtract(ctx, otelcGRPCPropagator)

	name, attrs := otelcGRPCParseFullMethod(info.FullMethodName)

	ctx, _ = otelcGRPCTracer.Start(
		trace.ContextWithRemoteSpanContext(ctx, trace.SpanContextFromContext(ctx)),
		name,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attrs...),
	)

	gctx := &otelcGRPCContext{
		metricAttrs:   attrs,
		metricAttrSet: attribute.NewSet(attrs...),
	}

	return context.WithValue(ctx, otelcGRPCContextKey{}, gctx)
}

//inject:add
func (h *otelcGRPCServerStatsHandler) HandleRPC(ctx context.Context, rs stats.RPCStats) {
	gctx, _ := ctx.Value(otelcGRPCContextKey{}).(*otelcGRPCContext)
	if gctx == nil {
		return
	}

	span := trace.SpanFromContext(ctx)

	switch rs := rs.(type) {
	case *stats.Begin:
		// RPC started
	case *stats.InPayload:
		atomic.AddInt64(&gctx.inMessages, 1)
		if otelcGRPCServerRequestSize.Inst() != nil {
			otelcGRPCServerRequestSize.Inst().Record(ctx, int64(rs.Length), metric.WithAttributeSet(gctx.metricAttrSet))
		}
	case *stats.OutPayload:
		atomic.AddInt64(&gctx.outMessages, 1)
		if otelcGRPCServerResponseSize.Inst() != nil {
			otelcGRPCServerResponseSize.Inst().Record(ctx, int64(rs.Length), metric.WithAttributeSet(gctx.metricAttrSet))
		}
	case *stats.OutHeader:
		if span.IsRecording() {
			if p, ok := peer.FromContext(ctx); ok {
				span.SetAttributes(otelcGRPCClientAddrAttrs(p.Addr.String())...)
			}
		}
	case *stats.End:
		var s *status.Status
		var statusAttr attribute.KeyValue
		if rs.Error != nil {
			s, _ = status.FromError(rs.Error)
			statusAttr = otelcGRPCStatusCodeAttr(int(s.Code()))
		} else {
			s = status.New(0, "")
			statusAttr = otelcGRPCStatusCodeAttr(0)
		}

		if span.IsRecording() {
			if s != nil {
				code, msg := otelcGRPCServerStatus(s)
				span.SetStatus(code, msg)
			}
			if rs.Error != nil {
				span.RecordError(rs.Error)
			}
			span.SetAttributes(statusAttr)
			span.End()
		}

		metricAttrs := make([]attribute.KeyValue, 0, len(gctx.metricAttrs)+1)
		metricAttrs = append(metricAttrs, gctx.metricAttrs...)
		metricAttrs = append(metricAttrs, statusAttr)
		recordOpts := []metric.RecordOption{metric.WithAttributeSet(attribute.NewSet(metricAttrs...))}

		duration := float64(rs.EndTime.Sub(rs.BeginTime)) / float64(time.Millisecond)

		if otelcGRPCServerDuration.Inst() != nil {
			otelcGRPCServerDuration.Inst().Record(ctx, duration, recordOpts...)
		}
		if otelcGRPCServerRequestsPerRPC.Inst() != nil {
			otelcGRPCServerRequestsPerRPC.Inst().Record(ctx, atomic.LoadInt64(&gctx.inMessages), recordOpts...)
		}
		if otelcGRPCServerResponsesPerRPC.Inst() != nil {
			otelcGRPCServerResponsesPerRPC.Inst().Record(ctx, atomic.LoadInt64(&gctx.outMessages), recordOpts...)
		}
	}
}

//inject:add
func (h *otelcGRPCServerStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

//inject:add
func (h *otelcGRPCServerStatsHandler) HandleConn(context.Context, stats.ConnStats) {
	// no-op
}

// ---- constructor templates ----

func NewClient(target string, opts ...DialOption) (otelcConn *ClientConn, otelcErr error) {
	if hooksupport.Instrumented(otelcGRPCInstrumentationKey) {
		otelcGRPCClientInit()

		handler := &otelcGRPCClientStatsHandler{}
		opts = append([]DialOption{WithStatsHandler(handler)}, opts...)
	}
	return nil, nil
}

func DialContext(ctx context.Context, target string, opts ...DialOption) (otelcConn *ClientConn, otelcErr error) {
	if hooksupport.Instrumented(otelcGRPCInstrumentationKey) {
		otelcGRPCClientInit()

		handler := &otelcGRPCClientStatsHandler{}
		opts = append([]DialOption{WithStatsHandler(handler)}, opts...)
	}
	return nil, nil
}

func NewServer(opt ...ServerOption) (otelcServer *Server) {
	if hooksupport.Instrumented(otelcGRPCInstrumentationKey) {
		otelcGRPCServerInit()

		handler := &otelcGRPCServerStatsHandler{}
		opt = append([]ServerOption{StatsHandler(handler)}, opt...)
	}
	return
}
