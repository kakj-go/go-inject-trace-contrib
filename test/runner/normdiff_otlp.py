#!/usr/bin/env python3
"""Normalized structural diff of two mockcol OTLP JSON dumps (A vs B).

Philosophy mirrors normdiff.py (the skywalking variant): strip every volatile
field, keep the telemetry's semantic skeleton, then require exact structural
equality.

Traces   – spans regrouped by traceId across export batches and serialized as
            parent/child trees (name, kind, status, attrs, events, links,
            scope name); ids and timestamps are dropped because the tree shape
            encodes the parent relation.
Metrics  – per-scope instrument skeletons: name, unit, type, temporality,
            attribute key-sets per data point, explicit-bucket bounds. All
            numeric values are dropped (runtime metrics fluctuate by nature).
Logs     – body, severity, attributes (minus trace ids), and whether the
            record is trace-correlated (flag only, not the ids themselves).
Resource – attributes minus volatile detector output (process.*, host.*,
            container ids, service.instance.id, telemetry.distro.*).
"""
import json
import sys

VOLATILE_RESOURCE_KEYS = {
    "service.instance.id",
    "host.id", "host.name",
    # os.description embeds the container hostname and kernel build string,
    # which differ between the A and B containers.
    "os.description",
    "container.id", "container.runtime.name", "container.runtime.version",
}
VOLATILE_RESOURCE_PREFIXES = ("process.", "telemetry.distro.")

KIND_NAMES = {
    0: "unspecified", 1: "internal", 2: "server", 3: "client",
    4: "producer", 5: "consumer",
}


def attr_value(v):
    if not isinstance(v, dict) or not v:
        return None
    key, val = next(iter(v.items()))
    if key == "stringValue" or key == "intValue" or key == "doubleValue" or key == "boolValue" or key == "bytesValue":
        return val
    if key == "arrayValue":
        return [attr_value({"x": item}) for item in (val or {}).get("values", [])]
    if key == "kvlistValue":
        return canon_attrs((val or {}).get("values", []))
    return val


def canon_attrs(attrs):
    out = []
    for a in attrs or []:
        k = a.get("key")
        if k is None:
            continue
        v = attr_value(a.get("value"))
        # The ephemeral source port of a TCP connection (e.g. localhost
        # self-requests) differs between the A and B processes; keep the
        # attribute present but mask its value.
        if k in ("network.peer.port", "client.port"):
            v = "<ephemeral>"
        out.append((k, repr(v)))
    return sorted(out)


def canon_resource(resource):
    attrs = []
    for k, v in canon_attrs((resource or {}).get("attributes")):
        if k in VOLATILE_RESOURCE_KEYS or k.startswith(VOLATILE_RESOURCE_PREFIXES):
            continue
        attrs.append((k, v))
    return sorted(attrs)


def canon_span_tree(spans):
    """spans: list of raw span dicts sharing one traceId. Returns list of
    serialized root trees; children sorted canonically."""
    # The runner's readiness polls and entry trigger all hit /health; their
    # count varies between sides, so drop them from the comparison. They are
    # always root spans (bare health handlers), so dropping cannot orphan
    # children.
    spans = [s for s in spans
             if not any(a.get("key") == "url.path" and a.get("value", {}).get("stringValue") == "/health"
                        for a in s.get("attributes") or [])]
    by_id = {}
    children = {}
    roots = []
    for s in spans:
        sid = s.get("spanId")
        by_id[sid] = s
        children.setdefault(sid, [])
    for s in spans:
        pid = s.get("parentSpanId") or ""
        if pid and pid in by_id:
            children[pid].append(s)
        else:
            roots.append(s)

    def ser(s):
        kind = s.get("kind", 0)
        if isinstance(kind, str):
            kindname = kind.replace("SPAN_KIND_", "").lower()
        else:
            kindname = KIND_NAMES.get(kind, str(kind))
        node = {
            "name": s.get("name", ""),
            "kind": kindname,
            "scope": s.get("_scope", ""),
            "attrs": canon_attrs(s.get("attributes")),
            "events": sorted(
                ({"name": e.get("name", ""), "attrs": canon_attrs(e.get("attributes"))}
                 for e in s.get("events") or []),
                key=canon_key),
            "links": sorted(
                ({"attrs": canon_attrs(l.get("attributes"))}
                 for l in s.get("links") or []),
                key=canon_key),
            "children": [],
        }
        status = s.get("status") or {}
        node["status"] = {"code": status.get("code", 0),
                          "message": status.get("description", "")}
        for c in children[s.get("spanId")]:
            node["children"].append(ser(c))
        node["children"].sort(key=canon_key)
        return node

    trees = [ser(r) for r in roots]
    trees.sort(key=canon_key)
    return trees


