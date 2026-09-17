//go:build goinject

//inject:k8s.io/client-go/tools/cache/controller.go
//inject:id otelc-k8s-deltas-batch
//inject:version >=v0.35.0 <v0.36.0
package cache

// Port of otelc.yaml rule k8s_hook_processdeltasinbatch. Shared declarations
// (tracer, bridge, attrs) live in deltas.go so the two rules never duplicate
// them. processDeltasInBatch's non-transaction path delegates to the
// instrumented processDeltas, so a batch produces both spans on both sides.
// Note: hooksupport/otelcK8sInit are added declarations from deltas.go — this
// file carries only its template, which references them once that rule is
// selected too (v0.35 satisfies both windows).

import (
	"context"

	"go.opentelemetry.io/otel/trace"

	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

func processDeltasInBatch(
	handler ResourceEventHandler,
	clientState Store,
	deltas []Delta,
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
