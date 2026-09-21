//go:build (goinject || generate) && otel

package main

// rule registration for go-inject: enables the otelc rule templates.
import _ "github.com/kakj-go/go-inject-trace-contrib/otelc"
