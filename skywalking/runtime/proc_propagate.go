//go:build goinject && go1.24 && !go1.28

//inject:runtime/proc.go
//inject:id skywalking-gls-propagation
package runtime

func newproc1(fn *funcval, callergp *g, callerpc uintptr, parked bool, waitreason waitReason) (newg *g) {
	defer func() {
		newg.skywalking_tls = skywalkingGoroutineChange(callergp.skywalking_tls)
	}()
	return
}
