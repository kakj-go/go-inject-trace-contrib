//go:build goinject

//inject:go-micro.dev/v4
package microv4

import (
	"go-micro.dev/v4/client"
)

func NewService(opts ...Option) Service {
	// NewServiceInterceptor.BeforeInvoke: append the tracing client wrapper
	// to the service options before they are applied.
	opts = append(opts, WrapClient(client.NewClientWrapper))
}
