#!/usr/bin/env python3
"""Port official skywalking-go test scenarios into A/B scenario directories.

Transformations per scenario:
  - main.go: add the contrib aggregator blank import (shared-source A/B)
  - excepted.yml: copied verbatim
  - go.mod.tpl: official go.mod with the framework version templated, plus
    skywalking-go/contrib/genproto pins and the /contrib replace
  - plugin.yml: normalized fields (paths, port, env from startup.sh,
    dependencies verbatim) and matrix cells limited to go-inject's Go series
"""
import os
import re
import shutil
import sys

import yaml

OFFICIAL = r"D:\workspace\go\skywalking-go\test\plugins\scenarios"
OUT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "scenarios"))
SKIP = {"logrus", "zap", "plugin_exclusion"}  # logger instruments are stretch
FRAMEWORK_PINS = {
    "dubbo.apache.org/dubbo-go/v3": "v3.0.1",
    "github.com/apache/pulsar-client-go": "v0.12.0",
    "github.com/apache/rocketmq-client-go/v2": "v2.1.2",
    "github.com/elastic/go-elasticsearch/v8": "v8.11.1",
    "github.com/emicklei/go-restful/v3": "v3.10.2",
    "github.com/go-kratos/kratos/v2": "v2.6.2",
    "github.com/go-sql-driver/mysql": "v1.7.1",
    "github.com/gofiber/fiber/v2": "v2.50.0",
    "github.com/gogf/gf/v2": "v2.7.3",
    "github.com/gorilla/mux": "v1.8.0",
    "github.com/jackc/pgx/v5": "v5.5.5",
    "github.com/kataras/iris/v12": "v12.2.0",
    "github.com/labstack/echo/v4": "v4.11.4",
    "github.com/rabbitmq/amqp091-go": "v1.9.0",
    "github.com/redis/go-redis/v9": "v9.0.5",
    "github.com/segmentio/kafka-go": "v0.4.47",
    "github.com/valyala/fasthttp": "v1.51.0",
    "go-micro.dev/v4": "v4.9.0",
    "go.mongodb.org/mongo-driver": "v1.11.7",
    "google.golang.org/grpc": "v1.56.2",
    "gorm.io/driver/mysql": "v1.5.1",
    "gorm.io/driver/postgres": "v1.5.2",
    "gorm.io/gorm": "v1.25.1",
}


def additions(fw):
    """Pin only the minimum: skywalking-go, contrib, genproto legacy fix,
    and the framework set (so rule packages resolve during discovery).
    The scenario's own framework is templated, excluded from pins, and
    force-pinned via replace to prevent MVS version drift."""
    reqs = ["\tgithub.com/apache/skywalking-go v0.7.0",
            "\tgithub.com/kakj-go/go-inject-trace-contrib v0.0.0",
            "\tgoogle.golang.org/genproto v0.0.0-20240213162025-012b6fc9bca9"]
    reqs += [f"\t{m} {v}" for m, v in sorted(FRAMEWORK_PINS.items()) if m != fw]
    reqs.append("\tgithub.com/apache/skywalking-go/toolkit v0.7.0")
    result = "\nrequire (\n" + "\n".join(reqs) + "\n)\n"
    result += "\nreplace github.com/kakj-go/go-inject-trace-contrib => /contrib\n"
    if fw != "google.golang.org/grpc":
        # pin volatile transitive deps to go1.24-compatible versions; the
        # grpc scenario needs newer x/net for v1.81+ (ReadFrameHeader)
        result += "replace golang.org/x/net => golang.org/x/net v0.33.0\n"
        result += "replace golang.org/x/crypto => golang.org/x/crypto v0.31.0\n"
        result += "replace golang.org/x/sys => golang.org/x/sys v0.28.0\n"
        result += "replace golang.org/x/text => golang.org/x/text v0.20.0\n"
        result += "replace google.golang.org/grpc => google.golang.org/grpc v1.70.0\n"
        result += "replace google.golang.org/genproto/googleapis/rpc => google.golang.org/genproto/googleapis/rpc v0.0.0-20240318140521-94a12d6c2237\n"
    else:
        result += "\nreplace google.golang.org/grpc => google.golang.org/grpc {{FRAMEWORK_VERSION}}\n"
    # otel and prometheus drift to versions requiring go >= 1.25
    result += "replace go.opentelemetry.io/otel => go.opentelemetry.io/otel v1.24.0\n"
    result += "replace go.opentelemetry.io/otel/trace => go.opentelemetry.io/otel/trace v1.24.0\n"
    result += "replace go.opentelemetry.io/otel/metric => go.opentelemetry.io/otel/metric v1.24.0\n"
    result += "replace go.opentelemetry.io/otel/sdk => go.opentelemetry.io/otel/sdk v1.24.0\n"
    result += "replace github.com/prometheus/client_golang => github.com/prometheus/client_golang v1.19.1\n"
    # klog v2.9 is incompatible with newer logr; pin both to compatible pair
    result += "replace k8s.io/klog/v2 => k8s.io/klog/v2 v2.110.1\n"
    result += "replace github.com/go-logr/logr => github.com/go-logr/logr v1.3.0\n"
    return result


