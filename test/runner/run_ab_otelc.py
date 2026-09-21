#!/usr/bin/env python3
"""A/B scenario runner for the otelc port: official otelc vs go-inject + contrib.

For each (scenario x Go version) matrix cell:
  1. render a per-side workspace from the scenario directory
  2. side A: `otelc go build` (official compile-time instrumentation, built
     from the local clone of opentelemetry-go-compile-instrumentation)
  3. side B: `go build -toolexec=go-inject` with the otelc rule tree in this
     repository
  4. both sides export OTLP to mockcol; the normalized dumps must match
     (normdiff_otlp.py). There is no excepted.yml validation step — side A,
     the official implementation, is the reference.

Usage:
  python run_ab_otelc.py --scenario ../scenarios-otelc/otelsdk [--go 1.26] [--only A|B] [--keep]
"""
import argparse
import os
import subprocess
import sys
import time
import uuid

import yaml

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
CONTRIB = ROOT
GO_INJECT_REPO = os.path.abspath(os.path.join(ROOT, "..", "go-inject"))
OTELC_REPO = os.path.abspath(os.path.join(ROOT, "..", "opentelemetry-go-compile-instrumentation"))
RUNNER_BIN = os.path.join(os.path.dirname(__file__), "bin", "linux")
MOCKCOL_SRC = os.path.join(os.path.dirname(__file__), "..", "mockcol")
CURL_IMAGE = "curlimages/curl:latest"
PROXY = os.environ.get("GOPROXY", "https://goproxy.cn,direct")
MOD_VOL = "abmod"
BUILD_VOL = {v: f"abcache{v.replace('.', '')}" for v in ("1.24", "1.25", "1.26", "1.27")}


def sh(*args, **kw):
    print("+", " ".join(args))
    return subprocess.run(args, check=True, capture_output=True, text=True, **kw)


def docker(*args, **kw):
    return sh("docker", *args, **kw)


# The aggregated otelc rule package imports every instrumented SDK under the
# goinject tag, so go-inject's readonly dependency scan needs them all
# resolvable even when the scenario itself only uses one library. Render
# appends the requires that the scenario's go.mod doesn't already carry.
OTELC_SDK_REQUIRES = [
    "github.com/rabbitmq/amqp091-go v1.15.0",
    "github.com/olivere/elastic/v7 v7.0.32",
    "github.com/apache/cassandra-gocql-driver/v2 v2.0.0",
    "github.com/aws/aws-sdk-go-v2/config v1.29.14",
    "github.com/aws/aws-sdk-go-v2 v1.44.0",
    "go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws v0.71.0",
    "k8s.io/client-go v0.35.0",
    "github.com/openai/openai-go v1.12.0",
    "github.com/openai/openai-go/v2 v2.7.1",
    "github.com/openai/openai-go/v3 v3.53.0",
    "github.com/anthropics/anthropic-sdk-go v1.67.0",
    "github.com/linode/linodego/v2 v2.5.0",
    "github.com/segmentio/kafka-go v0.4.51",
    "github.com/redis/go-redis/v9 v9.22.0",
    "go.mongodb.org/mongo-driver v1.17.9",
    "go.mongodb.org/mongo-driver/v2 v2.8.1",
    "go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo v0.71.0",
    "go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo v0.0.0-20260814181354-b0f078590c22",
    "google.golang.org/grpc v1.71.0",
    "github.com/gin-gonic/gin v1.12.0",
    "github.com/sirupsen/logrus v1.9.4",
    "go.uber.org/zap v1.21.0",
]


def append_sdk_requires(gomod):
    missing = [r for r in OTELC_SDK_REQUIRES if r.split()[0] not in gomod]
    if not missing:
        return gomod
    block = "\nrequire (\n" + "\n".join(f"\t{r} // indirect" for r in missing) + "\n)\n"
    idx = gomod.index("replace github.com/kakj-go/go-inject-trace-contrib")
    return gomod[:idx] + block + "\n" + gomod[idx:]


def render_workspace(scenario, out, framework=""):
    os.makedirs(out, exist_ok=True)
    for name in ("main.go", "inject.go"):
        with open(os.path.join(scenario, name), encoding="utf-8") as f:
            data = f.read()
        with open(os.path.join(out, name), "w", encoding="utf-8", newline="\n") as f:
            f.write(data)
    with open(os.path.join(scenario, "go.mod.tpl"), encoding="utf-8") as f:
        gomod = f.read()
    gomod = gomod.replace("{{FRAMEWORK_VERSION}}", framework)
    gomod = append_sdk_requires(gomod)
    with open(os.path.join(out, "go.mod"), "w", encoding="utf-8", newline="\n") as f:
        f.write(gomod)


