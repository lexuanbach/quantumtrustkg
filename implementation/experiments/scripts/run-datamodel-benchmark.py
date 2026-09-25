#!/usr/bin/env python3
"""Data-model microbenchmark: RDF/SPARQL against a property graph and a relational store.

Sect. 3 of the paper stores the evidence graph as RDF and queries it with
SPARQL. The camera-ready gives the reasons (open schema, late multi-way joins,
provenance per statement) and leaves the cost measurement to the extended
version, which reports it in the knowledge-graph section and in the appendix
subsection on the data-model microbenchmark (Table "datamodel"). This script
produces that table's data.

It reads one Turtle fixture (the Makefile target datamodel-benchmark passes
scale-1000.ttl, 1000 services and 2000 communication edges), loads it into three
in-process engines and answers the same per-edge admissibility question on each:

  * relational      : SQLite from the standard library. Normalized tables and
                      one SQL join with a correlated subquery.
  * property-graph  : networkx DiGraph. Services are nodes that carry their
                      supported classes and strongest policy requirement, and
                      the trust boundary is an edge attribute.
  * rdf             : rdflib plus one SPARQL query when rdflib imports. Without
                      rdflib a small in-memory triple store answers the same
                      predicate with hand-written joins. The notes column of
                      the CSV says which of the two ran.

Admissibility rule. An edge is admissible when the strongest security class that
both endpoints support is at least the required class of the edge. The required
class is the maximum of the boundary requirement (regulated 2, partner 1,
internal 0) and the strongest requirement among the policies that apply to the
destination. This is the per-edge check of the filter stage of QTPO applied to
declared capabilities. The controller's filterEdges path is the reference for the
rule, but this script re-implements it over the fixture and does not call the Go
code.

Measurement. Each engine is run --repeats times (5 by default). load_ms covers
parsing the Turtle and building the engine's structures, and query_ms covers the
join alone. Both columns are medians over the repeats. The relational and
property-graph engines share one regex parser, parse_fixture, which means their load time
includes that parse. rdflib uses its own Turtle parser. Load times are therefore
comparable in kind but not in implementation, and only query_ms isolates the
join. Every engine also returns its admissible-edge count, and the CSV places it
next to reference_admissible, computed in plain Python by reference_admissible().
Equal counts are what show that the three joins implement the same predicate.
The script does not assert this itself.

Output: the CSV given by --out (datamodel-benchmark.csv in the Makefile target).
It backs the extended-version statement that the property-graph and relational
encodings answer the join faster than rdflib/SPARQL, and that RDF pays for this
with native per-statement provenance. The provenance column holds a qualitative
label written by hand in this file. It is not measured.

Limitation: this is a microbenchmark of one join on one fixture. rdflib is a
pure-Python library used only for the comparison. The deployed store is Jena
Fuseki. As a result the absolute rdflib times say nothing about the controller's
selection latency.
"""

from __future__ import annotations

import argparse
import csv
import re
import sqlite3
import statistics
import time
from pathlib import Path

RANK = {"classical": 0, "hybrid": 1, "quantum_safe": 2}
BOUNDARY_REQ = {"regulated": 2, "partner": 1, "internal": 0, "": 0}


