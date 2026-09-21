//go:build !goinject

// stub keeps this rule package compilable in normal builds. The goinject
// build selects go-restfulv3.go, which is parsed as an injection template
// and checked against the real github.com/emicklei/go-restful/v3 package
// instead.
package restfulv3

type WebService struct {
}

type Container struct {
}

func (c *Container) Add(service *WebService) *Container { return nil }

func (c *Container) Dispatch(httpWriter interface{}, httpRequest interface{}) {}

func (c *Container) HandleWithFilter(pattern string, handler interface{}) {}
