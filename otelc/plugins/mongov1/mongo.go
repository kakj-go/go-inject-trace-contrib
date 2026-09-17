//go:build goinject

//inject:go.mongodb.org/mongo-driver/mongo
package mongo

// Ported from go.opentelemetry.io/otelc instrumentation/go.mongodb.org/mongo-driver/mongo
// (client_hook.go + otelc.yaml rules mongodb_hook_connect/_newclient): appends a
// trailing ClientOptions carrying the OTel CommandMonitor (contrib otelmongo),
// chained with any monitor the caller already configured. The monitor is
// appended rather than merged in place because options.MergeClientOptions takes
// the last non-nil field value — a trailing element can never override, or be
// overridden by, caller-supplied options.

import (
	"context"

	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"

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
	// Set only Monitor, and carry the caller's effective HTTPClient forward, so
	// the appended element is a no-op for every field except Monitor.
	injected := &options.ClientOptions{Monitor: monitor, HTTPClient: merged.HTTPClient}
	// Full slice expression forces a new backing array so this never mutates a
	// caller-owned slice passed in via `opts...`.
	return append(opts[:len(opts):len(opts)], injected)
}

func Connect(ctx context.Context, opts ...*options.ClientOptions) (*Client, error) {
	if hooksupport.Instrumented("MONGODB") {
		opts = otelcMongoInjectMonitor(opts)
	}
	return nil, nil
}

func NewClient(opts ...*options.ClientOptions) (*Client, error) {
	if hooksupport.Instrumented("MONGODB") {
		opts = otelcMongoInjectMonitor(opts)
	}
	return nil, nil
}