def parse_fixture(path: Path) -> dict:
    """Read the fixture into plain Python structures.

    Returns a dict with profiles (profile -> security class), policies
    (policy -> required rank), services (service -> policies and supported
    classes) and edges (id, source, destination, boundary). The parser is a
    regex reader for the layout that the fixture generator writes (one subject
    block per resource) and is not a general Turtle parser. Unknown resource
    types are skipped.
    """
    text = path.read_text(encoding="utf-8")
    # A block ends at a full stop at the end of a line, when the next line starts
    # a new subject (":") or the file ends.
    blocks = re.split(r"\.\s*\n(?=:|\Z)", text)
    profiles: dict[str, str] = {}      # profile -> class
    policies: dict[str, int] = {}      # policy -> required rank
    services: dict[str, dict] = {}     # service -> {"policies": [...], "supports": set(class)}
    edges: list[tuple[str, str, str, str]] = []  # (id, src, dst, boundary)

    for block in blocks:
        b = block.strip()
        if not b or b.startswith("@prefix"):
            continue
        subj_match = re.match(r"(:[A-Za-z0-9_]+)\s+a\s+:(\w+)", b)
        if not subj_match:
            continue
        subj, rdf_type = subj_match.group(1), subj_match.group(2)

        if rdf_type == "Protocol":
            m = re.search(r':hasSecurityCategory\s+"([^"]+)"', b)
            if m:
                profiles[subj] = m.group(1)
        elif rdf_type == "Policy":
            m = re.search(r':requiresLevel\s+"([^"]+)"', b)
            if m:
                policies[subj] = RANK.get(m.group(1), 0)
        elif rdf_type == "Service":
            pols = re.findall(r':subjectTo\s+((?::[A-Za-z0-9_]+(?:,\s*)?)+)', b)
            supp = re.findall(r':supportsProfile\s+((?::[A-Za-z0-9_]+(?:,\s*)?)+)', b)
            pol_list = re.findall(r':[A-Za-z0-9_]+', pols[0]) if pols else []
            supp_list = re.findall(r':[A-Za-z0-9_]+', supp[0]) if supp else []
            services[subj] = {"policies": pol_list, "supports_profiles": supp_list}
        elif rdf_type == "Communication":
            src = re.search(r':sourceService\s+(:[A-Za-z0-9_]+)', b)
            dst = re.search(r':destinationService\s+(:[A-Za-z0-9_]+)', b)
            bnd = re.search(r':crossesBoundary\s+"([^"]+)"', b)
            if src and dst:
                edges.append((subj, src.group(1), dst.group(1), bnd.group(1) if bnd else ""))

    # A profile that the fixture does not define counts as classical, which is the
    # weakest class and so never over-states what a service supports.
    for svc, info in services.items():
        info["supports"] = {profiles.get(p, "classical") for p in info["supports_profiles"]}
    return {"profiles": profiles, "policies": policies, "services": services, "edges": edges}


def required_rank(dst_info: dict, boundary: str, policies: dict) -> int:
    """Required class of an edge: the maximum of its boundary requirement and the
    strongest policy attached to the destination service."""
    req = BOUNDARY_REQ.get(boundary, 0)
    for pol in dst_info.get("policies", []):
        req = max(req, policies.get(pol, 0))
    return req


def reference_admissible(model: dict) -> int:
    """Admissible-edge count computed directly on the parsed structures.

    Each engine's own count is written next to this one so that a disagreement
    between the joins is visible in the CSV. Edges that mention a service without
    a definition are skipped here.
    """
    services, policies = model["services"], model["policies"]
    count = 0
    for _, src, dst, boundary in model["edges"]:
        s, d = services.get(src), services.get(dst)
        if not s or not d:
            continue
        mutual = s["supports"] & d["supports"]
        best = max((RANK[c] for c in mutual), default=-1)
        if best >= required_rank(d, boundary, policies):
            count += 1
    return count


