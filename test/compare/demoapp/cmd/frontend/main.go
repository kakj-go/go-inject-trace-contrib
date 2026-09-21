// demoapp frontend: gin HTTP server that fans a single /api/order request out
// to MySQL, Redis, a downstream HTTP service and a downstream gRPC service,
// plus one cross-goroutine span. Built twice — with the `sky` or `otel` build
// tag selecting the instrumentation backend — so both telemetry backends see
// the exact same request graph.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	v9 "github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"demoapp/api"
)

const (
	httpBackend = "http://127.0.0.1:8081"
	grpcBackend = "127.0.0.1:9090"
)

var (
	db    *sql.DB
	rdb   *v9.Client
	gconn *grpc.ClientConn
)

func main() {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "user:password@tcp(127.0.0.1:33061)/demo"
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:63791"
	}

	var err error
	for i := 0; i < 30; i++ {
		db, err = sql.Open("mysql", dsn)
		if err == nil {
			err = db.Ping()
		}
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	db.SetMaxOpenConns(10)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS orders (
		id INT AUTO_INCREMENT PRIMARY KEY,
		sku VARCHAR(64) NOT NULL,
		qty INT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		log.Fatalf("create orders: %v", err)
	}

	rdb = v9.NewClient(&v9.Options{Addr: redisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}

	gconn, err = grpc.NewClient(grpcBackend, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("grpc client: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.GET("/api/order", handleOrder)
	engine.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })

	slog.Info("frontend up", "addr", ":8080")
	log.Println("frontend listening on :8080")
	if err := engine.Run(":8080"); err != nil {
		log.Fatalf("frontend: %v", err)
	}
}

func handleOrder(c *gin.Context) {
	ctx := c.Request.Context()
	sku := c.DefaultQuery("sku", "SKU-42")
	slog.Info("order received", "sku", sku)

	// 1. persist the order (MySQL via database/sql)
	res, err := db.ExecContext(ctx, "INSERT INTO orders (sku, qty) VALUES (?, ?)", sku, 1)
	if err != nil {
		c.String(500, "insert order: %v", err)
		return
	}
	orderID, _ := res.LastInsertId()

	// 2. cache it (Redis)
	if err := rdb.Set(ctx, fmt.Sprintf("order:%d", orderID), sku, time.Minute).Err(); err != nil {
		c.String(500, "redis set: %v", err)
		return
	}

	// 3. ask the backend HTTP service for stock info (trace propagates via headers)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, httpBackend+"/api/inventory?sku="+sku, nil)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		c.String(500, "inventory http: %v", err)
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// 4. reserve stock over gRPC (trace propagates via metadata)
	gctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rres, err := api.NewInventoryClient(gconn).Reserve(gctx, &api.ReserveRequest{Sku: sku, Quantity: 1})
	if err != nil {
		c.String(500, "reserve grpc: %v", err)
		return
	}

	// 5. one cross-goroutine span: refresh the cache in the background
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := rdb.Get(ctx, fmt.Sprintf("order:%d", orderID)).Err(); err != nil {
			slog.Warn("cache refresh failed", "err", err)
		}
	}()
	wg.Wait()

	c.String(200, fmt.Sprintf("order=%d sku=%s stock=%s reserve=%v", orderID, sku, string(body), rres.GetOk()))
}
