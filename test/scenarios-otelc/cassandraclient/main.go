// Cassandra scenario: creates a keyspace + table, inserts and selects one row,
// producing query observer spans (SELECT/CREATE/INSERT) keyed by keyspace.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var (
	port     = flag.String("port", "8080", "The HTTP health port")
	addr     = flag.String("addr", "cassandra-server", "The Cassandra host")
	cport    = flag.Int("cport", 9042, "The Cassandra port")
	keyspace = flag.String("keyspace", "testks", "The keyspace to use")
)

var ready atomic.Bool

func main() {
	flag.Parse()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", fmt.Sprintf(":%s", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	go func() {
		if err := http.Serve(ln, nil); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	go func() {
		time.Sleep(500 * time.Millisecond)
		runCassandra()
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}

func runCassandra() {
	cluster := gocql.NewCluster(*addr)
	cluster.Port = *cport
	cluster.Keyspace = *keyspace
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 10 * time.Second

	// Bootstrap the keyspace through a session without one.
	bootstrap := *cluster
	bootstrap.Keyspace = ""
	session, err := bootstrap.CreateSession()
	if err != nil {
		log.Printf("bootstrap session failed: %v", err)
		return
	}
	if err := session.Query(fmt.Sprintf(
		"CREATE KEYSPACE IF NOT EXISTS %s WITH replication = {'class':'SimpleStrategy','replication_factor':1}", *keyspace)).Exec(); err != nil {
		log.Printf("create keyspace failed: %v", err)
	}
	session.Close()

	s, err := cluster.CreateSession()
	if err != nil {
		log.Printf("session failed: %v", err)
		return
	}
	defer s.Close()

	if err := s.Query("CREATE TABLE IF NOT EXISTS items (id int PRIMARY KEY, name text)").Exec(); err != nil {
		log.Printf("create table failed: %v", err)
	}
	if err := s.Query("INSERT INTO items (id, name) VALUES (1, 'otelc')").Exec(); err != nil {
		log.Printf("insert failed: %v", err)
	}
	var name string
	if err := s.Query("SELECT name FROM items WHERE id = 1").Scan(&name); err != nil {
		log.Printf("select failed: %v", err)
	}
}
