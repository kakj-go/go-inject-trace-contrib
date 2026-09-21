#!/usr/bin/env python3
"""Cross-backend trace comparison for the compare demo.

Input:
  --sky   dump from the SkyWalking mock-collector (GET :12800/receiveData, YAML)
  --otel  dump from the OTLP mockcol          (GET :4318/receiveData, JSON)

The two backends name and shape spans by different conventions, so a raw diff
is meaningless. This script normalizes every span to a semantic *role*
(http.server GET /api/order, db INSERT, redis set, grpc.client api.Inventory/Reserve,
...) and then compares:

  1. trace inventory    - how many traces each side saw, by root role
  2. per-trace topology - side-by-side span trees of the first business trace
  3. role multisets     - span counts per role across all business traces,
                          classified MATCH / SKY-ONLY / OTEL-ONLY
  4. attributes         - tag-level spot comparison on representative spans

SkyWalking's grpc plugin emits extra Local "detail" spans (SendMsg/RecvMsg)
that OTel folds into the rpc span itself; they are counted separately as a
known convention difference instead of a mismatch.
"""
import argparse
import json
import sys
from collections import Counter, defaultdict

import yaml

# ---------------------------------------------------------------- sky side --


def canon_sky(n):
    t, layer, name = n["type"], n["layer"], n["name"]
    if t == "Entry" and layer == "Http":
        method, _, path = name.partition(":")
        return f"http.server {method} {path}"
    if t == "Exit" and layer == "Http":
        method, _, path = name.partition(":")
        return f"http.client {method} {path}"
    if t == "Entry" and layer == "RPCFramework":
        return "grpc.server " + name.rsplit(".", 1)[0] + "/" + name.rsplit(".", 1)[-1]
    if t == "Exit" and layer == "RPCFramework":
        return "grpc.client " + name.rsplit(".", 1)[0] + "/" + name.rsplit(".", 1)[-1]
    if t == "Local" and layer == "RPCFramework":
        return "grpc.detail " + name
    if t == "Exit" and layer == "Database":
        stmt = n["tags"].get("db.statement") or ""
        if not stmt and name == "Mysql/Ping":
            return "db PING"
        verb = (stmt or name).split(None, 1)[0].upper()
        return f"db {verb}"
    if t == "Exit" and layer == "Cache":
        op = name.split("/", 1)[-1]
        return f"redis {op}"
    return f"other {t}/{layer}/{name}"


# --------------------------------------------------------------- otel side --

KIND = {1: "internal", 2: "server", 3: "client", 4: "producer", 5: "consumer"}


def load_otel(path):
    data = json.load(open(path, encoding="utf-8"))
    nodes = {}
    for rs in data.get("resourceSpans") or []:
        attrs = {}
        for a in rs["resource"].get("attributes") or []:
            attrs[a["key"]] = next(iter(a["value"].values()))
        svc = attrs.get("service.name", "?")
        for ss in rs.get("scopeSpans") or []:
            for sp in ss.get("spans") or []:
                ka = {}
                for a in sp.get("attributes") or []:
                    ka[a["key"]] = next(iter(a["value"].values()))
                nodes[sp["spanId"]] = {
                    "service": svc,
                    "name": sp.get("name", ""),
                    "kind": KIND.get(sp.get("kind"), "?"),
                    "attrs": ka,
                    "trace": sp.get("traceId"),
                    "parent": sp.get("parentSpanId") or None,
                }
    traces = defaultdict(list)
    for sid, n in nodes.items():
        if n["parent"] not in nodes:
            n["parent"] = None
        traces[n["trace"]].append(sid)
    out = []
    for members in traces.values():
        out.append({"nodes": {k: nodes[k] for k in members}, "keys": members})
    return out


def canon_otel(n):
    a, kind, name = n["attrs"], n["kind"], n["name"]
    if kind == "server" and "http.request.method" in a:
        route = a.get("http.route") or a.get("url.path", "")
        return f"http.server {a['http.request.method']} {route}"
    if kind == "client" and "url.full" in a:
        url = a["url.full"]
        path = url.split("//", 1)[-1].split("/", 1)
        path = "/" + path[1] if len(path) > 1 else url
        path = path.split("?")[0]
        return f"http.client {a.get('http.request.method', name)} {path}"
    if kind in ("server", "client") and a.get("rpc.system") == "grpc":
        return f"grpc.{kind} {a.get('rpc.service')}/{a.get('rpc.method')}"
    if a.get("db.system.name") == "mysql":
        verb = (a.get("db.query.text") or name).split(None, 1)[0].upper()
        return f"db {verb}"
    if a.get("db.system.name") == "redis":
        return f"redis {a.get('db.operation.name', name)}"
    return f"other {kind}/{name}"