def canon_key(obj):
    return json.dumps(obj, sort_keys=True, default=str)


def canon_traces(data):
    resource_spans = data.get("resourceSpans") or []
    # (traceId) -> {"resource": ..., "spans": [...]}; resource taken from the
    # first block that carried the span (one resource per process in practice).
    traces = {}
    for rs in resource_spans:
        res = canon_resource(rs.get("resource"))
        for ss in rs.get("scopeSpans") or []:
            scope = ((ss.get("scope") or {}).get("name")) or ""
            for s in ss.get("spans") or []:
                tid = s.get("traceId", "")
                slot = traces.setdefault(tid, {"resource": res, "spans": []})
                s = dict(s)
                s["_scope"] = scope
                slot["spans"].append(s)
    out = []
    for tid, slot in traces.items():
        out.append({"resource": slot["resource"],
                    "tree": canon_span_tree(slot["spans"])})
    out.sort(key=canon_key)
    return out


def canon_metric_points(metric):
    points = []
    for typ in ("gauge", "sum", "histogram", "expohistogram", "summary"):
        body = metric.get(typ)
        if not body:
            continue
        m = {"type": typ}
        if typ in ("sum",):
            m["temporality"] = body.get("aggregationTemporality")
            m["monotonic"] = body.get("isMonotonic")
        if typ in ("histogram",):
            m["temporality"] = body.get("aggregationTemporality")
        for dp in body.get("dataPoints") or []:
            p = {"attrs": canon_attrs(dp.get("attributes"))}
            if typ == "histogram":
                p["bucketBounds"] = dp.get("explicitBounds") or []
            points.append(p)
        points.sort(key=canon_key)
        m["points"] = points
        return m
    return {"type": "unknown", "points": []}


def canon_metrics(data):
    out = []
    for rm in data.get("resourceMetrics") or []:
        res = canon_resource(rm.get("resource"))
        for sm in rm.get("scopeMetrics") or []:
            scope = ((sm.get("scope") or {}).get("name")) or ""
            for m in sm.get("metrics") or []:
                out.append({
                    "resource": res,
                    "scope": scope,
                    "name": m.get("name", ""),
                    "unit": m.get("unit", ""),
                    "metric": canon_metric_points(m),
                })
    out.sort(key=canon_key)
    return out


def canon_logs(data):
    out = []
    for rl in data.get("resourceLogs") or []:
        res = canon_resource(rl.get("resource"))
        for sl in rl.get("scopeLogs") or []:
            scope = ((sl.get("scope") or {}).get("name")) or ""
            for rec in sl.get("logRecords") or []:
                attrs = [(k, v) for k, v in canon_attrs(rec.get("attributes"))
                         if k not in ("trace_id", "span_id", "traceID", "spanID")]
                out.append({
                    "resource": res,
                    "scope": scope,
                    "severity": rec.get("severityNumber", 0),
                    "severityText": rec.get("severityText", ""),
                    "body": repr(attr_value(rec.get("body"))),
                    "attrs": sorted(attrs),
                    "traceCorrelated": bool(rec.get("traceId")),
                })
    out.sort(key=canon_key)
    return out


def normalize(path):
    with open(path, encoding="utf-8") as f:
        data = json.load(f)
    if not isinstance(data, dict):  # empty mockcol dump "{}"
        data = {}
    return {
        "traces": canon_traces(data),
        "metrics": canon_metrics(data),
        "logs": canon_logs(data),
    }


def diff(name, a, b):
    ka = {canon_key(x) for x in a}
    kb = {canon_key(x) for x in b}
    only_a = [json.loads(k) for k in sorted(ka - kb)]
    only_b = [json.loads(k) for k in sorted(kb - ka)]
    if not only_a and not only_b:
        return True
    print(f"{name}: MISMATCH")
    for x in only_a:
        print(f"  only in A: {json.dumps(x, ensure_ascii=False)[:400]}")
    for x in only_b:
        print(f"  only in B: {json.dumps(x, ensure_ascii=False)[:400]}")
    return False


def main():
    if len(sys.argv) != 3:
        sys.exit("usage: normdiff_otlp.py <actualA.json> <actualB.json>")
    a, b = normalize(sys.argv[1]), normalize(sys.argv[2])
    ok = all(diff(sig, a[sig], b[sig]) for sig in ("traces", "metrics", "logs"))
    if ok:
        ntr = len(a["traces"])
        nme = len(a["metrics"])
        nlo = len(a["logs"])
        print(f"MATCH ({ntr} traces, {nme} metric instruments, {nlo} log records)")
    else:
        sys.exit(1)


if __name__ == "__main__":
    main()
