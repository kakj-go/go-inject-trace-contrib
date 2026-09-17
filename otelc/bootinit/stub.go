//go:build !goinject

// stub keeps this rule package compilable in normal builds; the goinject
// build parses main_init.go as a template targeting the application's main
// package.
package bootinit