def ensure_tools():
    """Build the linux tools once with the newest toolchain image."""
    go = "1.26"
    os.makedirs(RUNNER_BIN, exist_ok=True)
    if not os.path.exists(os.path.join(RUNNER_BIN, "go-inject")):
        docker("run", "--rm",
               "-v", f"{GO_INJECT_REPO}:/src", "-v", f"{RUNNER_BIN}:/out", "-v", f"{MOD_VOL}:/go/pkg/mod",
               "-e", f"GOPROXY={PROXY}", "-w", "/src", f"golang:{go}-bookworm",
               "go", "build", "-o", "/out/go-inject", "./cmd/go-inject")
        print("built linux go-inject")
    if not os.path.exists(os.path.join(RUNNER_BIN, "otelc")):
        docker("run", "--rm",
               "-v", f"{OTELC_REPO}:/src", "-v", f"{RUNNER_BIN}:/out", "-v", f"{MOD_VOL}:/go/pkg/mod",
               "-e", f"GOPROXY={PROXY}", "-w", "/src", f"golang:{go}-bookworm",
               "go", "build", "-o", "/out/otelc", "./tool/cmd/otelc")
        print("built linux otelc (official, from local clone)")
    if not os.path.exists(os.path.join(RUNNER_BIN, "mockcol")):
        docker("run", "--rm",
               "-v", f"{MOCKCOL_SRC}:/src", "-v", f"{RUNNER_BIN}:/out", "-v", f"{MOD_VOL}:/go/pkg/mod",
               "-e", f"GOPROXY={PROXY}", "-w", "/src", f"golang:{go}-bookworm",
               "go", "build", "-o", "/out/mockcol", ".")
        print("built linux mockcol")


def start_mock(runid):
    docker("run", "-d", "--name", f"{runid}-mock", "--network", runid,
           "--network-alias", "oap",
           "-v", f"{RUNNER_BIN}:/tools:ro", "golang:1.26-bookworm", "/tools/mockcol")
    wait_container_http(runid, "http://oap:4318/receiveData", timeout=120)


def stop_mock(runid):
    subprocess.run(["docker", "rm", "-f", f"{runid}-mock"], capture_output=True)
    time.sleep(1)


def start_deps(runid, deps):
    remaining = dict(deps)
    started = []
    deadline = time.time() + 300
    while remaining and time.time() < deadline:
        for name, dep in list(remaining.items()):
            waits = [d for d in dep.get("depends_on", []) if d in remaining]
            if waits:
                continue
            args = ["run", "-d", "--name", f"{runid}-dep-{name}", "--network", runid,
                    "--network-alias", dep.get("hostname", name)]
            if dep.get("privileged"):
                args.append("--privileged")
            hc = dep.get("healthcheck") or {}
            if hc.get("test"):
                cmd = " ".join(hc["test"][1:]) if hc["test"][0] == "CMD" else hc["test"][-1]
                args += ["--health-cmd", cmd,
                         "--health-interval", hc.get("interval", "5s"),
                         "--health-retries", str(hc.get("retries", 60))]
            for k, v in (dep.get("environment") or {}).items():
                args += ["-e", f"{k}={v}"]
            args.append(dep["image"])
            if dep.get("command"):
                args.extend(dep["command"].split())
            docker(*args)
            started.append((name, bool(hc.get("test"))))
            del remaining[name]
        if remaining:
            time.sleep(2)
    for name, has_hc in started:
        if has_hc:
            wait_healthy(f"{runid}-dep-{name}", timeout=600)
    if started and not any(h for _, h in started):
        time.sleep(15)
    # Post-health setup: extract kubeconfig from a k3s dependency and import
    # the pause image so pods with PullNever can start.
    for name, dep in deps.items():
        if dep.get("kubeconfig_from") == name or (name == "k3s-server" and dep.get("image","").startswith("rancher/k3s")):
            kubeconfig = extract_k3s_kubeconfig(f"{runid}-dep-{name}", dep.get("hostname", name))
            if kubeconfig:
                os.environ["OTELC_KUBECONFIG"] = kubeconfig
            load_k3s_image(f"{runid}-dep-{name}", "registry.k8s.io/pause")


