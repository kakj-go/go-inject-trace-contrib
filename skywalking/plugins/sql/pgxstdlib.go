//go:build goinject

//inject:github.com/jackc/pgx/v5/stdlib
package sql

import (
	"database/sql/driver"

	"github.com/apache/skywalking-go/plugins/core/tracing"

	"github.com/jackc/pgx/v5"
)

// OpenConnector publishes the DSN peer address through the runtime context
// while the intercepted database/sql.Open is waiting for it, mirroring the
// official sql/pgxstdlib OpenConnectorInterceptor.
func (d *Driver) OpenConnector(name string) (connector driver.Connector, err error) {
	defer func() {
		if tracing.GetRuntimeContextValue(tracing.SQLNeedInfoRuntimeContextKey) != true {
			return
		}
		if swInfo := swBuildDBInfo(name); swInfo != nil {
			tracing.SetRuntimeContextValue(tracing.SQLInfoRuntimeContextKey, swInfo)
		}
	}()
	return
}

type Driver struct {
}

//inject:add
const swPostgreSQLComponentID int32 = 22

//inject:add
func swBuildDBInfo(name string) *swDBInfo {
	if name == "" {
		return nil
	}
	cfg, err := pgx.ParseConfig(name)
	if err == nil {
		return swBuildDBInfoFromConnConfig(cfg)
	}
	return nil
}

//inject:add
type swDBInfo struct {
	PeerAddress string
}

//inject:add
func swBuildDBInfoFromConnConfig(cfg *pgx.ConnConfig) *swDBInfo {
	if cfg == nil {
		return nil
	}
	peer := swBuildPeerAddress(cfg)
	if peer == "" {
		return nil
	}
	return &swDBInfo{PeerAddress: peer}
}

//inject:add
func swBuildPeerAddress(cfg *pgx.ConnConfig) string {
	if cfg == nil {
		return ""
	}
	fallbacks := make([]tracing.PostgreSQLAddress, 0, len(cfg.Fallbacks))
	for _, fallback := range cfg.Fallbacks {
		if fallback == nil {
			continue
		}
		fallbacks = append(fallbacks, tracing.PostgreSQLAddress{Host: fallback.Host, Port: fallback.Port})
	}
	return tracing.BuildPostgreSQLPeer(
		tracing.PostgreSQLAddress{Host: cfg.Host, Port: cfg.Port},
		fallbacks,
	)
}

//inject:add
func (i *swDBInfo) Peer() string {
	return i.PeerAddress
}

//inject:add
func (i *swDBInfo) ComponentID() int32 {
	return swPostgreSQLComponentID
}

//inject:add
func (i *swDBInfo) DBType() string {
	return "PostgreSQL"
}