def bench_relational(path: Path, repeats: int) -> dict:
    """Load the model into an in-memory SQLite database and run one SQL join.

    Tables: supports(service, cls), svc_policy(service, req) with the strongest
    policy requirement per service, and edge(id, src, dst, boundary_req). The
    provenance label records that a relational design has to add columns or
    tables to attach a source to each fact.
    """
    load_times, query_times, admissible = [], [], 0
    for _ in range(repeats):
        t0 = time.perf_counter()
        # The parse is inside the timed load so that every engine pays for reading
        # the same Turtle text.
        model = parse_fixture(path)
        con = sqlite3.connect(":memory:")
        cur = con.cursor()
        cur.executescript(
            """
            CREATE TABLE supports(service TEXT, cls INTEGER);
            CREATE TABLE svc_policy(service TEXT, req INTEGER);
            CREATE TABLE edge(id TEXT, src TEXT, dst TEXT, boundary_req INTEGER);
            CREATE INDEX ix_sup ON supports(service);
            CREATE INDEX ix_pol ON svc_policy(service);
            """
        )
        for svc, info in model["services"].items():
            for c in info["supports"]:
                cur.execute("INSERT INTO supports VALUES(?,?)", (svc, RANK[c]))
            reqs = [model["policies"].get(p, 0) for p in info["policies"]] or [0]
            cur.execute("INSERT INTO svc_policy VALUES(?,?)", (svc, max(reqs)))
        for eid, src, dst, boundary in model["edges"]:
            cur.execute("INSERT INTO edge VALUES(?,?,?,?)", (eid, src, dst, BOUNDARY_REQ.get(boundary, 0)))
        con.commit()
        load_times.append((time.perf_counter() - t0) * 1000.0)

        t1 = time.perf_counter()
        # best is the highest class present in both supports sets. The subquery is
        # correlated on the edge's endpoints. A destination without policy gets 0.
        cur.execute(
            """
            SELECT COUNT(*) FROM (
              SELECT e.id,
                     MAX(e.boundary_req, COALESCE(p.req,0)) AS required,
                     (SELECT MAX(ss.cls) FROM supports ss
                        JOIN supports ds ON ss.cls = ds.cls
                       WHERE ss.service = e.src AND ds.service = e.dst) AS best
                FROM edge e LEFT JOIN svc_policy p ON p.service = e.dst
            ) WHERE best >= required
            """
        )
        admissible = cur.fetchone()[0]
        query_times.append((time.perf_counter() - t1) * 1000.0)
        con.close()
    return {
        "engine": "relational-sqlite",
        "load_ms": statistics.median(load_times),
        "query_ms": statistics.median(query_times),
        "admissible": admissible,
        "provenance": "bolt-on (extra columns/tables per fact)",
        "notes": f"sqlite {sqlite3.sqlite_version}",
    }


def bench_property_graph(path: Path, repeats: int) -> dict:
    """Load the model into a networkx DiGraph and evaluate the rule per edge.

    Per-node attributes hold the supported classes and the strongest policy
    requirement, and the boundary requirement sits on the edge. If networkx is
    not installed the row is written with empty timings.
    """
    try:
        import networkx as nx
    except ImportError:
        return {"engine": "property-graph-networkx", "load_ms": "", "query_ms": "",
                "admissible": "", "provenance": "per-node/edge props (partial)",
                "notes": "networkx unavailable"}
    load_times, query_times, admissible = [], [], 0
    for _ in range(repeats):
        t0 = time.perf_counter()
        model = parse_fixture(path)
        g = nx.DiGraph()
        for svc, info in model["services"].items():
            reqs = [model["policies"].get(p, 0) for p in info["policies"]] or [0]
            g.add_node(svc, supports={RANK[c] for c in info["supports"]}, policy_req=max(reqs))
        for eid, src, dst, boundary in model["edges"]:
            g.add_edge(src, dst, id=eid, boundary_req=BOUNDARY_REQ.get(boundary, 0))
        load_times.append((time.perf_counter() - t0) * 1000.0)

        t1 = time.perf_counter()
        cnt = 0
        for src, dst, data in g.edges(data=True):
            if src not in g or dst not in g:
                continue
            mutual = g.nodes[src]["supports"] & g.nodes[dst]["supports"]
            best = max(mutual, default=-1)
            required = max(data["boundary_req"], g.nodes[dst]["policy_req"])
            if best >= required:
                cnt += 1
        admissible = cnt
        query_times.append((time.perf_counter() - t1) * 1000.0)
    return {
        "engine": "property-graph-networkx",
        "load_ms": statistics.median(load_times),
        "query_ms": statistics.median(query_times),
        "admissible": admissible,
        "provenance": "per-node/edge props (partial)",
        "notes": f"networkx {nx.__version__}",
    }


def bench_rdf(path: Path, repeats: int) -> dict:
    """Use rdflib and SPARQL if rdflib imports, otherwise the fallback store."""
    try:
        import rdflib  # noqa: F401
        return _bench_rdflib(path, repeats)
    except ImportError:
        return _bench_triplestore(path, repeats)


