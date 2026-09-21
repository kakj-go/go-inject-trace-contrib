//go:build goinject

//inject:gorm.io/gorm
package gorm

import (
	"fmt"
	"os"
	"reflect"
	"strconv"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

// Open mirrors the official gorm/entry OpenInterceptor: it reads the database
// information the driver dialector carries and registers the six
// before/after callback pairs that create and finish the spans.
func Open(dialector Dialector, opts ...Option) (db *DB, err error) {
	defer func() {
		if err != nil {
			return
		}
		if db == nil {
			return
		}
		// setup database info
		info := swSetupDatabaseInfo(db)
		if info == nil {
			return
		}
		// add the callback
		_ = db.Callback().Create().Before("gorm:create").Register("sky_create_create_span", swBeforeCallback(info, "create"))
		_ = db.Callback().Query().Before("gorm:query").Register("sky_create_query_span", swBeforeCallback(info, "query"))
		_ = db.Callback().Update().Before("gorm:update").Register("sky_create_update_span", swBeforeCallback(info, "update"))
		_ = db.Callback().Delete().Before("gorm:delete").Register("sky_create_delete_span", swBeforeCallback(info, "delete"))
		_ = db.Callback().Row().Before("gorm:row").Register("sky_create_row_span", swBeforeCallback(info, "row"))
		_ = db.Callback().Raw().Before("gorm:raw").Register("sky_create_raw_span", swBeforeCallback(info, "raw"))

		// after database operation
		_ = db.Callback().Create().After("gorm:create").Register("sky_end_create_span", swAfterCallback(info))
		_ = db.Callback().Query().After("gorm:query").Register("sky_end_query_span", swAfterCallback(info))
		_ = db.Callback().Update().After("gorm:update").Register("sky_end_update_span", swAfterCallback(info))
		_ = db.Callback().Delete().After("gorm:delete").Register("sky_end_delete_span", swAfterCallback(info))
		_ = db.Callback().Row().After("gorm:row").Register("sky_end_row_span", swAfterCallback(info))
		_ = db.Callback().Raw().After("gorm:raw").Register("sky_end_raw_span", swAfterCallback(info))
	}()
	return
}

//inject:add
type swDatabaseInfo interface {
	Type() string
	ComponentID() int32
	Peer() string
}

//inject:add
type swEnhancedInstance interface {
	swGetSkywalkingData() interface{}
}

//inject:add
func swSetupDatabaseInfo(db *DB) swDatabaseInfo {
	if db == nil || db.Config == nil || db.Config.Dialector == nil {
		return nil
	}
	// use reflect to read the injected swSkywalkingData field directly,
	// avoiding the interface method that //inject:add may not apply to methods
	rv := reflect.ValueOf(db.Config.Dialector)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	f := rv.FieldByName("SwSkywalkingData")
	if !f.IsValid() || f.IsNil() {
		return nil
	}
	dbInfo, ok := f.Interface().(swDatabaseInfo)
	if !ok || dbInfo == nil {
		return nil
	}
	return dbInfo
}

//inject:add
var swSpanKey = "skywalking-span"

//inject:add
func swBeforeCallback(dbInfo swDatabaseInfo, op string) func(db *DB) {
	return func(db *DB) {
		tableName := db.Statement.Table
		operation := fmt.Sprintf("%s/%s", tableName, op)
		// a leftover span on this very Statement means a chained *gorm.DB is
		// shared across goroutines (unsupported by gorm): the previous span is
		// about to be overwritten and lost, so make the misuse visible
		if leftover, ok := db.InstanceGet(swSpanKey); ok {
			if _, isSpan := leftover.(tracing.Span); isSpan {
				db.Logger.Warn(db.Statement.Context,
					"gorm:skywalking found an unfinished span on the statement, "+
						"the *gorm.DB is probably shared across goroutines; its trace data will be lost")
			}
		}
		s, err := tracing.CreateExitSpan(operation, dbInfo.Peer(), func(k, v string) error {
			return nil
		}, tracing.WithComponent(dbInfo.ComponentID()),
			tracing.WithLayer(tracing.SpanLayerDatabase),
			tracing.WithTag(tracing.TagDBType, dbInfo.Type()))

		if err != nil {
			db.Logger.Error(db.Statement.Context, "gorm:skyWalking failed to create exit span, got error: %v", err)
			return
		}

		// InstanceSet keys by the Statement pointer: gorm's Statement.clone
		// copies plain db.Set Settings into every Session/Transaction clone,
		// which let a derived operation pick up - and end - the OUTER span
		db.InstanceSet(swSpanKey, s)
	}
}

//inject:add
func swAfterCallback(dbInfo swDatabaseInfo) func(db *DB) {
	return func(db *DB) {
		// get span from db instance's context
		spanInterface, _ := db.InstanceGet(swSpanKey)
		span, ok := spanInterface.(tracing.Span)
		if !ok {
			return
		}
		// the span is consumed: a later operation on the same statement must
		// not see it as a leftover
		db.InstanceSet(swSpanKey, nil)

		defer span.End()

		span.Tag(tracing.TagDBStatement, db.Statement.SQL.String())
		if swGormCollectParameter && len(db.Statement.Vars) > 0 {
			span.Tag(tracing.TagDBSqlParameters, swArgsToString(db.Statement.Vars))
		}
		if db.Statement.Error != nil {
			span.Error(db.Statement.Error.Error())
		}
	}
}

//inject:add
func swArgsToString(args []interface{}) string {
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
var swGormCollectParameter = func() bool {
	if v := os.Getenv("SW_AGENT_PLUGIN_CONFIG_GORM_COLLECT_PARAMETER"); v != "" {
		if b, e := strconv.ParseBool(v); e == nil {
			return b
		}
	}
	return false
}()
