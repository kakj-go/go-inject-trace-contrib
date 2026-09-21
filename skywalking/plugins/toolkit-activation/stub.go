//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects trace.go, metric.go and logging.go, which are parsed as
// injection templates and checked against the real
// github.com/apache/skywalking-go/toolkit packages instead.
package toolkitactivation
