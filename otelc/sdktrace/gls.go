//go:build goinject

//inject:go.opentelemetry.io/otel/sdk/trace
//inject:id otelc-sdktrace-gls
//inject:version >=v1.44.0
package trace

// Ported from go.opentelemetry.io/otelc instrumentation/go.opentelemetry.io/otel/sdk/trace
// (otel_trace_context.go + hook.go + gls.go + otelc.yaml rules trace_context,
// hook_new_recording_span, hook_new_non_recording_span, hook_recording_span_end,
// hook_non_recording_span_end).
//
// Every span created by the SDK is pushed on the creating goroutine's GLS span
// stack and popped when it ends, so the top of stack is always the innermost
// live span even without a context.Context. Differences from upstream: the
// go:linkname bridges into pkg/runtime are replaced by direct imports of
// otelgls (registration) and hooksupport (logger), which this port's package
// split keeps acyclic.

import (
	"container/list"
	"context"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"

	trace "go.opentelemetry.io/otel/trace"

	"github.com/kakj-go/go-inject-trace-contrib/otelc/hooklog"
	"github.com/kakj-go/go-inject-trace-contrib/otelc/otelgls"
)

//inject:add
const defaultGLSMaxSpans = 1000

// defaultMaxSpanStates bounds lifecycle bookkeeping. Evicted states are marked
// ended, so reaching the limit drops implicit propagation for the evicted span
// instead of retaining a stale parent for it.
//
//inject:add
const defaultMaxSpanStates = 100_000

//inject:add
var otelGLSMaxSpans = defaultGLSMaxSpans

//inject:add
var maxSpanStates = defaultMaxSpanStates

//inject:add
func init() {
	if parsed, ok := positiveIntEnv("OTEL_GLS_MAX_SPANS"); ok {
		otelGLSMaxSpans = parsed
	}
	if parsed, ok := positiveIntEnv("OTEL_GLS_MAX_SPAN_STATES"); ok {
		maxSpanStates = parsed
	}
	otelgls.RegisterTraceAndSpanID(GetTraceAndSpanID)
	otelgls.RegisterSpanFromGLS(func() interface{} { return spanFromGLS() })
}

