//go:build !goinject

// stub keeps this rule package compilable in normal builds; the goinject
// build parses runtime2_add.go and proc_propagate.go as templates against
// the real runtime package.
package runtime
