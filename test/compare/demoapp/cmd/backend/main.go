// demoapp backend: plain net/http inventory service plus a gRPC Inventory
// service, both touching MySQL and Redis. Same dual-backend build scheme as
// the frontend (sky / otel build tags).
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	v9 "github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	"demoapp/api"
)

var (
	db  *sql.DB
	rdb *v9.Client
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

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS inventory (
		sku VARCHAR(64) PRIMARY KEY,
		stock INT NOT NULL)`); err != nil {
		log.Fatalf("create inventory: %v", err)
	}
	if _, err := db.Exec("INSERT IGNORE INTO inventory (sku, stock) VALUES ('SKU-42', 100)"); err != nil {
		log.Fatalf("seed inventory: %v", err)
	}

	rdb = v9.NewClient(&v9.Options{Addr: redisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}

	// HTTP inventory endpoint
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/inventory", func(w http.ResponseWriter, r *http.Request) {
			sku := r.URL.Query().Get("sku")
			var stock int
			if err := db.QueryRowContext(r.Context(), "SELECT stock FROM inventory WHERE sku = ?", sku).Scan(&stock); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, "%d", stock)
		})
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
		log.Println("backend http listening on :8081")
		if err := http.ListenAndServe(":8081", mux); err != nil {
			log.Fatalf("backend http: %v", err)
		}
	}()

	// gRPC Inventory service
	ln, err := net.Listen("tcp", ":9090")
	if err != nil {
		log.Fatalf("grpc listen: %v", err)
	}
	gs := grpc.NewServer()
	api.RegisterInventoryServer(gs, &inventoryServer{})
	slog.Info("backend up", "grpc", ":9090")
	log.Println("backend grpc listening on :9090")
	if err := gs.Serve(ln); err != nil {
		log.Fatalf("backend grpc: %v", err)
	}
}

type inventoryServer struct {
	api.UnimplementedInventoryServer
}

func (s *inventoryServer) Reserve(ctx context.Context, req *api.ReserveRequest) (*api.ReserveResponse, error) {
	sku := req.GetSku()

	if err := rdb.Set(ctx, "reserve:"+sku, fmt.Sprintf("%d", req.GetQuantity()), time.Minute).Err(); err != nil {
		return nil, err
	}

	res, err := db.ExecContext(ctx, "UPDATE inventory SET stock = stock - ? WHERE sku = ?", req.GetQuantity(), sku)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	slog.Info("reserve done", "sku", sku, "rows", n)
	return &api.ReserveResponse{Ok: n > 0, Message: fmt.Sprintf("reserved %d of %s", req.GetQuantity(), sku)}, nil
}
