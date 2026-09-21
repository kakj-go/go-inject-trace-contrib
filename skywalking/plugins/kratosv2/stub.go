//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects http.go and grpc.go, which are parsed as injection templates
// and checked against the real go-kratos/kratos/v2 transport packages
// instead.
package kratosv2
