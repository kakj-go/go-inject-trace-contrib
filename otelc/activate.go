// activate keeps the instrumented OpenTelemetry packages in the business
// module's dependency graph so the package-targeting rules (otelroot,
// oteltrace, sdktrace) resolve during go-inject builds. Inert without go-inject.
package otelc

import (
	_ "go.opentelemetry.io/otel"
	_ "go.opentelemetry.io/otel/trace"

	_ "go.opentelemetry.io/otel/sdk/trace"
)
