// Adapted from go.opentelemetry.io/otelc test/apps/ginserverhttpclient: a gin
// server whose /dorequest handler issues an HTTP client request back to this
// same server, exercising the gin enrichment and both net/http hooks in one
// trace: gin server span (renamed to the route) -> client span -> server span.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var port = flag.String("port", "8080", "port to listen on")

var ready atomic.Bool

func main() {
	flag.Parse()
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	r.GET("/hello/:name", func(c *gin.Context) {
		name := c.Param("name")
		c.JSON(http.StatusOK, gin.H{"message": "Hello " + name})
	})

	r.GET("/dorequest", func(c *gin.Context) {
		url := fmt.Sprintf("http://127.0.0.1:%s/hello/world", *port)
		resp, err := http.Get(url)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.String(http.StatusOK, string(body))
	})

	r.GET("/health", func(c *gin.Context) {
		if !ready.Load() {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		c.Status(http.StatusOK)
	})

	ln, err := net.Listen("tcp", fmt.Sprintf(":%s", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	go func() {
		if err := r.RunListener(ln); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	go func() {
		time.Sleep(300 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/dorequest", *port))
		if err != nil {
			log.Printf("self request failed: %v", err)
		} else {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}
