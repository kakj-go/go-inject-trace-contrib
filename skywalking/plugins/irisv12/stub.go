//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects irisv12.go, which is parsed as an injection template and
// checked against the real github.com/kataras/iris/v12/core/router package
// instead.
package irisv12

type routerHandler struct {
}

func (h *routerHandler) HandleRequest(ctx interface{}) {}
