//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects service.go, client.go, server.go and socket.go, which are
// parsed as injection templates and checked against the real go-micro.dev/v4
// packages instead.
package microv4
