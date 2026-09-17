//go:build !goinject

// stub keeps this rule package compilable in normal builds; the goinject
// build parses gls.go as a template against the real
// go.opentelemetry.io/otel/sdk/trace package.
package trace
