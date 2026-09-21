//go:build goinject

//inject:runtime/runtime2.go
package runtime

import _ "unsafe"

type g struct {
	//inject:add
	skywalking_tls interface{}
}

//inject:add
//go:linkname skywalking_get_gls skywalking_get_gls
var skywalking_get_gls = skywalkingTLSGetImpl

//inject:add
//go:nosplit
func skywalkingTLSGetImpl() interface{} { return getg().m.curg.skywalking_tls }

//inject:add
//go:linkname skywalking_set_gls skywalking_set_gls
var skywalking_set_gls = skywalkingTLSSetImpl

//inject:add
//go:nosplit
func skywalkingTLSSetImpl(v interface{}) { getg().m.curg.skywalking_tls = v }

//inject:add
//go:linkname skywalking_get_goid skywalking_get_goid
var skywalking_get_goid = skywalkingGoIDImpl

//inject:add
//go:nosplit
func skywalkingGoIDImpl() int64 { return int64(getg().m.curg.goid) }

//inject:add
//go:linkname skywalking_goroutine_change skywalking_goroutine_change
var skywalking_goroutine_change = skywalkingGoroutineChange

//inject:add
func skywalkingGoroutineChange(tls interface{}) interface{} {
	if tls == nil {
		return nil
	}
	if taker, ok := tls.(interface {
		TakeSnapShot() interface{}
	}); ok && taker != nil {
		return taker.TakeSnapShot()
	}
	return tls
}
