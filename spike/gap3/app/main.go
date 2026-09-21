package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/hello/:name", func(c *gin.Context) {
		c.String(http.StatusOK, "hello %s", c.Param("name"))
	})
	go func() { _ = router.Run("127.0.0.1:18099") }()
	resp, err := http.Get("http://127.0.0.1:18099/hello/gap1")
	if err != nil {
		fmt.Println("request failed:", err)
		return
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	fmt.Printf("status=%d body=%s\n", resp.StatusCode, buf[:n])
}
