# go-inject-trace-contrib

Compile-time instrumentation for Go services, delivered as [go-inject](https://github.com/kakj-go/go-inject)
rule templates. One import plus one build flag — no framework-specific
middleware, no manual span code. Two instrumentation backends live in this
repository:

- **[`otelc/`](otelc/)** — [OpenTelemetry](https://opentelemetry.io) trace +
  metric + log (port of
  [opentelemetry-go-compile-instrumentation](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation))
- **[`skywalking/`](skywalking/)** — Apache SkyWalking tracing (port of
  [apache/skywalking-go](https://github.com/apache/skywalking-go))

Both produce telemetry structurally identical to their official counterparts,
verified 1:1 with A/B scenario builds against the official tools (see
Verification below).

## Usage

### OpenTelemetry (`otelc`)

```go
// main.go — a normal import (keeps otel SDK in the dependency graph):
import _ "github.com/kakj-go/go-inject-trace-contrib/otelc"
```

```go
// inject.go — the go-inject registration file:
//go:build goinject || generate

package main

import _ "github.com/kakj-go/go-inject-trace-contrib/otelc"
```

```bash
go mod tidy          # after adding the import
go build -toolexec=go-inject .
```

Configuration is read at runtime from the standard `OTEL_*` environment
variables — `OTEL_SERVICE_NAME`, `OTEL_EXPORTER_OTLP_ENDPOINT`,
`OTEL_TRACES/METRICS/LOGS_EXPORTER`, `OTEL_PROPAGATORS`,
`OTEL_TRACES_SAMPLER`, `OTEL_SDK_DISABLED` — plus
`OTEL_GO_ENABLED/DISABLED_INSTRUMENTATIONS` for per-library gating, matching
the upstream otelc behavior.

### Apache SkyWalking (`skywalking`)

```go
// any file in package main (a normal import):
import _ "github.com/kakj-go/go-inject-trace-contrib/skywalking"
```

```go
// inject.go — the go-inject registration file:
//go:build goinject || generate

package main

import _ "github.com/kakj-go/go-inject-trace-contrib/skywalking"
```

```bash
go mod tidy
go build -toolexec=go-inject .
```

Configuration is read at runtime from `SW_AGENT_*` environment variables
(service name, backend address, sampling, and per-plugin options), matching
the official agent's variable set.

## Layout

```
otelc/                   # OpenTelemetry instrumentation (27 rule packages)
  inject.go              # rule aggregation
  boot/                  # OTEL_* env setup, providers, signal flush
  runtime/               # goroutine-local storage (g-struct + newproc1)
  sdktrace/              # sdk/trace GLS span stack
  genai/                 # shared openai/anthropic middlewares
  plugins/<name>/        # one rule package per framework (22 frameworks)
skywalking/              # SkyWalking instrumentation
  activate.go            # keeps plugins/core in the dependency graph
  inject.go              # rule aggregation
  boot/                  # runtime activation: links GLS, boots the tracer
  bootinit/              # //inject:main rule adding the boot initializer
  coreinit/              # rule adding the tracer constructor to plugins/core
  runtime/               # GLS injected into the runtime package
  plugins/<name>/        # one rule package per framework
test/
  mockcol/               # in-repo OTLP receiver for A/B validation
  runner/normdiff.py     # skywalking A/B diff
  runner/normdiff_otlp.py # otelc A/B diff
  runner/normdiff_log.py  # otelc log-bridge stdout diff
  runner/run_ab.py        # skywalking scenario runner
  runner/run_ab_otelc.py  # otelc scenario runner
  runner/run_all.py       # skywalking full suite
  runner/run_all_otelc.py # otelc full suite
  scenarios/              # skywalking scenarios
  scenarios-otelc/        # otelc scenarios (22)
```

Plugin rule packages use the tag-split convention: the `goinject`-tagged file
is the template (parsed against the real target package, never compiled
standalone); the `!goinject` stub keeps the rule package buildable normally.

## Instrumented libraries (otelc)

`net/http` (server+client), `gin-gonic/gin`, `database/sql`, `log/slog`,
`log`, `logrus`, `zap`, `google.golang.org/grpc`, `segmentio/kafka-go`,
`redis/go-redis/v9`, `mongo-driver` (v1+v2), `rabbitmq/amqp091-go`,
`olivere/elastic/v7`, `cassandra-gocql-driver/v2`, `aws-sdk-go-v2`,
`openai/openai-go` (v1/v2/v3), `anthropics/anthropic-sdk-go`,
`linode/linodego/v2`, `k8s.io/client-go`, Go runtime metrics.

## Instrumented libraries (skywalking)

See [skywalking/README.md](skywalking/README.md) or the
[scenario directory](test/scenarios) for the complete list (33 frameworks).

## Verification

Every plugin is validated 1:1 against its official tool: the same scenario
program is built twice — once with the official tool (`otelc go build` or
`go build -toolexec=skywalking-go`) and once with go-inject + this project —
both report to a collector, and the received telemetry must match after
normalizing volatile fields (ids, timestamps, instance names).

- **otelc**: 21 scenarios × Go 1.25/1.26/1.27, normalized OTLP structural
  equality (spans, metrics, logs) via `normdiff_otlp.py`. Log-bridge
  scenarios additionally diff normalized application stdout. The k8s
  scenario requires a Linux CI environment (privileged k3s container).
- **skywalking**: 33 scenarios, validated against the official
  mock-collector's `/dataValidate` endpoint, then normalized-diffed via
  `normdiff.py`.

Note: the official skywalking go-agent has a Windows-only bug (module-version
detection uses forward-slash paths), so official-reference builds run in
Linux containers; go-inject builds work everywhere go-inject does.

## Requirements

- Go 1.25–1.27
- [go-inject](https://github.com/kakj-go/go-inject) ≥ v0.1.0-beta.3 (the
  otelc rules depend on the map-vs-struct-literal-key renaming fix)
