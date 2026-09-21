//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects hostclient.go and router.go, which are parsed as injection
// templates and checked against the real github.com/valyala/fasthttp and
// github.com/fasthttp/router packages instead.
package fasthttp

type HostClient struct {
}

func (hc *HostClient) Do(req interface{}, resp interface{}) (err error) { return }

type Router struct {
}

func (r *Router) Handler(ctx interface{}) {}
