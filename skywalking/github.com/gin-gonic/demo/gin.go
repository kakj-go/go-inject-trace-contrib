//inject:github.com/gin-gonic/gin/gin.go
package gin

import (
	"fmt"
	"github.com/gin-gonic/gin"
)

type Engine struct {
}

func (engine *Engine) handleHTTPRequest(c *gin.Context) {
	fmt.Println("before request")
	defer fmt.Println("after request")
}
