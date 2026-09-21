#!/usr/bin/env python3
"""Normalized diff of two app stdout logs (A vs B) for log-instrumentation
scenarios: the bridges inject trace_id/span_id into library output, so the
runner captures `docker logs` per side and this script compares them after
masking volatile tokens (timestamps, levels' time fields, trace/span ids,
ephemeral ports, goroutine ids)."""
import re
import sys

HEX32 = re.compile(r"\b[0-9a-f]{32}\b")
HEX16 = re.compile(r"\b[0-9a-f]{16}\b")
# slog text handler: time=2026-.. level=INFO msg=... ; logrus: time="..."
# level=info msg=... ; zap ISO8601: 2026-09-16T00:00:00.000Z
TIME_TOKENS = [
    re.compile(r"\btime=\"?[0-9]{4}-[0-9]{2}-[0-9]{2}[T ][0-9:.]+(Z|[+-][0-9:]+)?\"?"),
    re.compile(r"\b[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z"),
    re.compile(r"\bts=[0-9.]+"),
    re.compile(r"\b[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?"),
]
# zap caller pc and pid
PID = re.compile(r"\bpid[=:]\s*\d+")
PC = re.compile(r"\bpc[=:]\s*\d+")
PORT = re.compile(r":\d{4,5}\b")


def normalize(line):
    for t in TIME_TOKENS:
        line = t.sub("<time>", line)
    line = HEX32.sub("<traceid>", line)
    line = HEX16.sub("<spanid>", line)
    line = PID.sub("<pid>", line)
    line = PC.sub("<pc>", line)
    line = PORT.sub(":<port>", line)
    return line.strip()


def main():
    if len(sys.argv) != 3:
        sys.exit("usage: normdiff_log.py <stdoutA.log> <stdoutB.log>")
    out = {}
    for path, side in ((sys.argv[1], "A"), (sys.argv[2], "B")):
        with open(path, encoding="utf-8", errors="replace") as f:
            lines = sorted(normalize(l) for l in f if l.strip())
        out[side] = lines
    if out["A"] == out["B"]:
        print(f"STDOUT MATCH ({len(out['A'])} lines)")
        return
    sa, sb = set(out["A"]), set(out["B"])
    print("stdout: MISMATCH")
    for l in sorted(sa - sb):
        print(f"  only in A: {l[:300]}")
    for l in sorted(sb - sa):
        print(f"  only in B: {l[:300]}")
    sys.exit(1)


if __name__ == "__main__":
    main()
