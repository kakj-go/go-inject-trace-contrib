#!/usr/bin/env python3
"""Run every otelc scenario sequentially and summarize PASS/FAIL."""
import os
import subprocess
import sys

HERE = os.path.dirname(__file__)
SCENARIOS = os.path.join(HERE, "..", "scenarios-otelc")

# otelsdk first: it gates the shared skeleton; then libraries in dependency order.
ORDER = [
    "otelsdk",
    "httpserver",
    "httpclient",
    "ginserver",
    "ginserverhttpclient",
    "logslibs",
    "dbclient",
    "grpcserver",
    "kafkago",
    "redisclient",
    "mongoclient",
    "mongoclientv2",
    "amqpclient",
    "elasticclient",
    "cassandraclient",
    "awsclient",
    "openaiclient",
    "openaiclientv2",
    "openaiclientv3",
    "anthropicclient",
    "linodegoclient",
    "k8sclient",
]


def main():
    only = sys.argv[1:] or ORDER
    results = {}
    for name in only:
        path = os.path.join(SCENARIOS, name)
        if not os.path.isdir(path):
            results[name] = "NO SUCH SCENARIO"
            continue
        print(f"\n########## {name}", flush=True)
        r = subprocess.run([sys.executable, "-u", os.path.join(HERE, "run_ab_otelc.py"),
                            "--scenario", path], capture_output=False)
        results[name] = "PASS" if r.returncode == 0 else "FAIL"
    print("\n===== summary =====")
    for name, res in results.items():
        print(f"{res:6} {name}")
    if any(v != "PASS" for v in results.values()):
        sys.exit(1)


if __name__ == "__main__":
    main()