# ----------------------------------------------------------------- generic --

def tree_of(trace, loader="sky"):
    nodes, keys = trace["nodes"], trace["keys"]
    children = defaultdict(list)
    roots = []
    for k in keys:
        n = nodes[k]
        p = n.get("parent")
        if loader == "sky":
            p = sky_parent(trace, k)
        if p and p in nodes:
            children[p].append(k)
        else:
            roots.append(k)

    def build(k, depth=0):
        return {"key": k, "depth": depth,
                "kids": [build(c, depth + 1) for c in sorted(children[k], key=lambda x: str(x))]}

    return [build(r) for r in roots]


def sky_parent(trace, key):
    segid, spanid = key
    n = trace["nodes"][key]
    # reconstructed in load_sky: we stored no explicit parent; re-derive below
    return n.get("_parent")


def render(trace, canon, out):
    for root in tree_of(trace, trace.get("_loader", "sky")):
        dump_node(root, trace["nodes"], canon, out)


def dump_node(node, nodes, canon, out, prefix=""):
    n = nodes[node["key"]]
    role = canon(n)
    out.append(f"{prefix}[{n['service']}] {role}   ({n['name']})")
    for kid in node["kids"]:
        dump_node(kid, nodes, canon, out, prefix + "  ")


# The two loaders differ in how parents are derived; rebuild both into a
# uniform shape: each node gets "_parent" (global key) before comparison.
def normalize_sky(traces):
    for tr in traces:
        nodes = tr["nodes"]
        for item in tr["_raw_segments"]:
            segid = item["segmentId"]
            for sp in item.get("spans") or []:
                key = (segid, sp["spanId"])
                parent = None
                for ref in sp.get("refs") or []:
                    parent = (ref.get("parentTraceSegmentId"), ref.get("parentSpanId"))
                    break
                if parent is None and sp.get("parentSpanId", -1) >= 0:
                    parent = (segid, sp["parentSpanId"])
                nodes[key]["_parent"] = parent
        tr["_loader"] = "sky"


