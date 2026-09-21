//go:build goinject

//inject:gorm.io/driver/mysql
package gorm

import (
	driver "github.com/go-sql-driver/mysql"

	"gorm.io/gorm"
)

// Open and New mirror the official gorm/mysql InstanceInterceptor: after the
// dialector is built, its DSN is parsed and the peer address is stored on the
// dialector for the gorm.Open interception to pick up.
func Open(dsn string) (dialector gorm.Dialector) {
	defer func() {
		if res, ok := dialector.(*Dialector); ok && res != nil && res.Config != nil && res.Config.DSN != "" {
			if dbInfo := swBuildDBInfo(res); dbInfo != nil {
				res.SwSkywalkingData = dbInfo
			}
		}
	}()
	return
}

func New(config Config) (dialector gorm.Dialector) {
	defer func() {
		if res, ok := dialector.(*Dialector); ok && res != nil && res.Config != nil && res.Config.DSN != "" {
			if dbInfo := swBuildDBInfo(res); dbInfo != nil {
				res.SwSkywalkingData = dbInfo
			}
		}
	}()
	return
}

type Dialector struct {
	//inject:add
	SwSkywalkingData interface{}
}

//inject:add
func (d *Dialector) swGetSkywalkingData() interface{} {
	return d.SwSkywalkingData
}

//inject:add
type swDatabaseInfo struct {
	PeerAddress string
}

//inject:add
func swBuildDBInfo(dial *Dialector) *swDatabaseInfo {
	cfg, err := driver.ParseDSN(dial.Config.DSN)
	if err != nil {
		// ignore the db info if parse dsn failed
		return nil
	}
	return &swDatabaseInfo{PeerAddress: cfg.Addr}
}

//inject:add
func (d *swDatabaseInfo) Type() string {
	return "mysql"
}

//inject:add
func (d *swDatabaseInfo) ComponentID() int32 {
	return 5012
}

//inject:add
func (d *swDatabaseInfo) Peer() string {
	return d.PeerAddress
}
