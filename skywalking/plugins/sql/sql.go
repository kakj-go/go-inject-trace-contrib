//go:build goinject

//inject:database/sql
package sql

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

// Open propagates the peer/component information produced by the driver
// package (mysql ParseDSN / pgx stdlib OpenConnector) through the runtime
// context and attaches it to the returned *DB, mirroring the official
// sql/entry InstanceInterceptor.
func Open(driverName, dataSourceName string) (db *DB, err error) {
	tracing.SetRuntimeContextValue(tracing.SQLNeedInfoRuntimeContextKey, true)
	defer func() {
		tracing.SetRuntimeContextValue(tracing.SQLNeedInfoRuntimeContextKey, nil)
		swInfo, swOK := tracing.GetRuntimeContextValue(tracing.SQLInfoRuntimeContextKey).(swInstanceInfo)
		tracing.SetRuntimeContextValue(tracing.SQLInfoRuntimeContextKey, nil)
		if !swOK || swInfo == nil {
			return
		}
		// adding peer address into db
		if db != nil {
			db.SwSkywalkingData = swInfo
		}
	}()
	return
}

type DB struct {
	//inject:add
	SwSkywalkingData interface{}
}

type Stmt struct {
	//inject:add
	SwSkywalkingData interface{}
}

type Tx struct {
	//inject:add
	SwSkywalkingData interface{}
}

type Conn struct {
	//inject:add
	SwSkywalkingData interface{}
}

// (getter/setter methods removed: //inject:add does not support methods;
// use direct SwSkywalkingData field access instead)

// DB operations. A failed span creation (empty peer, for instance) never
// short-circuits the database call: the official agent logs the before-invoke
// error and still runs the original function without a span in the context.

func (db *DB) PingContext(ctx context.Context) (err error) {
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateExitSpan(swInstanceInfoOf(db), "Ping"); swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

func (db *DB) PrepareContext(ctx context.Context, query string) (stmt *Stmt, err error) {
	swInfo := swInstanceInfoOf(db)
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateLocalSpan(swInfo, "Prepare",
		tracing.WithTag(tracing.TagDBStatement, query)); swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
		// propagate the instance info
		if stmt != nil {
			stmt.SwSkywalkingData = swInfo
		}
	}()
	return
}

