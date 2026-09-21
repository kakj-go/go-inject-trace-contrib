//go:build goinject

//inject:main
package bootinit

import "github.com/kakj-go/go-inject-trace-contrib/skywalking/boot"

//inject:add
func init() { boot.Init() }
