#!/usr/bin/env python3
"""A/B scenario runner: official skywalking-go agent vs go-inject + contrib.

For each (scenario x Go version x framework version) matrix cell:
  1. render a shared-source workspace from the scenario directory
  2. build+run it with the official go-agent   -> capture mock-collector data (A)
  3. build+run it with go-inject + contrib     -> capture mock-collector data (B)
  4. normalized-diff A vs B, and validate both against the scenario's
     excepted.yml through the mock-collector's /dataValidate endpoint

Everything (build, run, trigger, capture) happens in docker containers on a
private network; the official go-agent must run on Linux because its
module-version detection breaks on Windows module-cache paths.

Usage:
  python run_ab.py --scenario ../scenarios/gin [--go 1.26] [--framework v1.10.1]
                   [--only A|B] [--keep]
"""
import argparse
import json
import os
import shutil
import subprocess
import sys
import time
import uuid

import yaml

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
CONTRIB = os.environ.get("CONTRIB_DIR") or os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
GO_INJECT_REPO = os.path.abspath(os.path.join(ROOT, "..", "go-inject"))
SKYWALKING_REPO = os.path.abspath(os.path.join(ROOT, "..", "skywalking-go"))
RUNNER_BIN = os.path.join(os.path.dirname(__file__), "bin", "linux")
MOCK_IMAGE = "ghcr.io/apache/skywalking-agent-test-tool/mock-collector:b22b7d8ba62dabdd8db1ecc52da6178b063edff7"
CURL_IMAGE = "curlimages/curl:latest"
PROXY = os.environ.get("GOPROXY", "https://goproxy.cn,direct")
MOD_VOL = "abmod"
BUILD_VOL = {v: f"abcache{v.replace('.', '')}" for v in ("1.24", "1.25", "1.26", "1.27")}


def sh(*args, **kw):
    print("+", " ".join(args))
    return subprocess.run(args, check=True, capture_output=True, text=True, **kw)


def docker(*args, **kw):
    return sh("docker", *args, **kw)


def render_workspace(scenario, out, framework):
    os.makedirs(out, exist_ok=True)
    # copy the scenario tree (multi-app scenarios carry api/, client/server
    # sources and extra files the builds reference); skip metadata
    for root, dirs, files in os.walk(scenario):
        dirs[:] = [d for d in dirs if d != "__pycache__"]
        rel = os.path.relpath(root, scenario)
        for f in files:
            if f in ("plugin.yml", "excepted.yml", "go.mod.tpl"):
                continue
            src = os.path.join(root, f)
            dst = os.path.join(out, rel, f)
            os.makedirs(os.path.dirname(dst), exist_ok=True)
            with open(src, "rb") as r, open(dst, "wb") as w:
                w.write(r.read())
    with open(os.path.join(scenario, "go.mod.tpl"), encoding="utf-8") as f:
        gomod = f.read().replace("{{FRAMEWORK_VERSION}}", framework)
    with open(os.path.join(out, "go.mod"), "w", encoding="utf-8", newline="\n") as f:
        f.write(gomod)
    # seed go.sum from the official scenario to anchor transitive versions
    official_sum = os.path.join(scenario, "go.sum.official")
    if os.path.exists(official_sum):
        shutil.copy(official_sum, os.path.join(out, "go.sum"))


def ensure_tools():
    """Build the linux tools once with the newest toolchain image; golang
    images pin GOTOOLCHAIN=local, and the tool binaries are toolchain-
    independent, so a single build serves every target Go series."""
    go = "1.26"
    os.makedirs(RUNNER_BIN, exist_ok=True)
    if not os.path.exists(os.path.join(RUNNER_BIN, "go-inject")):
        docker("run", "--rm",
               "-v", f"{GO_INJECT_REPO}:/src", "-v", f"{RUNNER_BIN}:/out", "-v", f"{MOD_VOL}:/go/pkg/mod",
               "-e", f"GOPROXY={PROXY}", "-w", "/src", f"golang:{go}-bookworm",
               "go", "build", "-o", "/out/go-inject", "./cmd/go-inject")
        print("built linux go-inject")
    if not os.path.exists(os.path.join(RUNNER_BIN, "skywalking-go")):
        docker("run", "--rm",
               "-v", f"{SKYWALKING_REPO}:/src", "-v", f"{RUNNER_BIN}:/out", "-v", f"{MOD_VOL}:/go/pkg/mod",
               "-e", f"GOPROXY={PROXY}", "-w", "/src/tools/go-agent", f"golang:{go}-bookworm",
               "go", "build", "-o", "/out/skywalking-go", "./cmd")
        print("built linux skywalking-go (official agent)")


