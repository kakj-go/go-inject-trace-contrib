//go:build goinject

//inject:github.com/apache/skywalking-go/plugins/core
package core

//inject:add
// NewTracerForInject exposes the package-internal unbooted tracer constructor
// (DiscardReporter + ConstSampler(false) + default logger) so the external
// boot package can initialize it the same way the official agent core does.
func NewTracerForInject() *Tracer { return newTracer() }
