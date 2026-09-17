//go:build goinject

//inject:database/sql
package sql

// Ported from go.opentelemetry.io/otelc instrumentation/database/sql (client.go,
// parse.go, otelc.yaml): adds connection-metadata fields to DB/Tx/Conn/Stmt
// (filled by Open/OpenDB/Prepare propagation) and creates a CLIENT span for
// every statement operation. database/sql may import go.opentelemetry.io/otel
// directly — nothing in the otel API chain imports database/sql — so unlike
// net/http no link bridge is needed.

import (
	"context"
	"database/sql/driver"
	"reflect"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/kakj-go/go-inject-trace-contrib/otelc/dbsemconv"
	"github.com/kakj-go/go-inject-trace-contrib/otelc/dsnparse"
	"github.com/kakj-go/go-inject-trace-contrib/otelc/hooklog"
	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

//inject:add
const otelcDBInstrumentationName = "go.opentelemetry.io/otelc/instrumentation/database/sql"

//inject:add
const otelcDBInstrumentationKey = "DATABASE"

//inject:add
var otelcDBTracerOnce sync.Once

//inject:add
var otelcDBTracer trace.Tracer

//inject:add
func otelcDBGetTracer() trace.Tracer {
	otelcDBTracerOnce.Do(func() {
		otelcDBTracer = otel.GetTracerProvider().Tracer(
			otelcDBInstrumentationName,
			trace.WithInstrumentationVersion(hooksupport.ModuleVersion()),
		)
		hooklog.Logger().Info("DB client instrumentation initialized")
	})
	return otelcDBTracer
}

// otelcDBStart is the port of instrumentStart: it returns the started span
// (nil when instrumentation is disabled).
//
//inject:add
func otelcDBStart(ctx context.Context, spanName, query, endpoint, driverName, dsn, dbName string, args ...interface{}) trace.Span {
	if !hooksupport.Instrumented(otelcDBInstrumentationKey) {
		return nil
	}
	req := dbsemconv.DatabaseSqlRequest{
		OpType:     dbsemconv.OperationName(query),
		Sql:        query,
		Endpoint:   endpoint,
		DriverName: driverName,
		Dsn:        dsn,
		Params:     args,
		DbName:     dbName,
	}
	attrs := dbsemconv.DbClientRequestTraceAttrs(req)

	_, span := otelcDBGetTracer().Start(ctx,
		req.OpType,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
	return span
}

//inject:add
func otelcDBEnd(span trace.Span, err error) {
	if span == nil {
		return
	}
	defer span.End()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// otelcDBOpenInfo parses a driverName/dsn pair the way beforeOpenInstrumentation does.
//
//inject:add
func otelcDBOpenInfo(driverName, dataSourceName string) map[string]string {
	info := dsnparse.ParseDSN(driverName, dataSourceName)
	addr := info.Addr()
	if addr == "" {
		// Surface the omission rather than silently emitting server.address:
		// "unknown". The DSN is intentionally not logged as it may carry
		// credentials.
		if dataSourceName != "" {
			hooklog.Logger().Warn("could not determine server.address from DSN", "driver", driverName)
		}
		addr = "unknown"
	}
	dbName := info.DBName
	if dbName == "" && dataSourceName != "" {
		dbName = dsnparse.ParseDbName(dataSourceName)
	}
	return map[string]string{
		"endpoint": addr,
		"driver":   driverName,
		"dsn":      dataSourceName,
		"dbName":   dbName,
	}
}

// otelcDBApplyInfo is the port of afterOpenInstrumentation's field plumbing.
//
//inject:add
func otelcDBApplyInfo(db *DB, data map[string]string) {
	if db == nil || data == nil {
		return
	}
	if v, ok := data["endpoint"]; ok {
		db.Endpoint = v
	}
	if v, ok := data["driver"]; ok {
		db.DriverName = v
	}
	if v, ok := data["dsn"]; ok {
		db.DSN = v
	}
	if v, ok := data["dbName"]; ok {
		db.DbName = v
	}
}

// otelcDBResolveDriverName maps a driver.Driver instance back to the standard
// driver name recognized by ParseDSN and semconv.
//
//inject:add
func otelcDBResolveDriverName(d driver.Driver) string {
	if d == nil {
		return ""
	}
	dType := reflect.TypeOf(d)
	pkgPath := dType.PkgPath()
	if dType.Kind() == reflect.Pointer {
		pkgPath = dType.Elem().PkgPath()
	}
	typeStr := dType.String()

	switch {
	case strings.Contains(pkgPath, "go-sql-driver/mysql") || strings.Contains(typeStr, "MySQLDriver"):
		return "mysql"
	case strings.Contains(pkgPath, "jackc/pgx"):
		return "pgx"
	case strings.Contains(pkgPath, "lib/pq"):
		return "postgres"
	case strings.Contains(pkgPath, "mattn/go-sqlite3") || strings.Contains(pkgPath, "modernc.org/sqlite") || strings.Contains(typeStr, "SQLiteDriver") || strings.Contains(typeStr, "sqlite"):
		return "sqlite3"
	case strings.Contains(pkgPath, "microsoft/go-mssqldb") || strings.Contains(pkgPath, "denisenkom/go-mssqldb"):
		return "sqlserver"
	case strings.Contains(pkgPath, "ClickHouse/clickhouse-go"):
		return "clickhouse"
	case strings.Contains(pkgPath, "godror/godror") || strings.Contains(typeStr, "godror"):
		return "godror"
	case strings.Contains(pkgPath, "testdb") || strings.Contains(typeStr, "testdb") || strings.Contains(typeStr, "testDriver"):
		return "mysql"
	default:
		return typeStr
	}
}

// ---- struct field projections ----

type DB struct {
	//inject:add
	Endpoint string
	//inject:add
	DbName string
	//inject:add
	DriverName string
	//inject:add
	DSN string
}

type Stmt struct {
	//inject:add
	Data map[string]string
	//inject:add
	DriverName string
	//inject:add
	DSN string
}

type Tx struct {
	//inject:add
	Endpoint string
	//inject:add
	DbName string
	//inject:add
	DriverName string
	//inject:add
	DSN string
}

type Conn struct {
	//inject:add
	Endpoint string
	//inject:add
	DbName string
	//inject:add
	DriverName string
	//inject:add
	DSN string
}

// ---- interception templates ----

func Open(driverName, dataSourceName string) (otelcDB *DB, otelcErr error) {
	otelcInfo := otelcDBOpenInfo(driverName, dataSourceName)
	defer func() { otelcDBApplyInfo(otelcDB, otelcInfo) }()
	return
}

func OpenDB(c driver.Connector) (otelcDB *DB) {
	otelcInfo := map[string]string{"endpoint": "unknown", "driver": "", "dsn": "", "dbName": ""}
	if c != nil {
		otelcDriverName := ""
		if d := c.Driver(); d != nil {
			otelcDriverName = otelcDBResolveDriverName(d)
		}

		otelcDSN := ""
		type otelcDSNGetter interface{ DSN() string }
		type otelcDataSourceNameGetter interface{ DataSourceName() string }
		if g, ok := c.(otelcDSNGetter); ok {
			otelcDSN = g.DSN()
		} else if g, ok := c.(otelcDataSourceNameGetter); ok {
			otelcDSN = g.DataSourceName()
		}

		info := dsnparse.ParseDSN(otelcDriverName, otelcDSN)
		addr := info.Addr()
		if addr == "" {
			if otelcDSN != "" {
				hooklog.Logger().Warn("could not determine server.address from DSN", "driver", otelcDriverName)
			}
			addr = "unknown"
		}
		dbName := info.DBName
		if dbName == "" && otelcDSN != "" {
			dbName = dsnparse.ParseDbName(otelcDSN)
		}
		otelcInfo = map[string]string{
			"endpoint": addr,
			"driver":   otelcDriverName,
			"dsn":      otelcDSN,
			"dbName":   dbName,
		}
	}
	defer func() { otelcDBApplyInfo(otelcDB, otelcInfo) }()
	return
}

// ---- DB operations ----

func (db *DB) PingContext(ctx context.Context) (otelcErr error) {
	var otelcSpan trace.Span
	if db != nil {
		otelcSpan = otelcDBStart(ctx, "ping", "ping", db.Endpoint, db.DriverName, db.DSN, db.DbName)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}

func (db *DB) PrepareContext(ctx context.Context, query string) (otelcStmt *Stmt, otelcErr error) {
	if db != nil {
		otelcInfo := map[string]string{
			"endpoint": db.Endpoint,
			"sql":      query,
			"driver":   db.DriverName,
			"dsn":      db.DSN,
			"dbName":   db.DbName,
		}
		defer func() {
			if otelcStmt == nil {
				return
			}
			otelcStmt.Data = map[string]string{
				"endpoint": otelcInfo["endpoint"],
				"sql":      otelcInfo["sql"],
				"driver":   otelcInfo["driver"],
				"dbName":   otelcInfo["dbName"],
			}
			otelcStmt.DSN = otelcInfo["dsn"]
		}()
	}
	return
}

func (db *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (otelcResult Result, otelcErr error) {
	var otelcSpan trace.Span
	if db != nil {
		otelcSpan = otelcDBStart(ctx, "exec", query, db.Endpoint, db.DriverName, db.DSN, db.DbName, args...)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}

func (db *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (otelcRows *Rows, otelcErr error) {
	var otelcSpan trace.Span
	if db != nil {
		otelcSpan = otelcDBStart(ctx, "query", query, db.Endpoint, db.DriverName, db.DSN, db.DbName, args...)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}

func (db *DB) BeginTx(ctx context.Context, opts *TxOptions) (otelcTx *Tx, otelcErr error) {
	var otelcSpan trace.Span
	if hooksupport.Instrumented(otelcDBInstrumentationKey) && db != nil {
		otelcEndpoint, otelcDriver, otelcDSN, otelcDBName := db.Endpoint, db.DriverName, db.DSN, db.DbName
		otelcSpan = otelcDBStart(ctx, "begin", "START TRANSACTION", otelcEndpoint, otelcDriver, otelcDSN, otelcDBName)
		if otelcSpan != nil {
			defer func() {
				otelcDBEnd(otelcSpan, otelcErr)

				// Port of afterTxInstrumentation's metadata plumbing: the
				// returned transaction carries the connection info for its
				// own spans.
				if otelcTx == nil {
					return
				}
				otelcTx.Endpoint = otelcEndpoint
				otelcTx.DriverName = otelcDriver
				otelcTx.DSN = otelcDSN
				otelcTx.DbName = otelcDBName
			}()
		}
	}
	return
}

func (db *DB) Conn(ctx context.Context) (otelcConn *Conn, otelcErr error) {
	if db != nil {
		otelcInfo := map[string]string{
			"endpoint": db.Endpoint,
			"driver":   db.DriverName,
			"dsn":      db.DSN,
			"dbName":   db.DbName,
		}
		defer func() {
			if otelcConn == nil {
				return
			}
			if v, ok := otelcInfo["endpoint"]; ok {
				otelcConn.Endpoint = v
			}
			if v, ok := otelcInfo["driver"]; ok {
				otelcConn.DriverName = v
			}
			if v, ok := otelcInfo["dsn"]; ok {
				otelcConn.DSN = v
			}
			if v, ok := otelcInfo["dbName"]; ok {
				otelcConn.DbName = v
			}
		}()
	}
	return
}

// ---- Conn operations ----

func (conn *Conn) PingContext(ctx context.Context) (otelcErr error) {
	var otelcSpan trace.Span
	if conn != nil {
		otelcSpan = otelcDBStart(ctx, "ping", "ping", conn.Endpoint, conn.DriverName, conn.DSN, conn.DbName)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}

func (conn *Conn) PrepareContext(ctx context.Context, query string) (otelcStmt *Stmt, otelcErr error) {
	if conn != nil {
		otelcInfo := map[string]string{
			"endpoint": conn.Endpoint,
			"sql":      query,
			"driver":   conn.DriverName,
			"dsn":      conn.DSN,
			"dbName":   conn.DbName,
		}
		defer func() {
			if otelcStmt == nil {
				return
			}
			otelcStmt.Data = map[string]string{
				"endpoint": otelcInfo["endpoint"],
				"sql":      otelcInfo["sql"],
				"driver":   otelcInfo["driver"],
				"dbName":   otelcInfo["dbName"],
			}
			otelcStmt.DSN = otelcInfo["dsn"]
		}()
	}
	return
}

func (conn *Conn) ExecContext(ctx context.Context, query string, args ...interface{}) (otelcResult Result, otelcErr error) {
	var otelcSpan trace.Span
	if conn != nil {
		otelcSpan = otelcDBStart(ctx, "exec", query, conn.Endpoint, conn.DriverName, conn.DSN, conn.DbName, args...)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}

func (conn *Conn) QueryContext(ctx context.Context, query string, args ...interface{}) (otelcRows *Rows, otelcErr error) {
	var otelcSpan trace.Span
	if conn != nil {
		otelcSpan = otelcDBStart(ctx, "query", query, conn.Endpoint, conn.DriverName, conn.DSN, conn.DbName, args...)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}

func (conn *Conn) BeginTx(ctx context.Context, opts *TxOptions) (otelcTx *Tx, otelcErr error) {
	var otelcSpan trace.Span
	if conn != nil {
		otelcSpan = otelcDBStart(ctx, "start", "START TRANSACTION", conn.Endpoint, conn.DriverName, conn.DSN, conn.DbName)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}

// ---- Tx operations ----

func (tx *Tx) PrepareContext(ctx context.Context, query string) (otelcStmt *Stmt, otelcErr error) {
	if tx != nil {
		otelcInfo := map[string]string{
			"endpoint": tx.Endpoint,
			"sql":      query,
			"driver":   tx.DriverName,
			"dsn":      tx.DSN,
			"dbName":   tx.DbName,
		}
		defer func() {
			if otelcStmt == nil {
				return
			}
			otelcStmt.Data = map[string]string{
				"endpoint": otelcInfo["endpoint"],
				"sql":      otelcInfo["sql"],
				"driver":   otelcInfo["driver"],
				"dbName":   otelcInfo["dbName"],
			}
			otelcStmt.DSN = otelcInfo["dsn"]
		}()
	}
	return
}

func (tx *Tx) StmtContext(ctx context.Context, stmt *Stmt) (otelcStmt *Stmt) {
	otelcInfo := map[string]string(nil)
	if stmt != nil && stmt.Data != nil {
		otelcInfo = map[string]string{
			"endpoint": stmt.Data["endpoint"],
			"driver":   stmt.Data["driver"],
			"dsn":      stmt.DSN,
			"sql":      stmt.Data["sql"],
			"dbName":   stmt.Data["dbName"],
		}
	}
	defer func() {
		if otelcStmt == nil {
			return
		}
		otelcStmt.Data = map[string]string{}
		if otelcInfo == nil {
			return
		}
		if v, ok := otelcInfo["endpoint"]; ok {
			otelcStmt.Data["endpoint"] = v
		}
		if v, ok := otelcInfo["driver"]; ok {
			otelcStmt.Data["driver"] = v
		}
		if v, ok := otelcInfo["dsn"]; ok {
			otelcStmt.Data["dsn"] = v
		}
		if v, ok := otelcInfo["dbName"]; ok {
			otelcStmt.Data["dbName"] = v
		}
	}()
	return
}

func (tx *Tx) ExecContext(ctx context.Context, query string, args ...interface{}) (otelcResult Result, otelcErr error) {
	var otelcSpan trace.Span
	if tx != nil {
		otelcSpan = otelcDBStart(ctx, "exec", query, tx.Endpoint, tx.DriverName, tx.DSN, tx.DbName, args...)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}

func (tx *Tx) QueryContext(ctx context.Context, query string, args ...interface{}) (otelcRows *Rows, otelcErr error) {
	var otelcSpan trace.Span
	if tx != nil {
		otelcSpan = otelcDBStart(ctx, "query", query, tx.Endpoint, tx.DriverName, tx.DSN, tx.DbName, args...)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}

func (tx *Tx) Commit() (otelcErr error) {
	var otelcSpan trace.Span
	if tx != nil {
		otelcSpan = otelcDBStart(context.Background(), "commit", "COMMIT", tx.Endpoint, tx.DriverName, tx.DSN, tx.DbName)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}

func (tx *Tx) Rollback() (otelcErr error) {
	var otelcSpan trace.Span
	if tx != nil {
		otelcSpan = otelcDBStart(context.Background(), "rollback", "ROLLBACK", tx.Endpoint, tx.DriverName, tx.DSN, tx.DbName)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}

// ---- Stmt operations ----

func (s *Stmt) ExecContext(ctx context.Context, args ...interface{}) (otelcResult Result, otelcErr error) {
	var otelcSpan trace.Span
	if s != nil {
		otelcQuery, otelcEndpoint, otelcDriver, otelcDSN, otelcDBName := "", "", "", "", ""
		if s.Data != nil {
			otelcQuery, otelcEndpoint, otelcDriver, otelcDSN, otelcDBName = s.Data["sql"], s.Data["endpoint"], s.Data["driver"], s.DSN, s.Data["dbName"]
		}
		otelcSpan = otelcDBStart(ctx, "exec", otelcQuery, otelcEndpoint, otelcDriver, otelcDSN, otelcDBName, args...)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}

func (s *Stmt) QueryContext(ctx context.Context, args ...interface{}) (otelcRows *Rows, otelcErr error) {
	var otelcSpan trace.Span
	if s != nil {
		otelcQuery, otelcEndpoint, otelcDriver, otelcDSN, otelcDBName := "", "", "", "", ""
		if s.Data != nil {
			otelcQuery, otelcEndpoint, otelcDriver, otelcDSN, otelcDBName = s.Data["sql"], s.Data["endpoint"], s.Data["driver"], s.DSN, s.Data["dbName"]
		}
		otelcSpan = otelcDBStart(ctx, "query", otelcQuery, otelcEndpoint, otelcDriver, otelcDSN, otelcDBName, args...)
	}
	if otelcSpan != nil {
		defer func() { otelcDBEnd(otelcSpan, otelcErr) }()
	}
	return
}
