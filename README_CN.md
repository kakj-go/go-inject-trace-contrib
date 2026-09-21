# go-inject-trace-contrib

[English](README.md) | [中文](README_CN.md)

Go 服务的编译期插桩,以 [go-inject](https://github.com/kakj-go/go-inject)
规则模板的形式交付。一个 import 加一个构建开关——不需要框架专属的
middleware,不需要手写 span 代码。本仓库包含两套插桩后端:

- **[`otelc/`](otelc/)** — [OpenTelemetry](https://opentelemetry.io)
  trace + metric + log(移植自
  [opentelemetry-go-compile-instrumentation](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation))
- **[`skywalking/`](skywalking/)** — Apache SkyWalking 链路追踪(移植自
  [apache/skywalking-go](https://github.com/apache/skywalking-go))

两者产出的遥测数据与各自官方工具结构一致,并通过与官方工具的 A/B 场景
构建做过 1:1 验证(见下文"验证")。

## 用法

### OpenTelemetry(`otelc`)

```go
// main.go —— 普通 import(让 otel SDK 留在依赖图中):
import _ "github.com/kakj-go/go-inject-trace-contrib/otelc"
```

```go
// inject.go —— go-inject 规则注册文件:
//go:build goinject || generate

package main

import _ "github.com/kakj-go/go-inject-trace-contrib/otelc"
```

```bash
go mod tidy          # 加完 import 之后
go build -toolexec=go-inject .
```

运行时配置读取标准的 `OTEL_*` 环境变量。必配项(不配也有默认值,但只
适合本地冒烟测试):

| 变量 | 含义 | 不设置时的默认值 |
|---|---|---|
| `OTEL_SERVICE_NAME` | 服务名 | `unknown_service:<可执行文件名>` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector 基础 URL | `http://localhost:4318` |

其余变量——exporter、propagator、协议、header、超时、按库门控
(`OTEL_GO_ENABLED/DISABLED_INSTRUMENTATIONS`)——遵循
[OpenTelemetry 环境变量规范][otelnv] 与[上游 otelc][otelc]
的行为,与其兼容。一个已知缺口:`OTEL_TRACES_SAMPLER` 尚未接线
(恒为 `ParentBased(AlwaysSample)`)。

[otelnv]: https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/
[otelc]: https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation

### Apache SkyWalking(`skywalking`)

```go
// main 包内任意文件(普通 import):
import _ "github.com/kakj-go/go-inject-trace-contrib/skywalking"
```

```go
// inject.go —— go-inject 规则注册文件:
//go:build goinject || generate

package main

import _ "github.com/kakj-go/go-inject-trace-contrib/skywalking"
```

```bash
go mod tidy
go build -toolexec=go-inject .
```

运行时配置读取 `SW_AGENT_*` 环境变量。必配项(不配也有默认值,但只
适合本地冒烟测试):

| 变量 | 含义 | 不设置时的默认值 |
|---|---|---|
| `SW_AGENT_NAME` | 服务名 | `Your_ApplicationName` |
| `SW_AGENT_REPORTER_GRPC_BACKEND_SERVICE` | OAP 后端地址 | `127.0.0.1:11800` |

其余变量——采样、reporter 各类间隔、认证、插件选项
(`SW_AGENT_PLUGIN_CONFIG_*`)——与[官方 skywalking-go agent][swcfg]
使用同一套变量集,与其兼容。

[swcfg]: https://skywalking.apache.org/docs/skywalking-go/next/en/setup/configurations/

## 目录结构

```
otelc/                   # OpenTelemetry 插桩(27 个规则包)
  inject.go              # 规则聚合
  boot/                  # OTEL_* 环境变量装配、providers、信号 flush
  runtime/               # goroutine 本地存储(g-struct + newproc1)
  sdktrace/              # sdk/trace 的 GLS span 栈
  genai/                 # openai/anthropic 共用 middleware
  plugins/<name>/        # 每个框架一个规则包(22 个框架)
skywalking/              # SkyWalking 插桩
  activate.go            # 让 plugins/core 留在依赖图中
  inject.go              # 规则聚合
  boot/                  # 运行时激活:链接 GLS、启动 tracer
  bootinit/              # //inject:main 规则,注入 boot 初始化器
  coreinit/              # 向 plugins/core 注入 tracer 构造器
  runtime/               # 注入 runtime 包的 GLS
  plugins/<name>/        # 每个框架一个规则包
test/
  mockcol/               # 仓库内 OTLP 接收端,用于 A/B 验证
  runner/normdiff.py     # skywalking A/B 差异
  runner/normdiff_otlp.py # otelc A/B 差异
  runner/normdiff_log.py  # otelc log 桥接 stdout 差异
  runner/run_ab.py        # skywalking 场景 runner
  runner/run_ab_otelc.py  # otelc 场景 runner
  runner/run_all.py       # skywalking 全量套件
  runner/run_all_otelc.py # otelc 全量套件
  scenarios/              # skywalking 场景
  scenarios-otelc/        # otelc 场景(22 个)
```

插件规则包采用 tag 拆分约定:带 `goinject` tag 的文件是模板(对着真实
目标包解析,绝不单独编译);`!goinject` 的 stub 保证规则包在普通构建下
也能编译。

## 已插桩的库(otelc)

`net/http`(server+client)、`gin-gonic/gin`、`database/sql`、
`log/slog`、`log`、`logrus`、`zap`、`google.golang.org/grpc`、
`segmentio/kafka-go`、`redis/go-redis/v9`、`mongo-driver`(v1+v2)、
`rabbitmq/amqp091-go`、`olivere/elastic/v7`、
`cassandra-gocql-driver/v2`、`aws-sdk-go-v2`、`openai/openai-go`
(v1/v2/v3)、`anthropics/anthropic-sdk-go`、`linode/linodego/v2`、
`k8s.io/client-go`、Go 运行时指标。

## 已插桩的库(skywalking)

完整列表(33 个框架)见 [skywalking/README_CN.md](skywalking/README_CN.md)
或[场景目录](test/scenarios)。

## 验证

每个插件都与官方工具做过 1:1 验证:同一个场景程序构建两次——一次用
官方工具(`otelc go build` 或 `go build -toolexec=skywalking-go`),
一次用 go-inject + 本项目——两者都上报到 collector,规范化易变字段
(id、时间戳、实例名)之后收到的遥测数据必须一致。

- **otelc**:21 个场景 × Go 1.25/1.26/1.27,通过 `normdiff_otlp.py`
  做规范化 OTLP 结构等价校验(span、metric、log)。log 桥接场景额外
  diff 规范化后的应用 stdout。k8s 场景需要 Linux CI 环境(特权 k3s
  容器)。
- **skywalking**:33 个场景,先经官方 mock-collector 的
  `/dataValidate` 端点校验,再通过 `normdiff.py` 做规范化 diff。

注意:官方 skywalking go-agent 有一个 Windows 专属 bug(模块版本检测
使用正斜杠路径),所以官方参照构建在 Linux 容器中运行;go-inject 构建
在 go-inject 支持的任何地方都能跑。

## 环境要求

- Go 1.25–1.27
- [go-inject](https://github.com/kakj-go/go-inject) ≥ v0.1.0-beta.3
  (otelc 规则依赖其 map 与 struct 字面量 key 重命名的修复)
