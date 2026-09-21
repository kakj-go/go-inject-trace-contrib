//go:build goinject || generate

// Package skywalking aggregates all tracing rule packages. Blank import it
// from a goinject-tagged registration file and build with
// `go build -toolexec=go-inject` to activate instrumentation.
package skywalking

import (
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/bootinit"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/coreinit"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/amqp"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/dubbo"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/echov4"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/fasthttp"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/fiber"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/gin"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/go-elasticsearchv8"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/go-redisv9"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/go-restfulv3"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/goframe"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/gorm"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/grpc"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/http"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/irisv12"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/kratosv2"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/microv4"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/mongo"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/mux"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/pprof"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/pulsar"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/rocketmq"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/segmentio-kafka"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/sql"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/plugins/toolkit-activation"
	_ "github.com/kakj-go/go-inject-trace-contrib/skywalking/runtime"
)
