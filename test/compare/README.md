# Cross-backend trace comparison (skywalking vs otelc)

Unlike the A/B suites (`../runner/`), which verify each backend 1:1 against
its official counterpart, this directory compares the **two backends against
each other**: the same demo service is built twice — once instrumented by
`skywalking/`, once by `otelc/` — and the resulting telemetry is normalized
to semantic roles and diffed.

## The demo service (`demoapp/`)

A two-service "order" flow exercising gin, net/http client, gRPC, MySQL
(database/sql), Redis (go-redis v9), slog/log, and one cross-goroutine span:

```
demo-frontend (gin :8080)                 demo-backend (http :8081, grpc :9090)
GET /api/order
 ├─ db INSERT orders
 ├─ redis SET order:<id>
 ├─ http GET /api/inventory ─────────────► GET /api/inventory
 │                                          └─ db SELECT inventory
 ├─ grpc Inventory/Reserve ───────────────► Inventory/Reserve
 │                                          ├─ redis SET reserve:<sku>
 │                                          └─ db UPDATE inventory
 └─ goroutine: redis GET order:<id>
```

Backend selection is by build tag (`sky` / `otel`), see
`demoapp/cmd/*/be_*.go` (runtime import) and `inj_*.go` (goinject rule
registration). grpc stubs are regenerated with `demoapp/api/gen.sh`
(dockerized protoc).

Dependency notes learned the hard way:
- go-redis must be **v9.22.0**: the otelc redis rule uses go1.21 builtins but
  v9.0.5 declares `-lang=go1.18`; the skywalking rule works on both.
- one `go mod tidy -e` is needed: skywalking's kratos rule template imports a
  kratos test-only package (`grpc/test/grpc_testing`) that no longer exists in
  modern grpc — ignorable, nothing in the build graph needs it.
- requires go-inject ≥ the "支撑 otelc trace 功能" commit (older betas crash
  rewriting go-redis).

## Running it

```bash
# infra (ports: mysql 33061, redis 63791, sw-collector 19876/12800)
docker run -d --name cmp-mysql -e MYSQL_ROOT_PASSWORD=password -e MYSQL_USER=user \
  -e MYSQL_PASSWORD=password -e MYSQL_DATABASE=demo -p 33061:3306 mysql:5.7
docker run -d --name cmp-redis -p 63791:6379 redis:7.4-alpine
docker run -d --name cmp-swmock -p 19876:19876 -p 12800:12800 \
  ghcr.io/apache/skywalking-agent-test-tool/mock-collector:b22b7d8ba62dabdd8db1ecc52da6178b063edff7
cd ../../mockcol && MOCKCOL_ADDR=:4318 go run . &     # OTLP receiver

cd ../compare/demoapp
for be in sky otel; do
  go build -a -toolexec=go-inject -tags $be -o bin/backend-$be.exe  ./cmd/backend
  go build -a -toolexec=go-inject -tags $be -o bin/frontend-$be.exe ./cmd/frontend
done

# round 1: skywalking
(export SW_AGENT_NAME=demo-backend  SW_AGENT_REPORTER_GRPC_BACKEND_SERVICE=127.0.0.1:19876 \
  SW_AGENT_PLUGIN_CONFIG_SQL_COLLECT_PARAMETER=true; ./bin/backend-sky.exe &)
(export SW_AGENT_NAME=demo-frontend SW_AGENT_REPORTER_GRPC_BACKEND_SERVICE=127.0.0.1:19876 \
  SW_AGENT_PLUGIN_CONFIG_SQL_COLLECT_PARAMETER=true; ./bin/frontend-sky.exe &)
curl 'http://127.0.0.1:8080/api/order?sku=SKU-42'    # x3
curl -s http://127.0.0.1:12800/receiveData > sky_traces.json   # YAML!

# round 2: otelc
(export OTEL_SERVICE_NAME=demo-backend  OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318 \
  OTEL_TRACES_EXPORTER=otlp OTEL_METRICS_EXPORTER=none; ./bin/backend-otel.exe &)
(export OTEL_SERVICE_NAME=demo-frontend OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318 \
  OTEL_TRACES_EXPORTER=otlp OTEL_METRICS_EXPORTER=none; ./bin/frontend-otel.exe &)
curl 'http://127.0.0.1:8080/api/order?sku=SKU-42'    # x3
curl -s http://127.0.0.1:4318/receiveData > otel_traces.json

python ../cmp_traces.py --sky sky_traces.json --otel otel_traces.json
```

## Result (2026-09-21, go1.25.3, gin v1.10.1 / grpc v1.81.1 / go-redis v9.22.0)

Full output in `compare_report.txt`. Summary:

- **3 business traces on each side, 0 hard mismatches.** All 13 semantic
  roles (http server/client, grpc server/client, db INSERT/SELECT/UPDATE,
  redis set/get) match 1:1 in count, service placement, and parent-child
  topology, including the cross-goroutine span and both cross-process hops
  (http + grpc propagation stitch correctly on both sides).
- Convention differences (expected, not bugs):
  - skywalking names spans `GET:/api/order` / `Mysql/Exec` / `redis/set`;
    otelc uses route/verb names `GET /api/order` / `INSERT` / `set`.
  - skywalking's grpc plugin adds Local detail spans
    (`.../Client/Request/SendMsg`, `.../RecvMsg`, `.../SendResponse`) that
    otelc folds into the single rpc span (3 extra spans per request).
  - attributes model the same facts under different keys: `db.statement` +
    `db.sql.parameters` (sky, even collects bound params) vs `db.query.text`
    + `db.namespace` (otel); `cache.cmd/key/args` vs
    `db.operation.name`/`db.query.text`; `peer` vs `server.address:port`.
  - redis handshake spans are grouped differently (`redis/dial` as Exit
    spans vs `hello`/`client` nested under the startup trace).
  - otelc additionally bridges slog/stdlib logs into log records (not
    captured in this run — default slog handler bypasses the bridge);
    skywalking has no log signal at all by design.
