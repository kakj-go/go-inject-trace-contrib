package skywalking

// activate keeps github.com/apache/skywalking-go/plugins/core in the
// business dependency graph so the coreinit rule can inject its tracer
// constructor. Import this package normally from application code:
//
//	import _ "github.com/kakj-go/go-inject-trace-contrib/skywalking"
//
// The import is inert in builds without go-inject.
import (
	_ "github.com/apache/skywalking-go/plugins/core"

	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/runtimemetrics"
)
