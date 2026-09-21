//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects the template files (gorm.go for gorm.io/gorm, mysql.go for
// gorm.io/driver/mysql, postgres.go for gorm.io/driver/postgres), which are
// parsed as injection templates and checked against the real target packages
// instead.
package gorm
