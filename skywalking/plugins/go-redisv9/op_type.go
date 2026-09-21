//go:build goinject

//inject:github.com/redis/go-redis/v9
package goredisv9

//inject:add
const (
	swOpTypeWrite   = "write"
	swOpTypeRead    = "read"
	swOpTypeUnknown = ""
)

// Commands are divided into different type.
// Ref to JedisPluginConfig.java under skywalking-java repo
//
//inject:add
var swWriteOperation = map[string]bool{
	"getset":           true,
	"set":              true,
	"setbit":           true,
	"setex":            true,
	"setnx":            true,
	"setrange":         true,
	"strlen":           true,
	"mset":             true,
	"msetnx":           true,
	"psetex":           true,
	"incr":             true,
	"incrby":           true,
	"incrbyfloat":      true,
	"decr":             true,
	"decrby":           true,
	"append":           true,
	"hmset":            true,
	"hset":             true,
	"hsetnx":           true,
	"hincrby":          true,
	"hincrbyfloat":     true,
	"hdel":             true,
	"rpoplpush":        true,
	"rpush":            true,
	"rpushx":           true,
	"lpush":            true,
	"lpushx":           true,
	"lrem":             true,
	"ltrim":            true,
	"lset":             true,
	"brpoplpush":       true,
	"linsert":          true,
	"sadd":             true,
	"sdiff":            true,
	"sdiffstore":       true,
	"sinterstore":      true,
	"sismember":        true,
	"srem":             true,
	"sunion":           true,
	"sunionstore":      true,
	"sinter":           true,
	"zadd":             true,
	"zincrby":          true,
	"zinterstore":      true,
	"zrange":           true,
	"zrangebylex":      true,
	"zrangebyscore":    true,
	"zrank":            true,
	"zrem":             true,
	"zremrangebylex":   true,
	"zremrangebyrank":  true,
	"zremrangebyscore": true,
	"zrevrange":        true,
	"zrevrangebyscore": true,
	"zrevrank":         true,
	"zunionstore":      true,
	"xadd":             true,
	"xdel":             true,
	"del":              true,
	"xtrim":            true,
}

//inject:add
var swReadOperation = map[string]bool{
	"keys":        true,
	"scan":        true,
	"getrange":    true,
	"getbit":      true,
	"mget":        true,
	"hvals":       true,
	"hkeys":       true,
	"hlen":        true,
	"hscan":       true,
	"hexists":     true,
	"hget":        true,
	"hgetall":     true,
	"hmget":       true,
	"blpop":       true,
	"brpop":       true,
	"lindex":      true,
	"llen":        true,
	"lpop":        true,
	"lrange":      true,
	"rpop":        true,
	"scard":       true,
	"srandmember": true,
	"spop":        true,
	"sscan":       true,
	"smove":       true,
	"zlexcount":   true,
	"zscore":      true,
	"zscan":       true,
	"zcard":       true,
	"zcount":      true,
	"xget":        true,
	"get":         true,
	"xread":       true,
	"xlen":        true,
	"xrange":      true,
	"xrevrange":   true,
}

// swGetCacheOp return "read" or "write" or "" based on the cmd.
//
//inject:add
func swGetCacheOp(cmd string) string {
	if swReadOperation[cmd] {
		return swOpTypeRead
	} else if swWriteOperation[cmd] {
		return swOpTypeWrite
	}
	return swOpTypeUnknown
}
