// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package k8sapi

import (
	"context"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	hooklog "github.com/kakj-go/go-inject-trace-contrib/otelc/hooklog"
	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

var k8sTracer trace.Tracer

var k8sTracerOnce sync.Once

func initTracer() {
	k8sTracerOnce.Do(func() {
		k8sTracer = otel.GetTracerProvider().Tracer(
			"go.opentelemetry.io/otelc/instrumentation/k8s.io/client-go",
			trace.WithInstrumentationVersion(hooksupport.ModuleVersion()),
		)
		hooklog.Logger().Info("K8S client-go instrumentation initialized")
	})
}

// NewHandler wraps a ResourceEventHandler so each event gets an INTERNAL span
// carrying the process span's context. It is the link-bridge target from the
// tools/cache templates.
func NewHandler(handler cache.ResourceEventHandler, ctx context.Context) cache.ResourceEventHandler {
	initTracer()
	return newOtelEventHandler(handler, ctx)
}

type k8sOtelEventHandler struct {
	handler cache.ResourceEventHandler
	ctx     context.Context
}

func newOtelEventHandler(handler cache.ResourceEventHandler, ctx context.Context) *k8sOtelEventHandler {
	return &k8sOtelEventHandler{handler, ctx}
}

func (h k8sOtelEventHandler) OnAdd(obj any, isInInitialList bool) {
	objInfo := getObjectInfo(obj)
	attrs := K8SObjectInfoTraceAttrs(objInfo)

	spanName := getSpanName(objInfo.Kind, "add")
	_, span := k8sTracer.Start(h.ctx,
		spanName,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
	defer span.End()

	h.handler.OnAdd(obj, isInInitialList)
}

func (h k8sOtelEventHandler) OnUpdate(oldObj, newObj any) {
	objInfo := getObjectInfo(newObj)
	attrs := K8SObjectInfoTraceAttrs(objInfo)

	spanName := getSpanName(objInfo.Kind, "update")
	_, span := k8sTracer.Start(h.ctx,
		spanName,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
	defer span.End()

	h.handler.OnUpdate(oldObj, newObj)
}

func (h k8sOtelEventHandler) OnDelete(obj any) {
	// Unwrap only for metadata extraction: a DeletedFinalStateUnknown tombstone
	// means client-go's cache lost track of the object (e.g. a missed watch
	// event during a resync), so the delete is inferred rather than confirmed.
	// The wrapped user handler needs the original, possibly-wrapped obj to make
	// that same determination itself; forwarding the unwrapped value would
	// silently change observable application behavior under instrumentation.
	metaObj := obj
	if o, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		metaObj = o.Obj
	}

	objInfo := getObjectInfo(metaObj)
	attrs := K8SObjectInfoTraceAttrs(objInfo)

	spanName := getSpanName(objInfo.Kind, "delete")
	_, span := k8sTracer.Start(h.ctx,
		spanName,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
	defer span.End()

	h.handler.OnDelete(obj)
}

func getSpanName(kind, action string) string {
	if len(kind) > 0 {
		return "k8s.informer." + strings.ToLower(kind) + "." + action
	}
	return "k8s.informer.object." + action
}

func getObjectInfo(obj any) K8SObjectInfo {
	objInfo := K8SObjectInfo{}

	if m, err := meta.Accessor(obj); err == nil {
		objInfo.UID = string(m.GetUID())
		objInfo.Name = m.GetName()
		objInfo.Namespace = m.GetNamespace()
	}

	runtimeObj, ok := obj.(runtime.Object)
	if !ok {
		hooklog.Logger().Debug("object does not implement runtime.Object, cannot determine GVK")
		return objInfo
	}

	gvks, _, err := scheme.Scheme.ObjectKinds(runtimeObj)
	if err != nil || len(gvks) == 0 {
		hooklog.Logger().Debug("failed to get GVK for object", "error", err)
		return objInfo
	}

	gvk := gvks[0]
	objInfo.Kind = gvk.Kind
	objInfo.APIVersion = gvk.GroupVersion().String()

	if objInfo.Kind != "Pod" && objInfo.Kind != "HorizontalPodAutoscaler" {
		return objInfo
	}

	switch o := obj.(type) {
	case *corev1.Pod:
		objInfo.NodeName = o.Spec.NodeName
	case *autoscalingv2.HorizontalPodAutoscaler:
		objInfo.HPAScaleTargetRefAPIVersion = o.Spec.ScaleTargetRef.APIVersion
		objInfo.HPAScaleTargetRefKind = o.Spec.ScaleTargetRef.Kind
		objInfo.HPAScaleTargetRefName = o.Spec.ScaleTargetRef.Name
	}

	return objInfo
}
