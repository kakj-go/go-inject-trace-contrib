//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects goframe.go, which is parsed as an injection template and
// checked against the real github.com/gogf/gf/v2/net/ghttp package instead.
package goframe

type Server struct {
}

func (srv *Server) ServeHTTP(w interface{}, r interface{}) {}