def extract_k3s_kubeconfig(container, hostname):
    """Extract the k3s kubeconfig and rewrite the server URL to the docker
    network hostname so the app container can reach the API server."""
    r = subprocess.run(["docker", "exec", container, "sh", "-c", "cat /etc/rancher/k3s/k3s.yaml"],
                       capture_output=True, text=True)
    if r.returncode != 0:
        print(f"kubeconfig extract failed: {r.stderr[:200]}")
        return None
    return r.stdout.replace("https://127.0.0.1:6443", f"https://{hostname}:6443")


def load_k3s_image(container, image):
    """Pipe a docker image into k3s's containerd so PullNever pods start."""
    r = subprocess.run(f"docker save {image} | docker exec -i {container} sh -c 'k3s ctr images import -'",
                       shell=True, capture_output=True, text=True)
    if r.returncode != 0:
        print(f"k3s image load failed (non-fatal): {r.stderr[:200]}")


def stop_deps(runid, deps):
    for name in deps:
        subprocess.run(["docker", "rm", "-f", f"{runid}-dep-{name}"], capture_output=True)


def wait_healthy(container, timeout):
    deadline = time.time() + timeout
    while time.time() < deadline:
        out = subprocess.run(["docker", "inspect", "-f", "{{.State.Health.Status}}", container],
                             capture_output=True, text=True).stdout.strip()
        if out == "healthy":
            return
        time.sleep(3)
    raise TimeoutError(f"{container} not healthy after {timeout}s")


def wait_container_http(runid, url, timeout):
    deadline = time.time() + timeout
    while time.time() < deadline:
        r = subprocess.run(["docker", "run", "--rm", "--network", runid, CURL_IMAGE,
                            "-s", "-o", "/dev/null", "-w", "%{http_code}", url],
                           capture_output=True, text=True)
        if r.stdout.strip() in ("200", "404"):
            return
        time.sleep(2)
    raise TimeoutError(f"endpoint not ready: {url}")


def wait_app(runid, app, url, timeout, host=None):
    curl = ["docker", "run", "--rm", "--network", runid, CURL_IMAGE]
    if host:
        curl += ["-H", f"Host: {host}"]
    curl += ["-s", "-o", "/dev/null", "-w", "%{http_code}", url]
    deadline = time.time() + timeout
    last = ""
    while time.time() < deadline:
        r = subprocess.run(curl, capture_output=True, text=True)
        last = f"{r.stdout.strip()} {r.stderr.strip()}"
        if r.stdout.strip() in ("200", "404"):
            return
        state = subprocess.run(["docker", "inspect", "-f", "{{.State.Running}}", app],
                               capture_output=True, text=True).stdout.strip()
        if state != "true":
            logs = subprocess.run(["docker", "logs", "--tail", "30", app],
                                  capture_output=True, text=True).stdout
            raise RuntimeError(f"{app} exited during build/startup:\n{logs}")
        time.sleep(3)
    raise TimeoutError(f"health check failed for {url}: {last}")


def otel_env(cfg):
    return {
        "OTEL_SERVICE_NAME": cfg["service"],
        "OTEL_EXPORTER_OTLP_ENDPOINT": "http://oap:4318",
        "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
        "OTEL_TRACES_EXPORTER": "otlp",
        "OTEL_METRICS_EXPORTER": "otlp",
        "OTEL_LOGS_EXPORTER": "otlp",
        "OTEL_GO_SIMPLE_SPAN_PROCESSOR": "true",
        "OTEL_METRIC_EXPORT_INTERVAL": "1000",
    }


