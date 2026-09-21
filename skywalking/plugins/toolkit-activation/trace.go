//go:build goinject

//inject:github.com/apache/skywalking-go/toolkit/trace
package toolkitactivation

import (
	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type SpanRef struct {
	//inject:add
	swSpan tracing.Span
}

func CreateEntrySpan(operationName string, extractor ExtractorRef) (swRet *SpanRef, swErr error) {
	defer func() {
		var swExtractor func(headerKey string) (string, error) = extractor
		s, err := tracing.CreateEntrySpan(operationName, swExtractor)
		if err != nil {
			swRet, swErr = nil, err
			return
		}
		if swRet != nil {
			swRet.swSpan = s
		}
	}()
	return
}

func CreateExitSpan(operationName string, peer string, injector InjectorRef) (swRet *SpanRef, swErr error) {
	defer func() {
		var swInjector func(headerKey, headerValue string) error = injector
		s, err := tracing.CreateExitSpan(operationName, peer, swInjector)
		if err != nil {
			swRet, swErr = nil, err
			return
		}
		if swRet != nil {
			swRet.swSpan = s
		}
	}()
	return
}

func CreateLocalSpan(operationName string) (swRet *SpanRef, swErr error) {
	defer func() {
		s, err := tracing.CreateLocalSpan(operationName)
		if err != nil {
			swRet, swErr = nil, err
			return
		}
		if swRet != nil {
			swRet.swSpan = s
		}
	}()
	return
}

func StopSpan() {
	if span := tracing.ActiveSpan(); span != nil {
		span.End()
	}
}

// The toolkit stub bodies return nil / "" — because a terminal bare return
// falls through to the original body, a direct assignment would be clobbered
// by the stub's return. The defer pattern (as used throughout metric.go)
// runs after the original body and wins.
func CaptureContext() (swRet ContextSnapshotRef) {
	defer func() {
		swRet = tracing.CaptureContext()
	}()
	return
}

func ContinueContext(ctx ContextSnapshotRef) {
	if ctx != nil {
		tracing.ContinueContext(ctx.(tracing.ContextSnapshot))
	}
}

func GetTraceID() (swRet string) {
	if span := tracing.ActiveSpan(); span != nil {
		swRet = span.TraceID()
		return
	}
	return
}

func GetSegmentID() (swRet string) {
	if span := tracing.ActiveSpan(); span != nil {
		swRet = span.TraceSegmentID()
		return
	}
	return
}

func GetSpanID() (swRet int32) {
	if span := tracing.ActiveSpan(); span != nil {
		swRet = span.SpanID()
		return
	}
	return
}

func SetOperationName(name string) {
	if span := tracing.ActiveSpan(); span != nil {
		span.SetOperationName(name)
	}
}

func SetTag(key string, value string) {
	if span := tracing.ActiveSpan(); span != nil {
		span.Tag(key, value)
	}
}

func AddLog(logs ...string) {
	if span := tracing.ActiveSpan(); span != nil {
		span.Log(logs...)
	}
}

func AddEvent(et EventType, event string) {
	if span := tracing.ActiveSpan(); span != nil {
		if event == "" {
			event = swDefaultEventMsg
		}
		span.Log(string(et), event)
	}
}

func GetCorrelation(key string) (swRet string) {
	defer func() {
		swRet = tracing.GetCorrelationContextValue(key)
	}()
	return
}

func SetCorrelation(key string, value string) {
	tracing.SetCorrelationContextValue(key, value)
}

func SetComponent(componentID int32) {
	if span := tracing.ActiveSpan(); span != nil {
		span.SetComponent(componentID)
	}
}

func Error(msgs ...string) {
	if span := tracing.ActiveSpan(); span != nil {
		span.Error(msgs...)
	}
}

// SpanRef methods: the official interceptors fetch the span from the
// receiver's dynamic field without a nil check for SetTag/AddLog/AddEvent/
// PrepareAsync/AsyncFinish (an unset field panics, as the unchecked type
// assertion did), while End and SetOperationName use the checked
// spanFromRef helper.

func (s *SpanRef) End() {
	defer func() {
		if s.swSpan != nil {
			s.swSpan.End()
		}
	}()
}

func (s *SpanRef) SetOperationName(name string) {
	defer func() {
		if s.swSpan != nil {
			s.swSpan.SetOperationName(name)
		}
	}()
}

func (s *SpanRef) SetTag(key string, value string) {
	defer func() {
		s.swSpan.Tag(key, value)
	}()
}

func (s *SpanRef) AddLog(logs ...string) {
	defer func() {
		s.swSpan.Log(logs...)
	}()
}

func (s *SpanRef) AddEvent(et EventType, event string) {
	defer func() {
		if event == "" {
			event = swDefaultEventMsg
		}
		s.swSpan.Log(string(et), event)
	}()
}

func (s *SpanRef) PrepareAsync() {
	defer func() {
		s.swSpan.PrepareAsync()
	}()
}

func (s *SpanRef) AsyncFinish() {
	defer func() {
		s.swSpan.AsyncFinish()
	}()
}

//inject:add
const swDefaultEventMsg = "unsetEvent"
