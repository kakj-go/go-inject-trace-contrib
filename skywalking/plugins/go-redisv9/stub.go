//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects redis.go and op_type.go, which are parsed as injection
// templates and checked against the real github.com/redis/go-redis/v9
// package instead.
package goredisv9
