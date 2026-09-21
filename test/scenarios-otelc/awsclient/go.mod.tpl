module awsclient

go 1.25

require (
	github.com/aws/aws-sdk-go-v2 v1.44.0
	github.com/aws/aws-sdk-go-v2/config v1.29.14
	github.com/aws/aws-sdk-go-v2/credentials v1.18.12
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.50.4
	github.com/kakj-go/go-inject-trace-contrib v0.0.0
	go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws v0.71.0 // indirect
	go.opentelemetry.io/otel v1.46.0
	go.opentelemetry.io/otel/sdk v1.46.0
	go.opentelemetry.io/otel/trace v1.46.0
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
