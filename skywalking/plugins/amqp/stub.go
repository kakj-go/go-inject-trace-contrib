//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects amqp.go, which is parsed as an injection template and checked
// against the real github.com/rabbitmq/amqp091-go package instead.
package amqp
