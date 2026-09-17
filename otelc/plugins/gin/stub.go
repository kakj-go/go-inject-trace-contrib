//go:build !goinject

// stub keeps this rule package compilable in normal builds; the goinject
// build parses gin.go as a template against the real gin package.
package gin