def _bench_rdflib(path: Path, repeats: int) -> dict:
    """Parse the fixture with rdflib and count admissible edges with one SPARQL query.

    The inner SELECT groups by edge. Over the shared supported classes it takes
    the largest one as best, and over the boundary and destination-policy
    requirements it takes the larger as required. The outer FILTER keeps the edges
    with best >= required. Class names are mapped to ranks with BIND/IF, since the
    fixture stores them as string literals.
    """
    import rdflib

    NS = "http://quantumtrustkg.io/ontology#"
    # Same predicate as the other engines. An edge without a shared class has no
    # solution row in the inner query and is not counted, which matches best = -1
    # in the other engines.
    query = f"""
    PREFIX : <{NS}>
    SELECT (COUNT(DISTINCT ?e) AS ?n) WHERE {{
      {{
        SELECT ?e (MAX(?shared) AS ?best) (MAX(?rq) AS ?required) WHERE {{
          ?e a :Communication ; :sourceService ?src ; :destinationService ?dst .
          OPTIONAL {{ ?e :crossesBoundary ?b }}
          ?src :supportsProfile ?p  . ?p  :hasSecurityCategory ?sc .
          ?dst :supportsProfile ?p2 . ?p2 :hasSecurityCategory ?sc .
          BIND(IF(?sc="quantum_safe",2,IF(?sc="hybrid",1,0)) AS ?shared)
          BIND(IF(BOUND(?b) && ?b="regulated",2,IF(BOUND(?b) && ?b="partner",1,0)) AS ?bndrq)
          OPTIONAL {{ ?dst :subjectTo ?pol . ?pol :requiresLevel ?rl . }}
          BIND(IF(BOUND(?rl) && ?rl="quantum_safe",2,IF(BOUND(?rl) && ?rl="hybrid",1,0)) AS ?polrq)
          BIND(IF(?polrq > ?bndrq, ?polrq, ?bndrq) AS ?rq)
        }} GROUP BY ?e
      }}
      FILTER(?best >= ?required)
    }}
    """
    load_times, query_times = [], []
    result_n = 0
    for _ in range(repeats):
        t0 = time.perf_counter()
        g = rdflib.Graph()
        g.parse(str(path), format="turtle")
        load_times.append((time.perf_counter() - t0) * 1000.0)
        t1 = time.perf_counter()
        rows = list(g.query(query))
        result_n = int(rows[0][0]) if rows else 0
        query_times.append((time.perf_counter() - t1) * 1000.0)
    return {
        "engine": f"rdf-rdflib-{rdflib.__version__}",
        "load_ms": statistics.median(load_times),
        "query_ms": statistics.median(query_times),
        "admissible": result_n,
        "provenance": "native (named graphs / per-statement)",
        "notes": "rdflib parse + SPARQL join over shared support classes",
    }


