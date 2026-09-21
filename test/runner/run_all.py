#!/usr/bin/env python3
"""Run the A/B matrix for every ported scenario sequentially, logging results."""
import os
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
SCENARIOS = os.path.abspath(os.path.join(HERE, "..", "scenarios"))
ORDER = [
    # ready and previously verified first
    "gin", "http", "mux", "fasthttp", "fiber", "irisv12", "go-restfulv3", "echov4", "goframe",
    "segmentio-kafka", "rocketmq", "pulsar", "amqp", "grpc", "dubbo", "microv4", "kratosv2",
    "mysql", "postgres", "gorm", "gorm-postgres", "go-redisv9", "mongo", "go-elasticsearchv8",
    "trace-activation", "metric-activation", "logging-activation",
    "runtime_metrics", "cross-goroutine", "so11y", "discard-reporter", "short_versions_gin", "pprof",
]


def main():
    only = sys.argv[1:]
    names = [n for n in ORDER if n in only] if only else ORDER
    results = {}
    for name in names:
        d = os.path.join(SCENARIOS, name)
        if not os.path.isdir(d):
            results[name] = "missing"
            continue
        print(f"\n########## {name} ##########", flush=True)
        r = subprocess.run([sys.executable, os.path.join(HERE, "run_ab.py"), "--scenario", d, "--min-go", "1.25"],
                           capture_output=True, text=True)
        tail = "\n".join((r.stdout + r.stderr).strip().splitlines()[-6:])
        ok = r.returncode == 0
        results[name] = "PASS" if ok else "FAIL"
        print(tail, flush=True)
    print("\n===== SUMMARY =====")
    for k, v in results.items():
        print(f"{v:6} {k}")
    failed = [k for k, v in results.items() if v != "PASS"]
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
