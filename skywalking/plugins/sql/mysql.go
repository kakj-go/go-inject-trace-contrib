//go:build goinject

//inject:github.com/go-sql-driver/mysql
package sql

import (
	"github.com/apache/skywalking-go/plugins/core/tracing"
)

// ParseDSN publishes the DSN peer address through the runtime context while
// the intercepted database/sql.Open is waiting for it, mirroring the official
// sql/mysql ParseInterceptor.
func ParseDSN(dsn string) (cfg *Config, err error) {
	defer func() {
		if cfg != nil &&
			tracing.GetRuntimeContextValue(tracing.SQLNeedInfoRuntimeContextKey) == true {
			tracing.SetRuntimeContextValue(tracing.SQLInfoRuntimeContextKey, &swDBInfo{Addr: cfg.Addr})
		}
	}()
	return
}

//inject:add
type swDBInfo struct {
	Addr string
}

//inject:add
func (i *swDBInfo) Peer() string {
	return i.Addr
}

//inject:add
func (i *swDBInfo) ComponentID() int32 {
	return 5012
}

//inject:add
func (i *swDBInfo) DBType() string {
	return "Mysql"
}