func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (result Result, err error) {
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateExitSpan(swInstanceInfoOf(db), "Exec",
		tracing.WithTag(tracing.TagDBStatement, query)); swErr == nil {
		swSpan = s
	}
	if swSpan != nil && swSQLCollectParameter && len(args) > 0 {
		swSpan.Tag(tracing.TagDBSqlParameters, swArgsToString(args))
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (rows *Rows, err error) {
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateExitSpan(swInstanceInfoOf(db), "Query",
		tracing.WithTag(tracing.TagDBStatement, query)); swErr == nil {
		swSpan = s
	}
	if swSpan != nil && swSQLCollectParameter && len(args) > 0 {
		swSpan.Tag(tracing.TagDBSqlParameters, swArgsToString(args))
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

func (db *DB) BeginTx(ctx context.Context, opts *TxOptions) (tx *Tx, err error) {
	swInfo := swInstanceInfoOf(db)
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateLocalSpan(swInfo, "BeginTx"); swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
		// propagate the instance info
		if tx != nil {
			tx.SwSkywalkingData = swInfo
		}
	}()
	return
}

func (db *DB) Conn(ctx context.Context) (conn *Conn, err error) {
	swInfo := swInstanceInfoOf(db)
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateLocalSpan(swInfo, "Conn"); swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
		// propagate the instance info
		if conn != nil {
			conn.SwSkywalkingData = swInfo
		}
	}()
	return
}

// Conn operations

func (c *Conn) PingContext(ctx context.Context) (err error) {
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateExitSpan(swInstanceInfoOf(c), "Conn/Ping"); swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

func (c *Conn) PrepareContext(ctx context.Context, query string) (stmt *Stmt, err error) {
	swInfo := swInstanceInfoOf(c)
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateLocalSpan(swInfo, "Conn/Prepare",
		tracing.WithTag(tracing.TagDBStatement, query)); swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
		// propagate the instance info
		if stmt != nil {
			stmt.SwSkywalkingData = swInfo
		}
	}()
	return
}

func (c *Conn) ExecContext(ctx context.Context, query string, args ...any) (result Result, err error) {
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateExitSpan(swInstanceInfoOf(c), "Conn/Exec",
		tracing.WithTag(tracing.TagDBStatement, query)); swErr == nil {
		swSpan = s
	}
	if swSpan != nil && swSQLCollectParameter && len(args) > 0 {
		swSpan.Tag(tracing.TagDBSqlParameters, swArgsToString(args))
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

func (c *Conn) QueryContext(ctx context.Context, query string, args ...any) (rows *Rows, err error) {
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateExitSpan(swInstanceInfoOf(c), "Conn/Query",
		tracing.WithTag(tracing.TagDBStatement, query)); swErr == nil {
		swSpan = s
	}
	if swSpan != nil && swSQLCollectParameter && len(args) > 0 {
		swSpan.Tag(tracing.TagDBSqlParameters, swArgsToString(args))
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

func (c *Conn) Raw(f func(driverConn any) error) (err error) {
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateExitSpan(swInstanceInfoOf(c), "Conn/Raw"); swErr == nil {
		swSpan = s
	}
	_ = swSpan
	// The official plugin's after-invoke reads results[1] on this single-result
	// method and panics; the agent recovers the panic, so the span is created
	// but never error-tagged nor ended. Replicate that net behavior: the span
	// stays open, exactly like the official agent leaves it.
	return
}

func (c *Conn) BeginTx(ctx context.Context, opts *TxOptions) (tx *Tx, err error) {
	swInfo := swInstanceInfoOf(c)
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateLocalSpan(swInfo, "Conn/BeginTx"); swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
		// propagate the instance info
		if tx != nil {
			tx.SwSkywalkingData = swInfo
		}
	}()
	return
}

// Tx operations

func (tx *Tx) Commit() (err error) {
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateExitSpan(swInstanceInfoOf(tx), "Tx/Commit"); swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

func (tx *Tx) Rollback() (err error) {
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateExitSpan(swInstanceInfoOf(tx), "Tx/Rollback"); swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

func (tx *Tx) PrepareContext(ctx context.Context, query string) (stmt *Stmt, err error) {
	swInfo := swInstanceInfoOf(tx)
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateLocalSpan(swInfo, "Tx/Prepare",
		tracing.WithTag(tracing.TagDBStatement, query)); swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
		// propagate the instance info
		if stmt != nil {
			stmt.SwSkywalkingData = swInfo
		}
	}()
	return
}

func (tx *Tx) StmtContext(ctx context.Context, stmt *Stmt) (txStmt *Stmt) {
	swInfo := swInstanceInfoOf(tx)
	defer func() {
		// propagate the instance info, overwriting with nil when the Tx has none,
		// exactly like the official TxStmtInterceptor
		if txStmt != nil {
			txStmt.SwSkywalkingData = swInfo
		}
	}()
	return
}

func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) (result Result, err error) {
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateExitSpan(swInstanceInfoOf(tx), "Tx/Exec",
		tracing.WithTag(tracing.TagDBStatement, query)); swErr == nil {
		swSpan = s
	}
	if swSpan != nil && swSQLCollectParameter && len(args) > 0 {
		swSpan.Tag(tracing.TagDBSqlParameters, swArgsToString(args))
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

func (tx *Tx) QueryContext(ctx context.Context, query string, args ...any) (rows *Rows, err error) {
	swSpan := tracing.Span(nil)
	if s, swErr := swCreateExitSpan(swInstanceInfoOf(tx), "Tx/Query",
		tracing.WithTag(tracing.TagDBStatement, query)); swErr == nil {
		swSpan = s
	}
	if swSpan != nil && swSQLCollectParameter && len(args) > 0 {
		swSpan.Tag(tracing.TagDBSqlParameters, swArgsToString(args))
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

// Stmt operations

func (s *Stmt) ExecContext(ctx context.Context, args ...any) (result Result, err error) {
	swSpan := tracing.Span(nil)
	if s2, swErr := swCreateExitSpan(swInstanceInfoOf(s), "Stmt/Exec"); swErr == nil {
		swSpan = s2
	}
	if swSpan != nil && swSQLCollectParameter && len(args) > 0 {
		swSpan.Tag(tracing.TagDBSqlParameters, swArgsToString(args))
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

func (s *Stmt) QueryContext(ctx context.Context, args ...any) (rows *Rows, err error) {
	swSpan := tracing.Span(nil)
	if s2, swErr := swCreateExitSpan(swInstanceInfoOf(s), "Stmt/Query"); swErr == nil {
		swSpan = s2
	}
	if swSpan != nil && swSQLCollectParameter && len(args) > 0 {
		swSpan.Tag(tracing.TagDBSqlParameters, swArgsToString(args))
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

//inject:add
type swInstanceInfo interface {
	Peer() string
	ComponentID() int32
	DBType() string
}

//inject:add
func swInstanceInfoOf(caller interface{}) swInstanceInfo {
	switch instance := caller.(type) {
	case *DB:
		if instance == nil {
			return nil
		}
		info, _ := instance.SwSkywalkingData.(swInstanceInfo)
		return info
	case *Stmt:
		if instance == nil {
			return nil
		}
		info, _ := instance.SwSkywalkingData.(swInstanceInfo)
		return info
	case *Tx:
		if instance == nil {
			return nil
		}
		info, _ := instance.SwSkywalkingData.(swInstanceInfo)
		return info
	case *Conn:
		if instance == nil {
			return nil
		}
		info, _ := instance.SwSkywalkingData.(swInstanceInfo)
		return info
	default:
		return nil
	}
}

//inject:add
func swCreateExitSpan(info swInstanceInfo, method string, opts ...tracing.SpanOption) (tracing.Span, error) {
	if info == nil {
		return nil, nil
	}
	return tracing.CreateExitSpan(info.DBType()+"/"+method, info.Peer(), func(headerKey, headerValue string) error {
		return nil
	}, append(opts, tracing.WithComponent(info.ComponentID()),
		tracing.WithLayer(tracing.SpanLayerDatabase),
		tracing.WithTag(tracing.TagDBType, info.DBType()))...)
}

//inject:add
func swCreateLocalSpan(info swInstanceInfo, method string, opts ...tracing.SpanOption) (tracing.Span, error) {
	if info == nil {
		return nil, nil
	}
	return tracing.CreateLocalSpan(info.DBType()+"/"+method,
		append(opts, tracing.WithComponent(info.ComponentID()),
			tracing.WithLayer(tracing.SpanLayerDatabase),
			tracing.WithTag(tracing.TagDBType, info.DBType()))...)
}

//inject:add
func swArgsToString(args []any) string {
	switch len(args) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("%v", args[0])
	}

	res := fmt.Sprintf("%v", args[0])
	for _, arg := range args[1:] {
		res += fmt.Sprintf(", %v", arg)
	}
	return res
}

//inject:add
var swSQLCollectParameter = func() bool {
	if v := os.Getenv("SW_AGENT_PLUGIN_CONFIG_SQL_COLLECT_PARAMETER"); v != "" {
		if b, e := strconv.ParseBool(v); e == nil {
			return b
		}
	}
	return false
}()
