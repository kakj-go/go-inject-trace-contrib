//go:build goinject

//inject:gorm.io/driver/postgres
package gorm

import (
	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

// Open and New mirror the official gorm/postgres InstanceInterceptor: after
// the dialector is built, the peer address is derived from the DSN (or from a
// pre-connected pool) and stored on the embedded config for the gorm.Open
// interception to pick up.
func Open(dsn string) (dialector gorm.Dialector) {
	defer func() {
		if res, ok := dialector.(*Dialector); ok && res != nil {
			if dbInfo := swBuildDBInfoFromDialector(res); dbInfo != nil {
				res.SwSkywalkingData = dbInfo
			}
		}
	}()
	return
}

func New(config Config) (dialector gorm.Dialector) {
	defer func() {
		if res, ok := dialector.(*Dialector); ok && res != nil {
			if dbInfo := swBuildDBInfoFromDialector(res); dbInfo != nil {
				res.SwSkywalkingData = dbInfo
			}
		}
	}()
	return
}

type Dialector struct {
	*Config
}

type Config struct {
	Conn gorm.ConnPool

	//inject:add
	SwSkywalkingData interface{}
}

// Initialize mirrors the official gorm/postgres InitializeInterceptor: when
// the dialector set up a database/sql pool without any peer information (a
// user-supplied Conn, for instance), propagate this dialector's info onto the
// pool so the sql interception can build spans from it.
func (dialector Dialector) Initialize(db *gorm.DB) (err error) {
	defer func() {
		if err != nil {
			return
		}
		if db == nil || db.ConnPool == nil {
			return
		}
		connPool, ok := db.ConnPool.(swEnhancedInstance)
		if !ok || connPool == nil {
			return
		}
		if connPool.swGetSkywalkingData() != nil {
			return
		}
		dbInfo := swBuildDBInfoFromInvocation(dialector)
		if dbInfo == nil {
			return
		}
		connPool.swSetSkywalkingData(dbInfo)
	}()
	return
}

//inject:add
func (c *Config) swGetSkywalkingData() interface{} {
	return c.SwSkywalkingData
}

//inject:add
type swEnhancedInstance interface {
	swGetSkywalkingData() interface{}
	swSetSkywalkingData(interface{})
}

//inject:add
const swPostgreSQLComponentID int32 = 22

//inject:add
const swPostgreSQLDBType = "PostgreSQL"

//inject:add
type swSQLDatabaseInfo interface {
	DBType() string
	ComponentID() int32
	Peer() string
}

//inject:add
type swDatabaseInfo struct {
	PeerAddress string
}

//inject:add
func swBuildDBInfoFromDialector(dial *Dialector) *swDatabaseInfo {
	if dial == nil {
		return nil
	}
	return swBuildDBInfoFromConfig(dial.Config)
}

//inject:add
func swBuildDBInfoFromConfig(config *Config) *swDatabaseInfo {
	if config == nil {
		return nil
	}
	if config.DSN != "" {
		cfg, err := pgx.ParseConfig(config.DSN)
		if err == nil {
			peer := swBuildPeerAddress(cfg)
			if peer != "" {
				return &swDatabaseInfo{PeerAddress: peer}
			}
		}
	}
	return swBuildDBInfoFromConn(config.Conn)
}

//inject:add
func (d *swDatabaseInfo) Type() string {
	return swPostgreSQLDBType
}

//inject:add
func (d *swDatabaseInfo) DBType() string {
	return swPostgreSQLDBType
}

//inject:add
func (d *swDatabaseInfo) ComponentID() int32 {
	return swPostgreSQLComponentID
}

//inject:add
func (d *swDatabaseInfo) Peer() string {
	return d.PeerAddress
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
func swBuildDBInfoFromConn(conn interface{}) *swDatabaseInfo {
	ins, ok := conn.(swEnhancedInstance)
	if !ok || ins == nil {
		return nil
	}
	return swAdaptSQLDatabaseInfo(ins.swGetSkywalkingData())
}

//inject:add
func swAdaptSQLDatabaseInfo(v interface{}) *swDatabaseInfo {
	switch info := v.(type) {
	case nil:
		return nil
	case *swDatabaseInfo:
		return info
	case swSQLDatabaseInfo:
		if info.DBType() != swPostgreSQLDBType || info.Peer() == "" {
			return nil
		}
		return &swDatabaseInfo{PeerAddress: info.Peer()}
	default:
		return nil
	}
}

//inject:add
func swBuildDBInfoFromInvocation(caller interface{}) *swDatabaseInfo {
	if caller == nil {
		return nil
	}
	// The official plugin reads the dynamic field through methods promoted
	// from the embedded *Config; when the embedded config is nil that read
	// panics inside the agent and is recovered, effectively skipping the
	// field lookup, so guard it here.
	switch c := caller.(type) {
	case *Dialector:
		if c == nil {
			return nil
		}
		if c.Config != nil {
			if dbInfo := swAdaptSQLDatabaseInfo(c.SwSkywalkingData); dbInfo != nil {
				return dbInfo
			}
		}
		return swBuildDBInfoFromConfig(c.Config)
	case Dialector:
		if c.Config != nil {
			if dbInfo := swAdaptSQLDatabaseInfo(c.SwSkywalkingData); dbInfo != nil {
				return dbInfo
			}
		}
		return swBuildDBInfoFromConfig(c.Config)
	default:
		return nil
	}
}
