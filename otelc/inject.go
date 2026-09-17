//go:build goinject || generate

// Package otelc aggregates all OpenTelemetry compile-time instrumentation rule
// packages (ported from go.opentelemetry.io/otelc). Import it from the
// application's go-inject registration file and build with
// `go build -toolexec=go-inject`.
package otelc

import (
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/bootinit"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/otelroot"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/oteltrace"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/amqp"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/anthropic"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/awssdkv2"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/dbsql"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/elasticv7"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/gin"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/gocqlv2"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/grpc"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/k8sclientgo"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/kafkago"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/linodego"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/logrus"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/logslog"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/logstdlib"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/mongov1"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/mongov2"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/nethttp"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/openaiv1"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/openaiv2"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/openaiv3"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/redisv9"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/plugins/zap"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/runtime"
	_ "github.com/kakj-go/go-inject-trace-contrib/otelc/sdktrace"
)