def normalize_otel(traces):
    for tr in traces:
        for k, n in tr["nodes"].items():
            n["_parent"] = n["parent"]
        tr["_loader"] = "otel"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--sky", required=True)
    ap.add_argument("--otel", required=True)
    args = ap.parse_args()

    sky = load_sky_with_raw(args.sky)
    otel = load_otel(args.otel)
    normalize_sky(sky)
    normalize_otel(otel)

    def canon(tr, n):
        return canon_sky(n) if tr["_loader"] == "sky" else canon_otel(n)

    def roots_role(tr):
        rs = []
        for k in tr["keys"]:
            if tr["nodes"][k]["_parent"] not in tr["nodes"]:
                rs.append(canon(tr, tr["nodes"][k]))
        return rs

    # ---- 1. trace inventory by root role
    def inventory(traces):
        c = Counter()
        for tr in traces:
            for r in roots_role(tr):
                c[r] += 1
        return c

    inv_sky, inv_otel = inventory(sky), inventory(otel)
    print("=" * 78)
    print("1) TRACE INVENTORY (traces by root span role)")
    print("=" * 78)
    roles = sorted(set(inv_sky) | set(inv_otel))
    print(f"{'root role':<44} {'sky':>5} {'otel':>5}")
    for r in roles:
        mark = "" if inv_sky[r] == inv_otel[r] else "   <-- count differs"
        print(f"{r:<44} {inv_sky[r]:>5} {inv_otel[r]:>5}{mark}")

    # ---- 2. side-by-side tree of the first business trace
    biz = "http.server GET /api/order"
    print()
    print("=" * 78)
    print("2) BUSINESS TRACE TOPOLOGY (first GET /api/order trace each side)")
    print("=" * 78)
    sky_biz = next((t for t in sky if biz in roots_role(t)), None)
    otel_biz = next((t for t in otel if biz in roots_role(t)), None)
    for label, tr in (("SKYWALKING", sky_biz), ("OTEL", otel_biz)):
        print(f"--- {label} ---")
        if tr is None:
            print("  <no such trace>")
            continue
        out = []
        render(tr, lambda n: canon(tr, n), out)
        for line in out:
            print("  " + line)

    # ---- 3. role multiset across all business traces
    print()
    print("=" * 78)
    print("3) ROLE MULTISET across ALL business traces (grpc.detail = known convention extra)")
    print("=" * 78)
    def multiset(traces, want):
        c = Counter()
        for tr in traces:
            if want not in roots_role(tr):
                continue
            for k in tr["keys"]:
                c[canon(tr, tr["nodes"][k])] += 1
        return c

    ms_sky = multiset(sky, biz)
    ms_otel = multiset(otel, biz)
    all_roles = sorted(set(ms_sky) | set(ms_otel))
    hard = 0
    for r in all_roles:
        a, b = ms_sky[r], ms_otel[r]
        if a == b:
            status = "MATCH"
        elif r.startswith("grpc.detail"):
            status = "sky-extra (convention)"
        else:
            status = "DIFF"
            hard += 1
        print(f"{r:<46} {a:>4} {b:>4}  {status}")
    print()
    print(f"business traces: sky={sum(1 for t in sky if biz in roots_role(t))} "
          f"otel={sum(1 for t in otel if biz in roots_role(t))}, "
          f"hard role mismatches: {hard}")

    # ---- 4. attribute spot check
    print()
    print("=" * 78)
    print("4) ATTRIBUTE SPOT CHECK (representative spans, first business trace)")
    print("=" * 78)
    picks = ["db INSERT", "db SELECT", "db UPDATE", "redis set", "redis get",
             "http.client GET /api/inventory", "grpc.client api.Inventory/Reserve"]
    for want in picks:
        rows = []
        for label, tr in (("sky", sky_biz), ("otel", otel_biz)):
            if tr is None:
                continue
            for k in tr["keys"]:
                n = tr["nodes"][k]
                if canon(tr, n) == want:
                    if label == "sky":
                        detail = {kk: vv for kk, vv in n["tags"].items()}
                        if n["peer"]:
                            detail["peer"] = n["peer"]
                    else:
                        keep = ("db.query.text", "db.namespace", "url.full",
                                "http.response.status_code", "rpc.grpc.status_code",
                                "server.address", "server.port", "db.operation.name")
                        detail = {kk: vv for kk, vv in n["attrs"].items() if kk in keep}
                    rows.append((label, detail))
                    break
        print(f"--- {want}")
        for label, detail in rows:
            print(f"  {label:>5}: {json.dumps(detail, ensure_ascii=False)}")


def load_sky_with_raw(path):
    data = yaml.safe_load(open(path, encoding="utf-8"))
    nodes = {}
    raw = []
    for item in data.get("segmentItems") or []:
        svc = item["serviceName"]
        for seg in item.get("segments") or []:
            segid = seg["segmentId"]
            raw.append({"service": svc, "segmentId": segid, "spans": seg.get("spans") or []})
            for sp in seg.get("spans") or []:
                key = (segid, sp["spanId"])
                tags = {t["key"]: t.get("value", "") for t in sp.get("tags") or []}
                nodes[key] = {
                    "service": svc,
                    "name": sp.get("operationName", ""),
                    "type": sp.get("spanType"),
                    "layer": sp.get("spanLayer"),
                    "peer": sp.get("peer", ""),
                    "tags": tags,
                }
    # union-find over spans to group into traces
    edges = []
    for seg in raw:
        for sp in seg["spans"]:
            linked = False
            for ref in sp.get("refs") or []:
                edges.append(((seg["segmentId"], sp["spanId"]),
                              (ref.get("parentTraceSegmentId"), ref.get("parentSpanId"))))
                linked = True
            if not linked and sp.get("parentSpanId", -1) >= 0:
                edges.append(((seg["segmentId"], sp["spanId"]),
                              (seg["segmentId"], sp["parentSpanId"])))
    keys = list(nodes)
    idx = {k: i for i, k in enumerate(keys)}
    par = list(range(len(keys)))

    def find(x):
        while par[x] != x:
            par[x] = par[par[x]]
            x = par[x]
        return x

    for c, p in edges:
        if c in idx and p in idx:
            ra, rb = find(idx[c]), find(idx[p])
            if ra != rb:
                par[rb] = ra

    groups = defaultdict(list)
    for k, i in idx.items():
        groups[find(i)].append(k)
    traces = []
    for members in groups.values():
        tr = {"nodes": {k: dict(nodes[k]) for k in members}, "keys": members}
        tr["_raw_segments"] = [s for s in raw
                               if any(m[0] == s["segmentId"] for m in members)]
        traces.append(tr)
    return traces


if __name__ == "__main__":
    main()
