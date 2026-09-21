//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects mongo.go, which is parsed as an injection template and
// checked against the real go.mongodb.org/mongo-driver/mongo package
// instead.
package mongo
