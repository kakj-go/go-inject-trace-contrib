//go:build goinject

//inject:github.com/anthropics/anthropic-sdk-go
package anthropic

// Ported from go.opentelemetry.io/otelc instrumentation/github.com/anthropics/anthropic-sdk-go (hook.go):
// prepends the GenAI HTTP middleware (in otelc/genai, reached with a plain
// import — importing back into the SDK closes no cycle) to the client
// options so every API call gets a GenAI CLIENT span. The middleware's
// scope name keeps the per-version instrumentation identity.

import (
	"github.com/kakj-go/go-inject-trace-contrib/otelc/genai"
	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"

	"github.com/anthropics/anthropic-sdk-go/option"
)

func NewClient(opts ...option.RequestOption) (otelcClient Client) {
	if hooksupport.Instrumented("ANTHROPIC") {
		otelcOpts := make([]option.RequestOption, 0, len(opts)+1)
		otelcOpts = append(otelcOpts, option.WithMiddleware(genai.AnthropicMiddleware("go.opentelemetry.io/otelc/instrumentation/github.com/anthropics/anthropic-sdk-go")))
		otelcOpts = append(otelcOpts, opts...)
		opts = otelcOpts
	}
	return
}