def start_mock(runid):
    docker("run", "-d", "--name", f"{runid}-mock", "--network", runid,
           "--network-alias", "oap", MOCK_IMAGE)
    wait_container_http(runid, "http://oap:12800/receiveData", timeout=120)


def stop_mock(runid):
    subprocess.run(["docker", "rm", "-f", f"{runid}-mock"], capture_output=True)
    time.sleep(1)


def start_deps(runid, deps):
    """Start scenario dependency services declared in plugin.yml."""
    remaining = dict(deps)
    started = []
    deadline = time.time() + 300
    while remaining and time.time() < deadline:
        for name, dep in list(remaining.items()):
            waits = [d for d in dep.get("depends_on", []) if d in remaining]
            if waits:
                continue
            args = ["run", "-d", "--name", f"{runid}-dep-{name}", "--network", runid,
                    "--network-alias", name]
            hostname = dep.get("hostname")
            if hostname and hostname != name:
                args += ["--network-alias", hostname]
            hc = dep.get("healthcheck") or {}
            if hc.get("test"):
                cmd = " ".join(hc["test"][1:]) if hc["test"][0] == "CMD" else hc["test"][-1]
                args += ["--health-cmd", cmd,
                         "--health-interval", hc.get("interval", "5s"),
                         "--health-retries", str(hc.get("retries", 60))]
            for k, v in (dep.get("environment") or {}).items():
                args += ["-e", f"{k}={v}"]
            args.append(dep["image"])
            # compose-style `command:` overrides the image CMD (words appended
            # after the image name); needed by images with no default server
            # CMD such as apachepulsar/pulsar
            args += [str(c) for c in (dep.get("command") or [])]
            docker(*args)
            started.append((name, bool(hc.get("test"))))
            del remaining[name]
        if remaining:
            time.sleep(2)
    for name, has_hc in started:
        if has_hc:
            wait_healthy(f"{runid}-dep-{name}", timeout=300)
    # extra settle time for DBs that report healthy before accepting
    # connections (mysql/postgres initialization race)
    if any(name in ("mysql", "postgres") for name, _ in started):
        time.sleep(10)
    if started and not any(h for _, h in started):
        time.sleep(15)


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
    """Wait until the app answers, failing fast with its logs if it exits."""
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


