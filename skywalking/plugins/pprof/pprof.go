//go:build goinject

//inject:runtime/pprof
package pprof

import (
	"context"

	"github.com/apache/skywalking-go/plugins/core/profile"
	"github.com/apache/skywalking-go/plugins/core/tracing"
)

func SetGoroutineLabels(ctx context.Context) {
	if sp := tracing.ActiveSpan(); sp != nil && sp.IsProfileTarget() {
		if now, ok := profile.CatchNowProfileLabel().(LabelSet); ok {
			ctx = WithLabels(ctx, now)
		}
	}
}
