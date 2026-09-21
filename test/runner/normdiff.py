#!/usr/bin/env python3
"""Normalized A/B diff for skywalking mock-collector receiveData YAML.

Strips volatile fields (ids, timestamps, instance names) and compares the
structural trace data: span trees, operation names, tags, refs, meters.
Uses subset comparison for segments: every segment in A must exist in B,
but B may have extra segments from timing artifacts (stale MQ consumption).
"""
import re
import sys
import yaml


def normalize_tag_value(value):
    """Strip volatile query-string parameters that differ between runs."""
    if not isinstance(value, str):
        return value
    # skywalking global ids (trace/segment ids captured by toolkit Get*ID):
    # uuid.epoch.counter — instance-specific by construction, both sides are
    # validated semantically by excepted.yml ("not null")
    if re.fullmatch(r"[0-9a-f]{32}\.\d+\.\d+", value):
        return "<sky-id>"
    # mq.args tags carry the full sw8 propagation header (base64 blobs with
    # embedded instance/segment ids) — structure matters, content does not
    if "sw8" in value:
        return "<mq-args-with-sw8>"
    if "=" not in value:
        return value
    return re.sub(r"[?&](timestamp|pid|port|t|ts|_)=\d+", "", value)


def normalize(data):
    out = {"segments": [], "meters": [], "logs": []}
    for item in data.get("segmentItems") or []:
        for seg in item.get("segments") or []:
            spans = []
            for sp in seg.get("spans") or []:
                refs = sorted(
                    (r.get("refType"), r.get("parentEndpoint"), r.get("networkAddress"))
                    for r in (sp.get("refs") or [])
                )
                tags = sorted((t.get("key"), normalize_tag_value(t.get("value"))) for t in (sp.get("tags") or []))
                spans.append({
                    "spanId": sp.get("spanId"),
                    "parentSpanId": sp.get("parentSpanId"),
                    "spanType": sp.get("spanType"),
                    "spanLayer": sp.get("spanLayer"),
                    "operationName": sp.get("operationName"),
                    "componentId": sp.get("componentId"),
                    "isError": sp.get("isError"),
                    "peer": sp.get("peer"),
                    "tags": tags,
                    "refs": refs,
                })
            if not spans:
                continue
            spans.sort(key=lambda s: (s["spanId"], s["operationName"]))
            out["segments"].append({"service": item.get("serviceName"), "spans": spans})
    out["segments"].sort(key=lambda s: (s["service"], str(s["spans"])))
    for m in data.get("meterItems") or []:
        out["meters"].append({"name": m.get("meterName") or m.get("name"), "labels": m.get("labels")})
    return out


def main():
    a = normalize(yaml.safe_load(open(sys.argv[1], encoding="utf-8")))
    b = normalize(yaml.safe_load(open(sys.argv[2], encoding="utf-8")))
    ok = True

    for label in ("segments", "meters", "logs"):
        if label == "segments":
            # subset comparison: every A segment must exist in B;
            # B may have extra segments from timing artifacts
            b_pool = list(b[label])
            missing = []
            for seg in a[label]:
                if seg in b_pool:
                    b_pool.remove(seg)
                else:
                    missing.append(seg)
            if missing:
                ok = False
                print(f"== DIFF in {label}: {len(missing)} A-segments missing from B ==")
                for x in missing:
                    print("  missing in B:", x)
            else:
                extras = len(b_pool)
                print(f"{label}: match (A={len(a[label])}, B={len(b[label])}, B-extras={extras})")
        else:
            if a[label] != b[label]:
                ok = False
                print(f"== DIFF in {label} ==")
                only_a = [x for x in a[label] if x not in b[label]]
                only_b = [x for x in b[label] if x not in a[label]]
                for x in only_a:
                    print("  only in A:", x)
                for x in only_b:
                    print("  only in B:", x)
            else:
                print(f"{label}: identical ({len(a[label])} items)")

    print("RESULT:", "MATCH" if ok else "MISMATCH")
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main()
