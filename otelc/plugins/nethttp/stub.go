//go:build !goinject

// stub keeps this rule package compilable in normal builds; the goinject
// build parses http.go as a template against the real net/http package.
package nethttp
