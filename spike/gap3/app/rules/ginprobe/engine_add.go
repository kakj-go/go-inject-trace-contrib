//inject:github.com/gin-gonic/gin
package gin

import "sync/atomic"

type Engine struct {
	//inject:add
	goInjectProbe atomic.Uint64
}
