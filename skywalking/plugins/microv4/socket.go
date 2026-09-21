//go:build goinject

//inject:go-micro.dev/v4/util/socket
package microv4

import (
	"sync"

	"go-micro.dev/v4/transport"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type Socket struct {
	//inject:add
	// SwInjectData is exported because the server-package rule reads it to
	// adopt the connection span (the official agent used the cross-package
	// EnhancedInstance dynamic field for the same purpose).
	SwInjectData *InjectData
}

//inject:add
type InjectData struct {
	Span     tracing.Span
	Snapshot tracing.ContextSnapshot
	// finished makes the AsyncFinish of the connection span one-shot under
	// concurrent Close calls (see Close below).
	finished sync.Once
}

func (s *Socket) Accept(m *transport.Message) (swErr error) {
	defer func() {
		// AcceptInterceptor.AfterInvoke: error != nil then ignore
		if swErr != nil {
			return
		}
		swSpan := tracing.ActiveSpan()
		if swSpan == nil {
			return
		}
		if s.SwInjectData != nil {
			return
		}

		swSpan.PrepareAsync()
		snapshot := tracing.CaptureContext()
		swSpan.End()
		s.SwInjectData = &InjectData{
			Span:     swSpan,
			Snapshot: snapshot,
		}
	}()
	return
}

func (s *Socket) Close() (swErr error) {
	defer func() {
		data := s.SwInjectData
		if data == nil {
			return
		}
		// one-shot under concurrent Close calls; the winner also clears the
		// injected data so a socket reused for a new connection gets a fresh
		// span instead of being blocked by the stale InjectData
		data.finished.Do(func() {
			data.Span.AsyncFinish()
			s.SwInjectData = nil
		})
	}()
	return
}
