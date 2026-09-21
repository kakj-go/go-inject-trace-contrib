# otelc —— 以 go-inject 规则交付的 OpenTelemetry 编译期插桩

[English](README.md) | [中文](README_CN.md)

[opentelemetry-go-compile-instrumentation](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation)
(otelc)向 [go-inject](https://github.com/kakj-go/go-inject) 的移植:
拦截点、span 语义、属性集合、运行时流程与官方工具一致,只是以声明式
go-inject 模板交付,替代 trampoline + `go:linkname` 代码生成。

```go
// inject.go(规则注册文件):
//go:build goinject || generate

package main

import _ "github.com/kakj-go/go-inject-trace-contrib/otelc"
```

```sh
go build -toolexec=go-inject .
```

## 配置

运行时配置读取标准的 `OTEL_*` 环境变量(经 autoexport/autoprop/SDK
resource 装配),与上游 `pkg/runtime` 行为一致。

必配项(不配也有默认值,但只适合本地冒烟测试):

| 变量 | 含义 | 不设置时的默认值 |
|---|---|---|
| `OTEL_SERVICE_NAME` | 服务名 | `unknown_service:<可执行文件名>` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector 基础 URL | `http://localhost:4318` |

其余变量——`OTEL_TRACES/METRICS/LOGS_EXPORTER`、按信号的 endpoint、
协议、header、超时、`OTEL_PROPAGATORS`、`OTEL_SDK_DISABLED`、
`OTEL_LOG_LEVEL`,以及按库门控
`OTEL_GO_ENABLED/DISABLED_INSTRUMENTATIONS`(名字如 `nethttp`、
`grpc`、`database`、`logs/slog`……)——与
[OpenTelemetry 环境变量规范][otelnv] 和[上游 otelc][upstream]
兼容;完整变量列表与默认值参考其官方文档。

两条移植专属说明:

- `OTEL_TRACES_SAMPLER` 尚未接线:采样恒为
  `ParentBased(AlwaysSample)`。
- 批处理器参数固定(1 秒 flush、每批 512 个 span),不读取
  `OTEL_BSP_*`。与上游一致,`OTEL_GO_SIMPLE_SPAN_PROCESSOR=true`
  切换为立即导出(SimpleSpanProcessor)。

[otelnv]: https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/
[upstream]: https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation

## 目录结构

```
otelc/
  inject.go              # 规则聚合(goinject tag)
  activate.go            # 让 otel API/SDK 留在依赖图中
  boot/                  # SetupOTelSDK:providers、propagator、信号 flush
  bootinit/              # //inject:main,注入 boot 初始化器
  runtime/               # GLS:g-struct 字段 + newproc1 传播
  sdktrace/              # sdk/trace 的 GLS span 栈(push/pop 钩子)
  oteltrace/             # SpanFromContext 的 GLS 兜底
  otelroot/              # SetTracerProvider 一次性守卫
  otelgls/               # 零依赖注册表,跨包桥接 GLS
  hooksupport/           # 轻量运行时 API(开关、版本、抑制)
  hooklog/               # hook 日志(从 hooksupport 拆出:log/slog 循环)
  httpapi/               # net/http link 桥接目标(otel→net/http 循环)
  k8sapi/                # k8s 事件处理包装(scheme import 循环)
  dbsemconv/ dsnparse/   # database/sql 辅助(自上游复制)
  genai/ + genai/streaming/ # openai/anthropic 共用 GenAI middleware
  plugins/
    nethttp/ gin/                       # HTTP server/client + gin 路由
    dbsql/                              # database/sql 操作 + 连接字段
    logslog/ logstdlib/ logrus/ zap/    # log 桥接(trace_id/span_id)
    grpc/                               # stats.Handler span + rpc.* 指标
    kafkago/ redisv9/                   # 消息 / 缓存客户端
    mongov1/ mongov2/                   # otelmongo monitor 注入
    amqp/ elasticv7/ gocqlv2/           # rabbitmq、elasticsearch、cassandra
    awssdkv2/                           # otelaws middleware 注入
    openaiv1/ openaiv2/ openaiv3/       # GenAI middleware(共用 genai 包)
    anthropic/                          # GenAI middleware(共用 genai 包)
    linodego/                           # doRequest + 公开方法 span
    k8sclientgo/                        # informer delta 处理 span
```

## 与上游的差异(均由注入模型所迫)

- 内联模板替代 trampoline/`go:linkname` 钩子;钩子函数体逐行移植。
- `pkg/runtime` 拆分为 `hooksupport`/`hooklog`/`otelgls`/`boot`,保证
  注入标准库包(`log`、`log/slog`、`net/http`)的代码不会 import 一个
  反过来 import 自己目标的包。
- 注入 `net/http` 的代码通过 go-inject link 桥(`otelc/httpapi`)访问
  otel:在 `net/http` 内部 import
  `go.opentelemetry.io/otel` 会闭合 import 环
  (otel → propagation → net/http)。
- k8s informer 钩子同理:事件处理包装放在 `otelc/k8sapi` 桥后,因为向
  `tools/cache` 注入 `k8s.io/client-go/kubernetes/scheme` 有成环风险。

## 验证

`test/runner/run_ab_otelc.py` 把每个场景构建两次——A 用官方 `otelc`
二进制,B 用 `go build -toolexec=go-inject`——都上报到仓库内的 mock
OTLP collector(`test/mockcol`),然后要求规范化后的遥测一致
(`normdiff_otlp.py`;log 场景额外 diff stdout)。场景列表见
`test/scenarios-otelc/`,全量套件见 `run_all_otelc.py`。

尚未做场景验证:k8s client-go(插件完整、可构建,并已在本地用
`go build -toolexec` 验证;对齐上游的场景需要特权 k3s 依赖容器,在
Windows Docker Desktop 下启动不稳定——请在 Linux CI 环境运行)。
linodego 公开方法模板覆盖了场景套件用到的方法——随覆盖面扩大,再按
"三行方法模板"的样式补充。
