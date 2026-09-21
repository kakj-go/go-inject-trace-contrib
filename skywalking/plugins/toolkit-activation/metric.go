//go:build goinject

//inject:github.com/apache/skywalking-go/toolkit/metric
package toolkitactivation

import (
	"github.com/apache/skywalking-go/plugins/core/metrics"
)

type CounterRef struct {
	//inject:add
	swCounter metrics.Counter
}

type GaugeRef struct {
	//inject:add
	swGauge metrics.Gauge
}

type HistogramRef struct {
	//inject:add
	swHistogram metrics.Histogram
}

func NewCounter(name string, opt ...MeterOpt) (swRet *CounterRef) {
	defer func() {
		var opts []metrics.Opt
		for _, o := range opt {
			opts = append(opts, o.(metrics.Opt))
		}
		swRet.swCounter = metrics.NewCounter(name, opts...)
	}()
	return
}

func (c *CounterRef) Get() (swRet float64) {
	defer func() {
		if c.swCounter != nil {
			swRet = c.swCounter.Get()
		}
	}()
	return
}

func (c *CounterRef) Inc(val float64) {
	defer func() {
		if c.swCounter != nil {
			c.swCounter.Inc(val)
		}
	}()
}

func NewGauge(name string, getter func() float64, opts ...MeterOpt) (swRet *GaugeRef) {
	defer func() {
		var swOpts []metrics.Opt
		for _, o := range opts {
			swOpts = append(swOpts, o.(metrics.Opt))
		}
		swRet.swGauge = metrics.NewGauge(name, getter, swOpts...)
	}()
	return
}

func (g *GaugeRef) Get() (swRet float64) {
	defer func() {
		if g.swGauge != nil {
			swRet = g.swGauge.Get()
		}
	}()
	return
}

func NewHistogram(name string, steps []float64, opts ...MeterOpt) (swRet *HistogramRef) {
	defer func() {
		var swOpts []metrics.Opt
		for _, o := range opts {
			swOpts = append(swOpts, o.(metrics.Opt))
		}
		swRet.swHistogram = metrics.NewHistogram(name, steps, swOpts...)
	}()
	return
}

func (h *HistogramRef) Observe(val float64) {
	defer func() {
		if h.swHistogram != nil {
			h.swHistogram.Observe(val)
		}
	}()
}

func (h *HistogramRef) ObserveWithCount(val float64, count int64) {
	defer func() {
		if h.swHistogram != nil {
			h.swHistogram.ObserveWithCount(val, count)
		}
	}()
}

func WithLabels(key, val string) (swRet MeterOpt) {
	defer func() {
		swRet = metrics.WithLabel(key, val)
	}()
	return
}
