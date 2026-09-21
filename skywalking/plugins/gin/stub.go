//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects gin.go, which is parsed as an injection template and checked
// against the real github.com/gin-gonic/gin package instead.
package gin

type Context struct {
}

func (c *Context) Next() {}
