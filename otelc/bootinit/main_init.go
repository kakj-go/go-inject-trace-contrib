//go:build goinject

//inject:main
package bootinit

import "github.com/kakj-go/go-inject-trace-contrib/otelc/boot"

//inject:add
func init() {
	// Mirrors go.opentelemetry.io/otelc instrumentation/go.opentelemetry.io/otel/init/init_otelsdk.go:
	// initialize the global SDK (tracer/meter/logger providers from OTEL_* env),
	// then start runtime metrics (gated by OTEL_GO_ENABLED/DISABLED_INSTRUMENTATIONS).
	boot.SetupOTelSDK()
	boot.StartRuntimeMetrics()
}
