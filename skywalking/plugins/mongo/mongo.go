//go:build goinject

//inject:go.mongodb.org/mongo-driver/mongo
package mongo

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/apache/skywalking-go/plugins/core/log"
	"github.com/apache/skywalking-go/plugins/core/tools"
	"github.com/apache/skywalking-go/plugins/core/tracing"
)

// NewClient mirrors the official mongo/mongo NewClientInterceptor: it wraps
// the CommandMonitor of every client option that carries hosts (chaining to a
// previously configured monitor) so each command event builds a span.
func NewClient(opts ...*options.ClientOptions) (client *Client, err error) {
	swSyncMap := tools.NewSyncMap()
	for _, swOpt := range opts {
		swHosts := swOpt.Hosts
		swHostLength := len(swHosts)
		// must contains host
		if swHostLength == 0 {
			continue
		}
		swConfiguredMonitor := swOpt.Monitor

		// overwrite monitor, if define multiple opts, it should only keep the latest on the mongo client
		swOpt.Monitor = &event.CommandMonitor{
			Started: func(ctx context.Context, startedEvent *event.CommandStartedEvent) {
				if swConfiguredMonitor != nil {
					swConfiguredMonitor.Started(ctx, startedEvent)
				}
				swHost := swHosts[0]
				if swHostLength > 1 {
					if swInfoSplit := strings.Index(startedEvent.ConnectionID, "["); swInfoSplit > 0 && strings.HasSuffix(startedEvent.ConnectionID, "]") {
						swHost = startedEvent.ConnectionID[0:swInfoSplit]
					}
				}
				swSpan, swErr := tracing.CreateExitSpan("MongoDB/"+startedEvent.CommandName, swHost, func(headerKey, headerValue string) error {
					return nil
				}, tracing.WithComponent(42),
					tracing.WithLayer(tracing.SpanLayerDatabase),
					tracing.WithTag(tracing.TagDBType, "MongoDB"))
				if swErr != nil {
					log.Warnf("cannot create exit span on mongo client: %v", swErr)
					return
				}

				if swMongoCollectStatement {
					swSpan.Tag(tracing.TagDBStatement, swGettingStatements(startedEvent))
				}

				// Succeeded/Failed may fire on a DIFFERENT goroutine, so the
				// completion goes through the async machinery; End() also pops
				// the span off this goroutine's active stack immediately.
				swSpan.PrepareAsync()
				swSpan.End()
				swSyncMap.Put(fmt.Sprintf("%d", startedEvent.RequestID), swSpan)
			},
			Succeeded: func(ctx context.Context, succeededEvent *event.CommandSucceededEvent) {
				if swConfiguredMonitor != nil {
					swConfiguredMonitor.Succeeded(ctx, succeededEvent)
				}
				if swSpan, ok := swSyncMap.Remove(fmt.Sprintf("%d", succeededEvent.RequestID)); ok && swSpan != nil {
					swSpan.(tracing.Span).AsyncFinish()
				}
			},
			Failed: func(ctx context.Context, failedEvent *event.CommandFailedEvent) {
				if swConfiguredMonitor != nil {
					swConfiguredMonitor.Failed(ctx, failedEvent)
				}
				if swSpan, ok := swSyncMap.Remove(fmt.Sprintf("%d", failedEvent.RequestID)); ok && swSpan != nil {
					swSpan.(tracing.Span).Error(failedEvent.Failure)
					swSpan.(tracing.Span).AsyncFinish()
				}
			},
		}
	}
	return
}

//inject:add
var swRemoveFieldsInStmt = map[string]*struct{}{
	"lsid":         nil,
	"$clusterTime": nil,
	"txnNumber":    nil,
}

//inject:add
func swGettingStatements(startedEvent *event.CommandStartedEvent) string {
	swRows := make(bson.RawElement, 0)
	swElements, swErr := startedEvent.Command.Elements()
	if swErr != nil {
		return ""
	}
	for _, swElement := range swElements {
		if _, ok := swRemoveFieldsInStmt[swElement.Key()]; !ok {
			swRows = append(swRows, swElement...)
		}
	}
	return swRows.String()
}

//inject:add
var swMongoCollectStatement = func() bool {
	if v := os.Getenv("SW_AGENT_PLUGIN_CONFIG_MONGO_COLLECT_STATEMENT"); v != "" {
		if b, e := strconv.ParseBool(v); e == nil {
			return b
		}
	}
	return false
}()
