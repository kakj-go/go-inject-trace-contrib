//go:build goinject

//inject:go.mongodb.org/mongo-driver/v2/mongo
package mongo

// Ported from go.opentelemetry.io/otelc instrumentation/go.mongodb.org/mongo-driver/v2/mongo
// (client_hook.go + otelc.yaml rule mongodb_v2_hook_connect): v2 removed the
// context parameter from Connect; the monitor injection is identical to v1.

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo"

	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

// otelcMongoChainMonitors invokes each non-nil callback of both user and otel
// monitors in turn (user first).
//
//inject:add
func otelcMongoChainMonitors(userMonitor, otel *event.CommandMonitor) *event.CommandMonitor {
	return &event.CommandMonitor{
		Started: func(ctx context.Context, e *event.CommandStartedEvent) {
			if userMonitor.Started != nil {
				userMonitor.Started(ctx, e)
			}
			if otel.Started != nil {
				otel.Started(ctx, e)
			}
		},
		Succeeded: func(ctx context.Context, e *event.CommandSucceededEvent) {
			if userMonitor.Succeeded != nil {
				userMonitor.Succeeded(ctx, e)
			}
			if otel.Succeeded != nil {
				otel.Succeeded(ctx, e)
			}
		},
		Failed: func(ctx context.Context, e *event.CommandFailedEvent) {
			if userMonitor.Failed != nil {
				userMonitor.Failed(ctx, e)
			}
			if otel.Failed != nil {
				otel.Failed(ctx, e)
			}
		},
	}
}

//inject:add
func otelcMongoInjectMonitor(opts []*options.ClientOptions) []*options.ClientOptions {
	merged := options.MergeClientOptions(opts...)
	otelMonitor := otelmongo.NewMonitor()
	monitor := otelMonitor
	if merged.Monitor != nil {
		monitor = otelcMongoChainMonitors(merged.Monitor, otelMonitor)
	}
	// Set only Monitor: v2 ClientOptions has no HTTPClient field to carry
	// forward, so the injected element touches nothing else.
	injected := &options.ClientOptions{Monitor: monitor}
	// Full slice expression forces a new backing array so this never mutates a
	// caller-owned slice passed in via `opts...`.
	return append(opts[:len(opts):len(opts)], injected)
}

func Connect(opts ...*options.ClientOptions) (*Client, error) {
	if hooksupport.Instrumented("MONGODB_V2") {
		opts = otelcMongoInjectMonitor(opts)
	}
	return nil, nil
}