def run_side(side, runid, ws, go, cfg):
    tool = "/tools/skywalking-go" if side == "A" else "/tools/go-inject"
    extra = ["-a"]
    host = f"service:{cfg['port']}"
    # ---- build phase: codegen (if any) + all binaries, in one container ----
    build_cmds = []
    for step in (cfg.get("build_steps") or "").splitlines():
        build_cmds.append(step.replace("{TOOL}", tool).replace("{WS}", "/ws"))
    for target in (cfg.get("build_targets") or [{"out": "app", "pkg": "."}]):
        pkg = target["pkg"].removeprefix("/ws/").removeprefix("/")
        pkg = "." if pkg in ("", ".") else "/ws/" + pkg
        build_cmds.append(
            f"go build -mod=mod {' '.join(extra)} -toolexec {tool} -o /ws/{target['out']} {pkg}")
    env = {
        "GOPROXY": PROXY,
        "SW_AGENT_NAME": cfg["service"],
        "SW_AGENT_REPORTER_GRPC_BACKEND_SERVICE": "oap:19876",
        **cfg.get("env", {}),
    }
    build_args = ["run", "--rm", "--name", f"{runid}-build{side}", "--network", runid,
                  "-v", f"{ws}:/ws", "-v", f"{CONTRIB}:/contrib:ro", "-v", f"{RUNNER_BIN}:/tools:ro",
                  "-v", f"{MOD_VOL}:/go/pkg/mod", "-v", f"{BUILD_VOL[go]}:/root/.cache/go-build",
                  "-w", "/ws"]
    for k, v in env.items():
        build_args += ["-e", f"{k}={v}"]
    script = "cd /ws && go mod download && " + " && ".join(build_cmds)
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
                if k != "SW_AGENT_NAME":
                    args += ["-e", f"{k}={v}"]
            script = cfg["app_script"].replace("{WS}", "/ws")
            args += [f"golang:{go}-bookworm", "bash", "-c", script]
            docker(*args)
        else:
            for app in (cfg.get("apps") or [{"name": "app"}]):
                name = f"{runid}-{side.lower()}-{app['name']}"
                names.append(name)
                app_env = dict(env)
                app_env.update(app.get("env", {}))
                args = ["run", "-d", "--name", name, "--network", runid,
                        "--network-alias", app.get("alias", "service"),
                        "-v", f"{ws}:/ws:ro", "-v", f"{RUNNER_BIN}:/tools:ro", "-w", "/ws"]
                for k, v in app_env.items():
                    args += ["-e", f"{k}={v}"]
                args += [f"golang:{go}-bookworm", "/ws/" + app.get("bin", "app")]
                docker(*args)
        entry_name = f"{runid}-{side.lower()}-" + cfg.get("entry_app", "app")
        wait_app(runid, entry_name, f"http://service:{cfg['port']}{cfg['health']}", timeout=600, host=host)
        # trigger the traced entrypoint from inside the network so the Host header matches excepted.yml
        sh("docker", "run", "--rm", "--network", runid, CURL_IMAGE,
           "-H", f"Host: {host}", "-s", "-o", "/dev/null", f"http://service:{cfg['port']}{cfg['entry']}")
        time.sleep(3)
        actual = docker("run", "--rm", "--network", runid, CURL_IMAGE, "-s",
                        "http://oap:12800/receiveData").stdout
        out = os.path.join(ws, f"actual{side}.yaml")
        with open(out, "w", encoding="utf-8", newline="\n") as f:
            f.write(actual)
        v = subprocess.run(["docker", "run", "--rm", "--network", runid,
                            "-v", f"{cfg['scenario']}:/scn:ro", CURL_IMAGE, "-s", "-o", "/dev/null",
                            "-w", "%{http_code}", "-X", "POST",
                            "--data-binary", "@/scn/excepted.yml",
                            "http://oap:12800/dataValidate"], capture_output=True, text=True)
        print(f"[{side}] excepted.yml validate -> HTTP {v.stdout.strip()} {v.stderr.strip()}")
        if v.stdout.strip() != "200":
            raise RuntimeError(f"{side} failed excepted.yml validation")
        return out
    finally:
        for name in [f"{runid}-build{side}"] + [f"{runid}-{side.lower()}-" + a["name"] for a in (cfg.get("apps") or [{"name": "app"}])]:
            subprocess.run(["docker", "rm", "-f", name], capture_output=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--scenario", required=True)
    ap.add_argument("--go", choices=("1.24", "1.25", "1.26", "1.27"))
    ap.add_argument("--min-go", choices=("1.24", "1.25", "1.26"), default="1.24",
                    help="skip matrix cells below this Go series")
    ap.add_argument("--framework")
    ap.add_argument("--only", choices=("A", "B"))
    ap.add_argument("--keep", action="store_true")
    args = ap.parse_args()

    scenario = os.path.abspath(args.scenario)
    meta = yaml.safe_load(open(os.path.join(scenario, "plugin.yml"), encoding="utf-8"))
    expected = yaml.safe_load(open(os.path.join(scenario, "excepted.yml"), encoding="utf-8"))
    items = expected.get("segmentItems") or []
    cfg = {
        "scenario": scenario,
        "service": items[0]["serviceName"] if items else os.path.basename(scenario),
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
    }
    cells = []
    for row in meta["support-version"]:
        go = str(row["go"])
        for fw in row.get("framework") or [""]:
            if (args.go and go != args.go) or (args.framework and fw != args.framework):
                continue
            if float(go) < float(args.min_go):
                continue
            cells.append((go, fw))
    if not cells:
        sys.exit("no matrix cell selected")

    failures = []
    for go, fw in cells:
        runid = f"ab{uuid.uuid4().hex[:8]}"
        base_ws = os.path.join(os.path.dirname(__file__), "build",
                               f"{os.path.basename(scenario)}-{go}-{fw}")
        ensure_tools()
        print(f"===== {os.path.basename(scenario)} go{go} {fw} =====")
        docker("network", "create", runid)
        try:
            results = {}
            for side in ("A", "B"):
                if args.only and side != args.only:
                    continue
                # each side gets a fresh workspace: builds run with -mod=mod
                # and mutate go.mod, which would desync the two link steps.
                # deps are also recreated per side: persistent MQ topics would
                # otherwise redeliver side A's messages to side B's fresh
                # subscriptions (official CI runs each side in a fresh compose)
                ws = base_ws + side
                render_workspace(scenario, ws, fw)
                stop_deps(runid, cfg["deps"])
                start_deps(runid, cfg["deps"])
                start_mock(runid)
                results[side] = run_side(side, runid, ws, go, cfg)
                stop_mock(runid)
            if len(results) == 2:
                r = subprocess.run([sys.executable, os.path.join(os.path.dirname(__file__), "normdiff.py"),
                                    results["A"], results["B"]], text=True, capture_output=True)
                print(r.stdout, end="")
                if r.returncode != 0:
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
