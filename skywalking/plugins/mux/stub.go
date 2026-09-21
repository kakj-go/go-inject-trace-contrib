//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects mux.go, which is parsed as an injection template and checked
// against the real github.com/gorilla/mux package instead.
package mux

import "net/http"

type RouteMatch struct {
}

type Router struct {
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {}

func (r *Router) Match(req *http.Request, match *RouteMatch) (matched bool) { return }
