// Adapted from go.opentelemetry.io/otelc test/apps/ginserver: a minimal gin
// server. The gin instrumentation enriches the net/http server span with the
// matched route (span renamed to "METHOD /route", http.route attribute) and
// records c.Error() values when the middleware chain unwinds. Self-driving:
// requests /hello/world, /error, and /status/500 from 127.0.0.1 at startup.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
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

	r.GET("/error", func(c *gin.Context) {
		_ = c.Error(fmt.Errorf("gin context error"))
		c.JSON(http.StatusOK, gin.H{"error": "recorded"})
	})

	r.GET("/status/:code", func(c *gin.Context) {
		code, err := strconv.Atoi(c.Param("code"))
		if err != nil || code < 100 || code > 599 {
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(code)
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
		for _, path := range []string{"/hello/world", "/error", "/status/500"} {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s%s", *port, path))
			if err != nil {
				log.Printf("self request to %s failed: %v", path, err)
				continue
			}
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}
