//go:build goinject

//inject:k8s.io/client-go/tools/cache/controller.go
//inject:id otelc-k8s-deltas
//inject:version >=v0.34.0 <v0.36.0
package cache

// Ported from go.opentelemetry.io/otelc instrumentation/k8s.io/client-go
// (hook.go + k8s_otel_handler.go + semconv/k8s.go + otelc.yaml rule
// k8s_hook_processdeltas). The process span lives here; per-event spans are
// created by the handler wrapper in otelc/k8sapi, reached through a link
// bridge because injecting k8s.io/client-go/kubernetes/scheme imports here
// would risk an import cycle back into tools/cache.

import (
	"context"
	"sync"
	_ "unsafe"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	hooklog "github.com/kakj-go/go-inject-trace-contrib/otelc/hooklog"
	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcK8sInstrumentationName = "go.opentelemetry.io/otelc/instrumentation/k8s.io/client-go"

//inject:add
const otelcK8sInstrumentationKey = "K8S_CLIENT_GO"

//inject:add
var otelcK8sTracer trace.Tracer

//inject:add
var otelcK8sInitOnce sync.Once

//inject:add
func otelcK8sInit() {
	otelcK8sInitOnce.Do(func() {
		otelcK8sTracer = otel.GetTracerProvider().Tracer(
			otelcK8sInstrumentationName,
			trace.WithInstrumentationVersion(hooksupport.ModuleVersion()),
		)
		hooklog.Logger().Info("K8S client-go instrumentation initialized")
	})
}

//inject:add
//go:linkname otelcK8sNewHandler github.com/kakj-go/go-inject-trace-contrib/otelc/k8sapi.NewHandler
func otelcK8sNewHandler(h ResourceEventHandler, ctx context.Context) ResourceEventHandler

//inject:add
func otelcK8sObjectsAttrs(count int, isInInitialList bool) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Int("k8s.objects.count", count),
		attribute.Bool("k8s.objects.is_in_initial_list", isInInitialList),
	}
}

func processDeltas(
	handler ResourceEventHandler,
	clientState Store,
	deltas Deltas,
	isInInitialList bool,
) (otelcErr error) {
	var otelcSpan trace.Span
	if hooksupport.Instrumented(otelcK8sInstrumentationKey) {
		otelcK8sInit()

		otelcCtx, span := otelcK8sTracer.Start(context.Background(),
			"k8s.informer.objects.process",
			trace.WithSpanKind(trace.SpanKindInternal),
			trace.WithAttributes(otelcK8sObjectsAttrs(len(deltas), isInInitialList)...),
		)
		otelcSpan = span
		handler = otelcK8sNewHandler(handler, otelcCtx)
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
