//go:build goinject

//inject:github.com/aws/aws-sdk-go-v2/config
package config

// Ported from go.opentelemetry.io/otelc instrumentation/github.com/aws/aws-sdk-go-v2
// (config_hook.go + otelc.yaml rule aws_sdk_v2_hook_loaddefaultconfig): appends
// the otelaws middlewares to the returned aws.Config.APIOptions so every client
// created from that config is traced.

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"

	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"

	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

func LoadDefaultConfig(ctx context.Context, optFns ...func(*LoadOptions) error) (otelcCfg aws.Config, otelcErr error) {
	defer func() {
		if otelcErr == nil && hooksupport.Instrumented("AWS_SDK_V2") {
			otelaws.AppendMiddlewares(&otelcCfg.APIOptions)
		}
	}()
	return
}
