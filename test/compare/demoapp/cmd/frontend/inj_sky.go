//go:build (goinject || generate) && sky

package main

// rule registration for go-inject: enables the skywalking rule templates.
import _ "github.com/kakj-go/go-inject-trace-contrib/skywalking"
