# otelc — OpenTelemetry compile-time instrumentation as go-inject rules

Port of [opentelemetry-go-compile-instrumentation](https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation)
(otelc) onto [go-inject](https://github.com/kakj-go/go-inject): the same
interception points, span semantics, attribute sets, and runtime flow as the
official tool, delivered as declarative go-inject templates instead of
trampoline + `go:linkname` code generation.

```go
// inject.go (registration file):
//go:build goinject || generate

package main

import _ "github.com/kakj-go/go-inject-trace-contrib/otelc"
```

```sh
go build -toolexec=go-inject .
```

Configuration is read at runtime from the standard `OTEL_*` environment
variables (autoexport/autoprop: `OTEL_SERVICE_NAME`,
`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_TRACES/METRICS/LOGS_EXPORTER`,
`OTEL_PROPAGATORS`, `OTEL_TRACES_SAMPLER`, …), plus
`OTEL_GO_ENABLED/DISABLED_INSTRUMENTATIONS` per-library gating — matching the
upstream `pkg/runtime` behavior.

## Layout

```
otelc/
  inject.go              # rule aggregation (goinject tag)
  activate.go            # keeps otel API/SDK in the dependency graph
  boot/                  # SetupOTelSDK: providers, propagators, signal flush
  bootinit/              # //inject:main adding the boot initializer
  runtime/               # GLS: g-struct fields + newproc1 propagation
  sdktrace/              # sdk/trace GLS span stack (push/pop hooks)
  oteltrace/             # SpanFromContext GLS fallback
  otelroot/              # SetTracerProvider one-shot guard
  otelgls/               # zero-dep registry bridging GLS across packages
  hooksupport/           # light runtime API (enabler, version, suppression)
  hooklog/               # hook logger (split from hooksupport: log/slog cycle)
  httpapi/               # net/http link-bridge target (otel→net/http cycle)
  k8sapi/                # k8s event-handler wrapper (scheme import cycle)
  dbsemconv/ dsnparse/   # database/sql helpers (copied from upstream)
  genai/ + genai/streaming/ # shared openai/anthropic GenAI middlewares
  plugins/
    nethttp/ gin/                       # HTTP server/client + gin routes
    dbsql/                              # database/sql ops + connection fields
    logslog/ logstdlib/ logrus/ zap/    # log bridges (trace_id/span_id)
    grpc/                               # stats.Handler spans + rpc.* metrics
    kafkago/ redisv9/                   # messaging / cache clients
    mongov1/ mongov2/                   # otelmongo monitor injection
    amqp/ elasticv7/ gocqlv2/           # rabbitmq, elasticsearch, cassandra
    awssdkv2/                           # otelaws middleware injection
    openaiv1/ openaiv2/ openaiv3/       # GenAI middleware (shared genai pkg)
    anthropic/                          # GenAI middleware (shared genai pkg)
    linodego/                           # doRequest + public method spans
    k8sclientgo/                        # informer delta processing spans
```

## Divergences from upstream (all forced by the injection model)

- Inline templates replace the trampoline/`go:linkname` hooks; the hook bodies
  themselves are ported line-for-line.
- `pkg/runtime` is split into `hooksupport`/`hooklog`/`otelgls`/`boot` so that
  code injected into stdlib packages (`log`, `log/slog`, `net/http`) never
  imports a package that imports its own target.
- Code injected into `net/http` reaches otel through go-inject link bridges
  (`otelc/httpapi`): importing `go.opentelemetry.io/otel` from inside
  `net/http` would close an import cycle (otel → propagation → net/http).
- Same for the k8s informer hook: the event-handler wrapper lives in
  `otelc/k8sapi` behind a bridge, since injecting
  `k8s.io/client-go/kubernetes/scheme` into `tools/cache` risks a cycle.

## Verification

`test/runner/run_ab_otelc.py` builds every scenario twice — A with the
official `otelc` binary, B with `go build -toolexec=go-inject` — against the
in-repo mock OTLP collector (`test/mockcol`), then requires the normalized
telemetry to match (`normdiff_otlp.py`; stdout diff for log scenarios). See
`test/scenarios-otelc/` for the scenario list and `run_all_otelc.py` for the
full suite.

Not yet scenario-validated: k8s client-go (the plugin is complete, builds,
and was verified with `go build -toolexec` locally; the upstream-mirroring
scenario needs a privileged k3s dependency container that does not start
reliably under Docker Desktop on Windows — run it in a Linux CI
environment). linodego public-method templates cover the methods the
scenario suite exercises — extend with more three-line method templates as
coverage grows.