def _bench_triplestore(path: Path, repeats: int) -> dict:
    """Fallback when rdflib is missing: read (s, p, o) triples and answer the same
    predicate with hand-written joins over dictionaries indexed by subject.

    This stand-in for a triple store has no SPARQL engine behind it. Its times are
    not comparable with the rdflib row, and the engine name in the CSV differs
    to keep the two cases apart."""
    text = path.read_text(encoding="utf-8")
    load_times, query_times = [], []
    admissible = 0
    for _ in range(repeats):
        t0 = time.perf_counter()
        triples = _turtle_to_triples(text)
        prof_class = {s: o.strip('"') for s, p, o in triples if p == ":hasSecurityCategory"}
        pol_req = {s: RANK.get(o.strip('"'), 0) for s, p, o in triples if p == ":requiresLevel"}
        support: dict[str, set[int]] = {}
        svc_pol: dict[str, list[str]] = {}
        for s, p, o in triples:
            if p == ":supportsProfile":
                support.setdefault(s, set()).add(RANK.get(prof_class.get(o, "classical"), 0))
            elif p == ":subjectTo":
                svc_pol.setdefault(s, []).append(o)
        src_of = {s: o for s, p, o in triples if p == ":sourceService"}
        dst_of = {s: o for s, p, o in triples if p == ":destinationService"}
        bnd_of = {s: o.strip('"') for s, p, o in triples if p == ":crossesBoundary"}
        load_times.append((time.perf_counter() - t0) * 1000.0)

        t1 = time.perf_counter()
        cnt = 0
        for e, src in src_of.items():
            dst = dst_of.get(e)
            if not dst:
                continue
            best = max(support.get(src, set()) & support.get(dst, set()), default=-1)
            required = BOUNDARY_REQ.get(bnd_of.get(e, ""), 0)
            for pol in svc_pol.get(dst, []):
                required = max(required, pol_req.get(pol, 0))
            if best >= required:
                cnt += 1
        admissible = cnt
        query_times.append((time.perf_counter() - t1) * 1000.0)
    return {
        "engine": "rdf-inmemory-triplestore",
        "load_ms": statistics.median(load_times),
        "query_ms": statistics.median(query_times),
        "admissible": admissible,
        "provenance": "native (per-statement, no rdflib installed)",
        "notes": "rdflib unavailable; hand-written triple store + BGP match",
    }


def _turtle_to_triples(text: str) -> list[tuple[str, str, str]]:
    """Split the fixture into (subject, predicate, object) triples with regexes.

    Handles the predicate-object and comma-list forms that the fixture uses.
    Objects are returned as written. String literals keep their quotes.
    """
    triples = []
    for block in re.split(r"\.\s*\n(?=:|\Z)", text):
        b = block.strip()
        if not b or b.startswith("@prefix"):
            continue
        sm = re.match(r"(:[A-Za-z0-9_]+)", b)
        if not sm:
            continue
        subj = sm.group(1)
        for pm in re.finditer(r"(:[A-Za-z0-9_]+|a)\s+((?::[A-Za-z0-9_]+(?:,\s*:[A-Za-z0-9_]+)*)|\"[^\"]*\")", b):
            pred, obj = pm.group(1), pm.group(2)
            if pred == subj:
                continue
            for o in re.split(r",\s*", obj):
                triples.append((subj, pred, o.strip()))
    return triples


def main() -> None:
    """Run the three engines on one fixture and write one CSV row per engine."""
    ap = argparse.ArgumentParser()
    ap.add_argument("--fixture", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--repeats", type=int, default=5)
    args = ap.parse_args()

    path = Path(args.fixture)
    model = parse_fixture(path)
    n_services = len(model["services"])
    n_edges = len(model["edges"])
    ref = reference_admissible(model)

    results = [
        bench_relational(path, args.repeats),
        bench_property_graph(path, args.repeats),
        bench_rdf(path, args.repeats),
    ]

    fieldnames = ["engine", "fixture", "services", "edges", "load_ms", "query_ms",
                  "total_ms", "admissible_edges", "reference_admissible",
                  "provenance_model", "repeats", "notes"]
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    with out.open("w", newline="", encoding="utf-8") as fh:
        w = csv.DictWriter(fh, fieldnames=fieldnames, lineterminator="\n")
        w.writeheader()
        for r in results:
            load = r["load_ms"]
            query = r["query_ms"]
            # total_ms stays empty when an engine was unavailable.
            total = ""
            if isinstance(load, (int, float)) and isinstance(query, (int, float)):
                total = round(load + query, 3)
            w.writerow({
                "engine": r["engine"],
                "fixture": path.name,
                "services": n_services,
                "edges": n_edges,
                "load_ms": round(load, 3) if isinstance(load, (int, float)) else "",
                "query_ms": round(query, 3) if isinstance(query, (int, float)) else "",
                "total_ms": total,
                "admissible_edges": r["admissible"],
                "reference_admissible": ref,
                "provenance_model": r["provenance"],
                "repeats": args.repeats,
                "notes": r["notes"],
            })
    print(f"reference admissible={ref}; wrote {out}")


if __name__ == "__main__":
    main()