//inject:add
func positiveIntEnv(name string) (int, bool) {
	v := os.Getenv(name)
	if v == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(v)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

//inject:add
type traceContext struct {
	sw  *spanWrapper
	n   int
	lcs trace.Span
}

//inject:add
type spanWrapper struct {
	span  trace.Span
	prev  *spanWrapper
	ended *atomic.Bool
}

//inject:add
type spanKey struct {
	traceID trace.TraceID
	spanID  trace.SpanID
}

// spanStateEntry pairs a span's ended flag with its position in the
// insertion-order eviction list.
//
//inject:add
type spanStateEntry struct {
	state *atomic.Bool
	elem  *list.Element
}

// spanStates bounds shared lifecycle bookkeeping across goroutines. order
// tracks insertion order (oldest at the front) so that once the map hits
// maxSpanStates, eviction always drops the oldest entry rather than an
// arbitrary one — Go map iteration order is randomized, so iterating the map
// itself to pick a victim can evict a span that is still genuinely in flight
// on an unrelated goroutine.
//
//inject:add
var spanStates = struct {
	sync.Mutex
	states map[spanKey]*spanStateEntry
	order  *list.List
}{
	states: make(map[spanKey]*spanStateEntry),
	order:  list.New(),
}

// evictOldestLocked drops the oldest tracked span state, marking it ended so
// any goroutine still holding it stops treating it as a live parent. Callers
// must hold spanStates.Mutex.
//
//inject:add
func evictOldestLocked() {
	front := spanStates.order.Front()
	if front == nil {
		return
	}
	key, _ := front.Value.(spanKey)
	if entry, ok := spanStates.states[key]; ok {
		entry.state.Store(true)
		delete(spanStates.states, key)
	}
	spanStates.order.Remove(front)
	hooklog.Logger().Debug("GLS span state tracker at capacity, evicting oldest entry",
		"limit", maxSpanStates)
}

//inject:add
func stateForSpan(span trace.Span) *atomic.Bool {
	sc := span.SpanContext()
	if !sc.IsValid() {
		return &atomic.Bool{}
	}
	key := spanKey{sc.TraceID(), sc.SpanID()}
	spanStates.Lock()
	defer spanStates.Unlock()
	if entry, ok := spanStates.states[key]; ok {
		return entry.state
	}
	if len(spanStates.states) >= maxSpanStates {
		evictOldestLocked()
	}
	// Local names deliberately differ from the struct field names: the
	// template local renamer would otherwise rewrite the matching field keys
	// of the keyed literal below.
	st := &atomic.Bool{}
	el := spanStates.order.PushBack(key)
	spanStates.states[key] = &spanStateEntry{state: st, elem: el}
	return st
}

//inject:add
func markSpanEnded(span trace.Span) {
	sc := span.SpanContext()
	if !sc.IsValid() {
		return
	}
	key := spanKey{sc.TraceID(), sc.SpanID()}
	spanStates.Lock()
	defer spanStates.Unlock()
	if entry, ok := spanStates.states[key]; ok {
		entry.state.Store(true)
		delete(spanStates.states, key)
		spanStates.order.Remove(entry.elem)
	}
}

//inject:add
func (tc *traceContext) compact() {
	addr := &tc.sw
	for *addr != nil {
		if (*addr).ended.Load() {
			*addr = (*addr).prev
			tc.n--
			continue
		}
		addr = &(*addr).prev
	}
	tc.lcs = nil
	for cur := tc.sw; cur != nil; cur = cur.prev {
		tc.lcs = cur.span
	}
}

//inject:add
func (tc *traceContext) add(span trace.Span) bool {
	if tc.n >= otelGLSMaxSpans {
		tc.compact()
		if tc.n >= otelGLSMaxSpans {
			// The new span is dropped from GLS: it won't be reachable via
			// implicit propagation, and any trace.SpanFromContext(context.Background())
			// lookup on this goroutine keeps returning whatever was already
			// on top of the stack until it compacts below the limit. Surface
			// that instead of dropping it silently.
			hooklog.Logger().Debug("GLS span stack at capacity, span not tracked for implicit propagation",
				"limit", otelGLSMaxSpans)
			return false
		}
	}
	wrapper := &spanWrapper{span, tc.sw, stateForSpan(span)}
	if tc.n == 0 {
		tc.lcs = span
	}
	tc.sw = wrapper
	tc.n++
	return true
}

// tail must be called only on the current goroutine's own context: it mutates the
// stack, whose list fields are unsynchronized. Only ended flags are shared.
//
//inject:add
//go:norace
func (tc *traceContext) tail() trace.Span {
	for tc.sw != nil && tc.sw.ended.Load() {
		tc.sw = tc.sw.prev
		tc.n--
	}
	if tc.sw == nil {
		tc.lcs = nil
		return nil
	}
	return tc.sw.span
}

//inject:add
func (tc *traceContext) localRootSpan() trace.Span {
	if tc.n == 0 {
		return nil
	} else {
		return tc.lcs
	}
}

//inject:add
func (tc *traceContext) del(span trace.Span) {
	if tc.n == 0 {
		return
	}
	addr := &tc.sw
	cur := tc.sw
	for cur != nil {
		sc1 := cur.span.SpanContext()
		sc2 := span.SpanContext()
		if sc1.TraceID() == sc2.TraceID() && sc1.SpanID() == sc2.SpanID() {
			cur.ended.Store(true)
			*addr = cur.prev
			tc.n--
			if cur.prev == nil {
				tc.lcs = nil
				for remaining := tc.sw; remaining != nil; remaining = remaining.prev {
					tc.lcs = remaining.span
				}
			}
			break
		}
		addr = &cur.prev
		cur = cur.prev
	}
}

//inject:add
func (tc *traceContext) clear() {
	tc.sw = nil
	tc.n = 0
	tc.lcs = nil
	runtime.SetBaggageContainerToGLS(nil)
}

//inject:add
//go:norace
func (tc *traceContext) Clone() interface{} {
	last := tc.tail()
	if last == nil {
		return &traceContext{nil, 0, nil}
	}
	sw := &spanWrapper{last, nil, tc.sw.ended}
	return &traceContext{sw, 1, nil}
}

//inject:add
func GetTraceContext() trace.SpanContext {
	t := getOrInitTraceContext()
	if span := t.tail(); span != nil {
		return span.SpanContext()
	}
	return trace.SpanContext{}
}

//inject:add
func getOrInitTraceContext() *traceContext {
	tc := runtime.GetTraceContextFromGLS()
	if tc == nil {
		newTc := &traceContext{nil, 0, nil}
		setTraceContext(newTc)
		return newTc
	} else {
		return tc.(*traceContext)
	}
}

//inject:add
func setTraceContext(tc *traceContext) {
	runtime.SetTraceContextToGLS(tc)
}

//inject:add
func traceContextAddSpan(span trace.Span) {
	tc := getOrInitTraceContext()
	if tc.add(span) {
		setTraceContext(tc)
	}
}

//inject:add
func GetTraceAndSpanID() (string, string) {
	tc := runtime.GetTraceContextFromGLS()
	if tc == nil {
		return "", ""
	}
	span := tc.(*traceContext).tail()
	if span == nil {
		return "", ""
	}
	ctx := span.SpanContext()
	return ctx.TraceID().String(), ctx.SpanID().String()
}

//inject:add
func traceContextDelSpan(span trace.Span) {
	markSpanEnded(span)
	if ctx := runtime.GetTraceContextFromGLS(); ctx != nil {
		ctx.(*traceContext).del(span)
	}
}

//inject:add
func ClearTraceContext() {
	getOrInitTraceContext().clear()
}

//inject:add
func spanFromGLS() trace.Span {
	gls := runtime.GetTraceContextFromGLS()
	if gls == nil {
		return nil
	}
	return gls.(*traceContext).tail()
}

// Hook templates: the after hooks push newly created spans onto the GLS stack
// (reading the named result in a defer), the before hooks pop spans on End.

func (tr *tracer) newRecordingSpan(
	ctx context.Context,
	psc, sc trace.SpanContext,
	name string,
	sr SamplingResult,
	config *trace.SpanConfig,
) (otelcSpan *recordingSpan) {
	defer func() {
		if otelcSpan != nil {
			traceContextAddSpan(otelcSpan)
		}
	}()
	return
}

func (tr *tracer) newNonRecordingSpan(sc trace.SpanContext) (otelcSpan nonRecordingSpan) {
	defer func() {
		traceContextAddSpan(otelcSpan)
	}()
	return
}

func (s *recordingSpan) End(options ...trace.SpanEndOption) {
	traceContextDelSpan(s)
}

func (otelcS nonRecordingSpan) End(options ...trace.SpanEndOption) {
	traceContextDelSpan(otelcS)
}
