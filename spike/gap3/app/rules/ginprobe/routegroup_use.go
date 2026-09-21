//inject:github.com/gin-gonic/gin
package gin

type RouterGroup struct {
	engine *Engine
}

func (group *RouterGroup) calculateAbsolutePath(relativePath string) string {
	group.engine.goInjectProbe.Add(1)
}
