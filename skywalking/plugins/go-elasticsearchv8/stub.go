//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects elasticsearch.go, which is parsed as an injection template
// and checked against the real github.com/elastic/go-elasticsearch/v8
// (package elasticsearch) instead.
package goelasticsearchv8
