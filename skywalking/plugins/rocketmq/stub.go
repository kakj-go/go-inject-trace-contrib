//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects producer.go and consumer.go, which are parsed as injection
// templates and checked against the real
// github.com/apache/rocketmq-client-go/v2 packages instead.
package rocketmq