def adapt_main(src):
    if "go-inject-trace-contrib" in src:
        return src
    imp = '\t_ "github.com/kakj-go/go-inject-trace-contrib/skywalking"\n'
    marker = '_ "github.com/apache/skywalking-go"\n'
    if marker in src:
        return src.replace(marker, marker + imp)
    return src.replace('"\n\n', '"' + imp + '\n\n', 1)


MULTI_APP = {
    "grpc": {
        "build_steps": "\n".join([
            "apt-get update -qq && apt-get install -y -qq protobuf-compiler",
            "go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28",
            "go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2",
            "export PATH=$PATH:$(go env GOPATH)/bin",
            "protoc -I={WS}/api --go_out={WS}/api --go-grpc_out={WS}/api {WS}/api/api.proto",
        ]),
        "build_targets": [{"out": "grpc-server", "pkg": "/ws/grpc_server"},
                          {"out": "grpc-client", "pkg": "/ws/grpc_client"}],
        "app_script": "SW_AGENT_NAME=grpc-server {WS}/grpc-server & sleep 3; exec env SW_AGENT_NAME=grpc-client {WS}/grpc-client",
    },
    "dubbo": {
        "build_steps": "\n".join([
            "apt-get update -qq && apt-get install -y -qq protobuf-compiler",
            "go install github.com/dubbogo/tools/cmd/protoc-gen-dubbo3grpc@latest",
            "go install github.com/dubbogo/tools/cmd/protoc-gen-go-triple@v1.0.10-rc2",
            "go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28",
            "go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2",
            "export PATH=$PATH:$(go env GOPATH)/bin",
            "protoc -I={WS}/api --go_out={WS}/api --go-triple_out={WS}/api {WS}/api/api.proto",
        ]),
        "build_targets": [{"out": "dubbo-server", "pkg": "/ws/go-server/cmd/server.go"},
                          {"out": "dubbo-client", "pkg": "/ws/go-client/cmd/client.go"}],
        "app_script": ("DUBBO_GO_CONFIG_PATH={WS}/go-server/conf/dubbogo.yaml SW_AGENT_NAME=dubbo-server {WS}/dubbo-server & sleep 5; "
                       "exec env DUBBO_GO_CONFIG_PATH={WS}/go-client/conf/dubbogo.yaml SW_AGENT_NAME=dubbo-client {WS}/dubbo-client"),
    },
    "microv4": {
        "build_targets": [{"out": "micro-server", "pkg": "/ws/server.go"},
                          {"out": "micro-client", "pkg": "/ws/client.go"}],
        "app_script": "SW_AGENT_NAME=micro-server {WS}/micro-server & sleep 5; exec env SW_AGENT_NAME=micro-client {WS}/micro-client",
    },
}


def adapt_tree(src_dir, out_dir):
    """Copy the whole scenario tree, adding the contrib import to every Go
    file that carries the official skywalking activation import."""
    for root, dirs, files in os.walk(src_dir):
        dirs[:] = [d for d in dirs if d not in ("bin", "config")]
        rel = os.path.relpath(root, src_dir)
        for f in files:
            src = os.path.join(root, f)
            dst = os.path.join(out_dir, rel, f)
            os.makedirs(os.path.dirname(dst), exist_ok=True)
            data = open(src, encoding="utf-8").read()
            if f.endswith(".go"):
                data = adapt_main(data)
            with open(dst, "w", encoding="utf-8", newline="\n") as w:
                w.write(data)


