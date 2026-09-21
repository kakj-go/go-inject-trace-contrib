//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects echov4.go, which is parsed as an injection template and
// checked against the real github.com/labstack/echo/v4 package instead.
package echov4

type Echo struct {
}

func New() (e *Echo) { return }
