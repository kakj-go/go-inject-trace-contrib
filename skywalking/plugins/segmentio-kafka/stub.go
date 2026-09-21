//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects kafka.go, which is parsed as an injection template and checked
// against the real github.com/segmentio/kafka-go package instead.
package segmentiokafka