def port(name, force=False):
    src_dir = os.path.join(OFFICIAL, name)
    out_dir = os.path.join(OUT, name)
    if os.path.exists(out_dir) and not force:
        return "exists"
    meta = yaml.safe_load(open(os.path.join(src_dir, "plugin.yml"), encoding="utf-8"))
    health_path = re.sub(r"^http://[^/]+", "", meta.get("health-checker", "")).split("${")[0] or "/health"
    entry_path = re.sub(r"^http://[^/]+", "", meta.get("entry-service", "")).split("${")[0] or "/"
    port_no = str(meta.get("export-port", 8080))
    env = {}
    startup = os.path.join(src_dir, "bin", "startup.sh")
    if os.path.exists(startup):
        for m in re.finditer(r"export (SW_[A-Z0-9_]+)=(\S+)", open(startup, encoding="utf-8").read()):
            env[m.group(1)] = m.group(2).strip('"')
    cells = [row for row in meta.get("support-version", []) if str(row["go"]) in ("1.24", "1.25", "1.26", "1.27")]
    if not cells:
        return "no supported go cell"
    # go1.24-only matrices hit transitive-dep version drift under -mod=mod
    # resolution; mirror them onto go1.25 so the A/B comparison still runs
    if cells and all(str(row["go"]) == "1.24" for row in cells):
        cells = [dict(row, go="1.25") for row in cells]
    our = {
        "entry": entry_path,
        "health": health_path,
        "port": port_no,
        "env": env,
        "dependencies": meta.get("dependencies", {}),
        "support-version": cells,
    }
    if name in MULTI_APP:
        our.update(MULTI_APP[name])
        adapt_tree(src_dir, out_dir)
        # each entry binary builds from its own package directory; the
        # goinject registration file must live there to be discovered
        reg = ('//go:build goinject || generate\n\npackage main\n\n'
               'import _ "github.com/kakj-go/go-inject-trace-contrib/skywalking"\n')
        for root, dirs, files in os.walk(out_dir):
            for f in files:
                if not f.endswith(".go"):
                    continue
                head = open(os.path.join(root, f), encoding="utf-8").read()[:1200]
                if "package main" in head:
                    with open(os.path.join(root, "inject.go"), "w", encoding="utf-8", newline="\n") as w:
                        w.write(reg)
                    break
    else:
        os.makedirs(out_dir, exist_ok=True)
        main = open(os.path.join(src_dir, "main.go"), encoding="utf-8").read()
        open(os.path.join(out_dir, "main.go"), "w", encoding="utf-8", newline="\n").write(adapt_main(main))
        # copy additional .go files at the scenario root (helpers, test services)
        for f in os.listdir(src_dir):
            if f.endswith(".go") and f != "main.go":
                data = open(os.path.join(src_dir, f), encoding="utf-8").read()
                open(os.path.join(out_dir, f), "w", encoding="utf-8", newline="\n").write(data)
    open(os.path.join(out_dir, "inject.go"), "w", encoding="utf-8", newline="\n").write(
        "//go:build goinject || generate\n\npackage main\n\n"
        "import _ \"github.com/kakj-go/go-inject-trace-contrib/skywalking\"\n")
    shutil.copy(os.path.join(src_dir, "config", "excepted.yml"), os.path.join(out_dir, "excepted.yml"))
    # carry the official go.sum so -mod=mod resolution stays anchored to
    # the tested transitive dependency versions instead of drifting to latest
    official_sum = os.path.join(src_dir, "go.sum")
    if os.path.exists(official_sum):
        shutil.copy(official_sum, os.path.join(out_dir, "go.sum.official"))
    gomod = open(os.path.join(src_dir, "go.mod"), encoding="utf-8").read()
    fw = meta.get("framework", "")
    # some plugin.yml framework names differ from the actual Go module path
    # (e.g. github.com/gogf/gf vs github.com/gogf/gf/v2)
    fw_module = {"github.com/gogf/gf": "github.com/gogf/gf/v2"}.get(fw, fw)
    # "framework: go" names the toolchain itself (stdlib-only scenarios); any
    # framework whose matrix cells carry no versions is versionless too.
    has_version = bool(fw) and fw != "go" and any(v for row in cells for v in (row.get("framework") or []))
    if has_version:
        gomod = re.sub(rf"(\n(?:\t|require ){re.escape(fw_module)} )v\S+", r"\1{{FRAMEWORK_VERSION}}", gomod, count=1)
        if "{{FRAMEWORK_VERSION}}" not in gomod:
            gomod = gomod.rstrip() + f"\n\nrequire {fw_module} {{{{FRAMEWORK_VERSION}}}}\n"
    gomod = gomod.rstrip() + additions(fw_module if has_version else '')
    open(os.path.join(out_dir, "go.mod.tpl"), "w", encoding="utf-8", newline="\n").write(gomod)
    yaml.safe_dump(our, open(os.path.join(out_dir, "plugin.yml"), "w", encoding="utf-8"), sort_keys=False)
    return "ported"


def main():
    force = "--force" in sys.argv
    names = sys.argv[1:]
    names = [n for n in names if not n.startswith("--")]
    if not names:
        names = sorted(os.listdir(OFFICIAL))
    for name in names:
        if name in SKIP:
            print(f"{name}: skipped")
            continue
        print(f"{name}: {port(name, force)}")


if __name__ == "__main__":
    main()
