//go:build otel

package main

// runtime-side import: keeps the otelc runtime packages linked in.
import _ "github.com/kakj-go/go-inject-trace-contrib/otelc"