def run_side(side, runid, ws, go, cfg):
    host = f"service:{cfg['port']}"
    # ---- build phase: one container per side, own workspace copy ----
    if side == "A":
        build_cmds = [
            "/tools/otelc go build -mod=mod -o /ws/app .",
        ]
    else:
        build_cmds = [
            "go build -mod=mod -toolexec /tools/go-inject -o /ws/app .",
        ]
    for step in (cfg.get("build_steps") or "").splitlines():
        build_cmds.append(step.replace("{TOOL}", "/tools/go-inject" if side == "B" else "/tools/otelc"))
    for target in (cfg.get("build_targets") or []):
        if side == "A":
            build_cmds.append(f"/tools/otelc go build -mod=mod -o /ws/{target['out']} /ws/{target['pkg']}")
        else:
            build_cmds.append(f"go build -mod=mod -toolexec /tools/go-inject -o /ws/{target['out']} /ws/{target['pkg']}")

    env = {"GOPROXY": PROXY, "GOFLAGS": "-mod=mod", **otel_env(cfg), **cfg.get("env", {})}
    if os.environ.get("OTELC_KUBECONFIG"):
        env["KUBECONFIG_YAML"] = os.environ["OTELC_KUBECONFIG"]
    build_args = ["run", "--rm", "--name", f"{runid}-build{side}", "--network", runid,
                  "-v", f"{ws}:/ws", "-v", f"{CONTRIB}:/contrib", "-v", f"{RUNNER_BIN}:/tools:ro",
                  "-v", f"{MOD_VOL}:/go/pkg/mod", "-v", f"{BUILD_VOL[go]}:/root/.cache/go-build",
                  "-w", "/ws"]
    for k, v in env.items():
        build_args += ["-e", f"{k}={v}"]
    # tidy is best-effort: the aggregated rule packages reference every
    # instrumented SDK under the goinject tag, so scenarios that exercise one
    # library lack requires for the others. GOFLAGS=-mod=mod lets the build
    # itself add whatever is actually needed for THIS scenario's graph.
    script = "cd /ws && (go mod tidy || true) && " + " && ".join(build_cmds)
    build_args += [f"golang:{go}-bookworm", "bash", "-c", script]
    try:
        r = subprocess.run(["docker", *build_args], capture_output=True, text=True)
        if r.returncode != 0:
            raise RuntimeError(f"build {side} failed:\n{r.stdout[-3000:]}\n{r.stderr[-3000:]}")
        # ---- run phase ----
        names = []
        if cfg.get("app_script"):
            name = f"{runid}-{side.lower()}-app"
            names.append(name)
            args = ["run", "-d", "--name", name, "--network", runid, "--network-alias", "service",
                    "-v", f"{ws}:/ws:ro", "-v", f"{RUNNER_BIN}:/tools:ro", "-w", "/ws"]
            for k, v in env.items():
                if k != "OTEL_SERVICE_NAME":
                    args += ["-e", f"{k}={v}"]
            script = cfg["app_script"].replace("{WS}", "/ws")
            args += [f"golang:{go}-bookworm", "bash", "-c", script]
            docker(*args)
        else:
            for app in (cfg.get("apps") or [{"name": "app"}]):
                name = f"{runid}-{side.lower()}-{app['name']}"
                names.append(name)
                app_env = {k: v for k, v in env.items() if k not in ("GOPROXY", "GOFLAGS")}
                app_env.update(app.get("env", {}))
                args = ["run", "-d", "--name", name, "--network", runid,
                        "--network-alias", app.get("alias", "service"),
                        "-v", f"{ws}:/ws:ro", "-w", "/ws"]
                for k, v in app_env.items():
                    args += ["-e", f"{k}={v}"]
                args += [f"golang:{go}-bookworm", "/ws/" + app.get("bin", "app")]
                docker(*args)
        entry_name = f"{runid}-{side.lower()}-" + cfg.get("entry_app", "app")
        wait_app(runid, entry_name, f"http://service:{cfg['port']}{cfg['health']}", timeout=600, host=host)
        # trigger the entrypoint from inside the network (Host kept stable so
        # any url/server.address attributes stay deterministic)
        sh("docker", "run", "--rm", "--network", runid, CURL_IMAGE,
           "-H", f"Host: {host}", "-s", "-o", "/dev/null", f"http://service:{cfg['port']}{cfg['entry']}")
        # metrics flush on a 1s interval; logs batch ~1s; give both a margin
        time.sleep(6)
        actual = docker("run", "--rm", "--network", runid, CURL_IMAGE, "-s",
                        "http://oap:4318/receiveData").stdout
        out = os.path.join(ws, f"actual{side}.json")
        with open(out, "w", encoding="utf-8", newline="\n") as f:
            f.write(actual)
        # Log-instrumentation scenarios verify trace_id injection by diffing
        # the app's stdout (normalized) — the log bridges write to library
        # output, not OTLP.
        stdout_out = None
        if cfg.get("compare_stdout"):
            entry_name = f"{runid}-{side.lower()}-" + cfg.get("entry_app", "app")
            logs = subprocess.run(["docker", "logs", entry_name], capture_output=True, text=True).stdout
            stdout_out = os.path.join(ws, f"stdout{side}.log")
            with open(stdout_out, "w", encoding="utf-8", newline="\n") as f:
                f.write(logs)
        return out if stdout_out is None else (out, stdout_out)
    finally:
        for name in [f"{runid}-build{side}"] + [f"{runid}-{side.lower()}-" + a["name"] for a in (cfg.get("apps") or [{"name": "app"}])]:
            subprocess.run(["docker", "rm", "-f", name], capture_output=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--scenario", required=True)
    ap.add_argument("--go", choices=("1.25", "1.26", "1.27"))
    ap.add_argument("--framework")
    ap.add_argument("--only", choices=("A", "B"))
    ap.add_argument("--keep", action="store_true")
    args = ap.parse_args()

    scenario = os.path.abspath(args.scenario)
    meta = yaml.safe_load(open(os.path.join(scenario, "plugin.yml"), encoding="utf-8"))
    cfg = {
        "scenario": scenario,
        "service": meta.get("service") or os.path.basename(scenario),
        "entry": meta["entry"],
        "health": meta.get("health", "/health"),
        "port": str(meta.get("port", 8080)),
        "env": meta.get("env") or {},
        "deps": meta.get("dependencies") or {},
        "build_steps": meta.get("build_steps", ""),
        "build_targets": meta.get("build_targets"),
        "apps": meta.get("apps"),
        "app_script": meta.get("app_script", ""),
        "entry_app": meta.get("entry_app", "app"),
        "compare_stdout": meta.get("compare_stdout", False),
    }
    cells = []
    for row in meta["support-version"]:
        go = str(row["go"])
        for fw in row.get("framework") or [""]:
            if (args.go and go != args.go) or (args.framework and fw != args.framework):
                continue
            cells.append((go, fw))
    if not cells:
        sys.exit("no matrix cell selected")

    failures = []
    for go, fw in cells:
        runid = f"otelab{uuid.uuid4().hex[:8]}"
        print(f"===== {os.path.basename(scenario)} go{go} {fw} =====")
        ensure_tools()
        docker("network", "create", runid)
        try:
            results = {}
            for side in ("A", "B"):
                if args.only and side != args.only:
                    continue
                # per-side workspace and dependencies: otelc's setup mutates
                # go.mod, and stateful brokers (kafka offsets, consumer group
                # positions) must start virgin on each side or the second
                # side's spans carry the first side's offsets.
                start_deps(runid, cfg["deps"])
                # per-side workspace: otelc's setup mutates go.mod and drops
                # generated files that must not leak into the other side's build
                ws = os.path.join(os.path.dirname(__file__), "build",
                                  f"{os.path.basename(scenario)}-{go}-{fw}-{side}")
                render_workspace(scenario, ws, fw)
                start_mock(runid)
                try:
                    results[side] = run_side(side, runid, ws, go, cfg)
                finally:
                    stop_mock(runid)
                    if not args.keep:
                        stop_deps(runid, cfg["deps"])
            if len(results) == 2:
                norm_args = [sys.executable, os.path.join(os.path.dirname(__file__), "normdiff_otlp.py"),
                             results["A"][0] if isinstance(results["A"], tuple) else results["A"],
                             results["B"][0] if isinstance(results["B"], tuple) else results["B"]]
                r = subprocess.run(norm_args, text=True, capture_output=True)
                print(r.stdout, end="")
                if r.returncode != 0:
                    failures.append((go, fw))
                    continue
                if cfg.get("compare_stdout"):
                    r2 = subprocess.run([sys.executable, os.path.join(os.path.dirname(__file__), "normdiff_log.py"),
                                         results["A"][1], results["B"][1]], text=True, capture_output=True)
                    print(r2.stdout, end="")
                    if r2.returncode != 0:
                        failures.append((go, fw))
        except Exception as e:
            import traceback
            traceback.print_exc()
            print(f"CELL FAILED: {e}", file=sys.stderr)
            failures.append((go, fw, str(e)[:200]))
        finally:
            if not args.keep:
                stop_deps(runid, cfg["deps"])
                subprocess.run(["docker", "rm", "-f", f"{runid}-mock", f"{runid}-appA", f"{runid}-appB"],
                               capture_output=True)
                subprocess.run(["docker", "network", "rm", runid], capture_output=True)
    if failures:
        sys.exit(f"FAILED cells: {failures}")
    print("ALL CELLS MATCH")


if __name__ == "__main__":
    main()
