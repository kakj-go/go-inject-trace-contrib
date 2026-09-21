module logslibs

go 1.25

require (
	github.com/kakj-go/go-inject-trace-contrib v0.0.0
	github.com/sirupsen/logrus v1.9.4
	go.opentelemetry.io/otel v1.46.0
	go.opentelemetry.io/otel/sdk v1.46.0
	go.opentelemetry.io/otel/trace v1.46.0
	go.uber.org/zap v1.21.0
)

require (
	go.opentelemetry.io/contrib/exporters/autoexport v0.71.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/runtime v0.71.0 // indirect
	go.opentelemetry.io/contrib/propagators/autoprop v0.71.0 // indirect
	go.opentelemetry.io/otel/log v0.22.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.22.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.46.0 // indirect
)

replace github.com/kakj-go/go-inject-trace-contrib => /contrib
