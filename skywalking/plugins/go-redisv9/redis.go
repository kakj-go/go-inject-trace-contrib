//go:build goinject

//inject:github.com/redis/go-redis/v9
package goredisv9

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

// The five client constructors mirror the official go-redisv9
// GoRedisInterceptor: after the client is built, the tracing hook is added to
// it (and to every node a cluster/ring client discovers later).

func NewClient(opt *Options) (rdb *Client) {
	defer func() {
		swInstallHook(rdb)
	}()
	return
}

func NewFailoverClient(failoverOpt *FailoverOptions) (rdb *Client) {
	defer func() {
		swInstallHook(rdb)
	}()
	return
}

func NewClusterClient(opt *ClusterOptions) (rdb *ClusterClient) {
	defer func() {
		swInstallHook(rdb)
	}()
	return
}

func NewRing(opt *RingOptions) (rdb *Ring) {
	defer func() {
		swInstallHook(rdb)
	}()
	return
}

func NewUniversalClient(opts *UniversalOptions) (rdb UniversalClient) {
	defer func() {
		swInstallHook(rdb)
	}()
	return
}

//inject:add
func swInstallHook(rdb interface{}) {
	c, ok := rdb.(UniversalClient)
	if !ok {
		// The official interceptor reports an agent-log error here; that log is
		// the only effect, and none of the five intercepted constructors can
		// produce a non-UniversalClient result.
		return
	}

	switch c := c.(type) {
	case *Client:
		c.AddHook(swNewRedisHook(c.Options().Addr))
	case *ClusterClient:
		c.AddHook(swNewRedisHook(""))

		c.OnNewNode(func(rdb *Client) {
			rdb.AddHook(swNewRedisHook(rdb.Options().Addr))
		})
	case *Ring:
		c.AddHook(swNewRedisHook(""))

		c.OnNewNode(func(rdb *Client) {
			rdb.AddHook(swNewRedisHook(rdb.Options().Addr))
		})
	default:
		// unsupported client type: the official interceptor only logs an error
	}
}

//inject:add
const (
	swGoRedisComponentID = 5014
	swGoRedisCacheType   = "redis"
)

//inject:add
type swRedisHook struct {
	Addr string
}

//inject:add
func swNewRedisHook(addr string) *swRedisHook {
	return &swRedisHook{
		Addr: addr,
	}
}

//inject:add
func (r *swRedisHook) DialHook(next DialHook) DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		s, err := tracing.CreateExitSpan(
			// operationName
			swGoRedisCacheType+"/"+"dial",

			// peer
			r.Addr,

			// injector
			func(k, v string) error {
				return nil
			},

			// opts
			tracing.WithComponent(swGoRedisComponentID),
			tracing.WithLayer(tracing.SpanLayerCache),
			tracing.WithTag(tracing.TagCacheType, swGoRedisCacheType),
		)

		if err != nil {
			err = fmt.Errorf("go-redis :skyWalking failed to create exit span, got error: %v", err)
			return nil, err
		}

		defer s.End()

		conn, err := next(ctx, network, addr)
		if err != nil {
			swRecordError(s, err)
		}
		return conn, err
	}
}

//inject:add
func (r *swRedisHook) ProcessHook(next ProcessHook) ProcessHook {
	return func(ctx context.Context, cmd Cmder) error {
		var s tracing.Span
		var err error
		if r.Addr != "" {
			s, err = tracing.CreateExitSpan(
				// operationName
				swGoRedisCacheType+"/"+cmd.FullName(),

				// peer
				r.Addr,

				// injector
				func(k, v string) error {
					return nil
				},

				// opts
				tracing.WithComponent(swGoRedisComponentID),
				tracing.WithLayer(tracing.SpanLayerCache),
				tracing.WithTag(tracing.TagCacheType, swGoRedisCacheType),
				tracing.WithTag(tracing.TagCacheOp, swGetCacheOp(cmd.FullName())),
				tracing.WithTag(tracing.TagCacheCmd, cmd.FullName()),
				tracing.WithTag(tracing.TagCacheKey, swGetKey(cmd.Args())),
				tracing.WithTag(tracing.TagCacheArgs, swMaxString(cmd.String(), swRedisMaxArgsBytes)),
			)

			if err != nil {
				err = fmt.Errorf("go-redis :skyWalking failed to create exit span, got error: %v", err)
				return err
			}

			defer s.End()
		}

		if err = next(ctx, cmd); err != nil {
			swRecordError(s, err)
			return err
		}

		return nil
	}
}

//inject:add
func (r *swRedisHook) ProcessPipelineHook(next ProcessPipelineHook) ProcessPipelineHook {
	return func(ctx context.Context, cmds []Cmder) error {
		summary := ""
		summaryCmds := cmds
		if len(summaryCmds) > 10 {
			summaryCmds = summaryCmds[:10]
		}
		for i := range summaryCmds {
			summary += summaryCmds[i].FullName() + "/"
		}
		if len(cmds) > 10 {
			summary += "..."
		}

		var s tracing.Span
		var err error
		if r.Addr != "" {
			s, err = tracing.CreateExitSpan(
				// operationName
				"redis/pipeline",

				// peer
				r.Addr,

				// injector
				func(k, v string) error {
					return nil
				},

				// opts
				tracing.WithComponent(swGoRedisComponentID),
				tracing.WithLayer(tracing.SpanLayerCache),
				tracing.WithTag(tracing.TagCacheType, swGoRedisCacheType),
				tracing.WithTag(tracing.TagCacheCmd, "pipeline:"+strings.TrimRight(summary, "/")),
			)
			if err != nil {
				err = fmt.Errorf("go-redis :skyWalking failed to create exit span, got error: %v", err)
				return err
			}
			defer s.End()
		}

		if err = next(ctx, cmds); err != nil {
			swRecordError(s, err)
			return err
		}

		return nil
	}
}

//inject:add
func swRecordError(span tracing.Span, err error) {
	if err != Nil && span != nil {
		span.Error(err.Error())
	}
}

// swGetKey Try to transform the second argument into string
// e.g. "GET my_key" -> "my_key"
//
//inject:add
func swGetKey(args []interface{}) string {
	key := ""
	if len(args) >= 2 {
		k := args[1]
		switch v := k.(type) {
		case string:
			key = v
		default:
			break
		}
	}
	return key
}

// swMaxString limit the bytes length of the redis args.
//
//inject:add
func swMaxString(s string, length int) string {
	if length <= 0 { // no define or no limit
		return s
	}

	if len(s) > length {
		return s[:length]
	}
	return s
}

//inject:add
var swRedisMaxArgsBytes = func() int {
	if v := os.Getenv("SW_AGENT_PLUGIN_CONFIG_REDIS_MAX_ARGS_BYTES"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			return n
		}
	}
	return 1024
}()
