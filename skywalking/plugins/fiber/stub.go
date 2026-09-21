//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects fiber.go, which is parsed as an injection template and
// checked against the real github.com/gofiber/fiber/v2 package instead.
package fiber

type App struct {
}

func (app *App) handler(ctx interface{}) {}
