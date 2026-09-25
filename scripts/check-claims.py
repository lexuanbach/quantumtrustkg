#!/usr/bin/env python3
"""Check the numbers of the paper against the committed result files.

Every quantitative claim of the camera-ready paper (paper/camera-ready.pdf) that
comes from a result file, and the headline claims and result tables of the
extended version (paper/extended.pdf), is listed in CLAIMS below. Each entry
names the paper location, the value printed in the paper, the paper's
measured/modelled tag, the result file and key, and the command that produces
the file. The check reads the file, rounds as the paper does and compares.

Statuses:
  PASS      the artifact value matches the paper value within the paper's rounding
  WARN      a known difference between paper and artifact, explained in the note
            (it does not make the script fail)
  UNTRACED  the paper value has no result file in the artifact (does not fail)
  SKIP      host-dependent timing claim skipped with --skip-timing
  FAIL      the artifact value does not match (the script exits with status 1)

Usage:
  python3 scripts/check-claims.py                       check the committed files
  python3 scripts/check-claims.py --processed DIR --skip-timing
                                                        check a regenerated output folder
  python3 scripts/check-claims.py --markdown            print the claim matrix as Markdown
"""
from __future__ import annotations

import argparse
import csv
import json
import re
import statistics
import sys
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
IMPL = ROOT / "implementation"
DEFAULT_PROCESSED = IMPL / "experiments/outputs/processed"
TOPO = IMPL / "experiments/fixtures/real-topologies/alibaba"
CANONICAL = IMPL / "experiments/canonical/paper-canonical-results.json"

PROCESSED = DEFAULT_PROCESSED

# Evidence classes. "timing" claims depend on the host and are skipped by --skip-timing.
DET = "H2/H3 regenerated, deterministic"
REPLAY = "H1 or model, replayed from canonical record"
TIMING = "host-dependent measurement"
TOPOLOGY = "computed from committed edge lists"
CODE = "code or configuration constant"
DERIVED = "derived from result files"

# Paper method labels and the method keys in the CSV files.
METHODS = [
    ("QTPO", "QTKG"),
    ("NKG/NP", "NoKG"),
    ("OPP", "OPP"),
    ("ConfigProf.", "ConfigProfile"),
    ("Static", "Static"),
    ("ELCA", "ELCA"),
    ("SL-CA", "SL-CA"),
]
SCALES_REAL = [50, 100, 200, 500, 1000, 2000]


# ---------------------------------------------------------------- helpers

def rows(name: str) -> list[dict[str, str]]:
    with (PROCESSED / name).open(newline="", encoding="utf-8") as fh:
        return list(csv.DictReader(fh))


def row(name: str, **match: str) -> dict[str, str]:
    for r in rows(name):
        if all(r.get(k) == v for k, v in match.items()):
            return r
    raise KeyError(f"{name}: no row with {match}")


def val(name: str, col: str, **match: str) -> float:
    return float(row(name, **match)[col])


def decimals(text: str) -> int:
    m = re.search(r"\.(\d+)", text)
    return len(m.group(1)) if m else 0


def near(actual: float, paper: str) -> bool:
    """True when actual rounds to the printed paper value."""
    tol = 0.5 * 10 ** (-decimals(paper)) + 1e-9
    return abs(actual - float(paper.replace(",", ""))) <= tol


def sub(label: str, paper: str, actual: float | int | str, ok: bool | None = None) -> tuple:
    if ok is None:
        ok = near(float(actual), paper) if isinstance(actual, (int, float)) else str(actual) == paper
    return (label, paper, actual, ok)


def edges(scale: str) -> list[tuple[str, str]]:
    out = []
    with (TOPO / f"alibaba_{scale}.edges").open(encoding="utf-8") as fh:
        for line in fh:
            parts = line.split()
            if len(parts) >= 2 and not line.startswith("#"):
                out.append((parts[0], parts[1]))
    return out


def hub_share(scale: str) -> float:
    e = edges(scale)
    return 100.0 * Counter(d for _, d in e).most_common(1)[0][1] / len(e)


def top5_share(scale: str) -> float:
    e = edges(scale)
    nodes = {a for a, _ in e} | {b for _, b in e}
    k = max(1, round(0.05 * len(nodes)))
    return 100.0 * sum(c for _, c in Counter(d for _, d in e).most_common(k)) / len(e)


def grep(path: Path, pattern: str) -> bool:
    return re.search(pattern, path.read_text(encoding="utf-8")) is not None


def canonical() -> dict:
    return json.loads(CANONICAL.read_text(encoding="utf-8"))


BC = "baseline-comparison.csv"
RA = "baseline-comparison-real-alibaba.csv"
ST = "staleness-sweep-results.csv"
SR = "staleness-sweep-real-alibaba.csv"
WS = "weight-sweep-results.csv"
CW = "compliance-wilson.csv"
LT = "latency-results.csv"
SC = "setup-cost-results.csv"
HS = "pq-tls-handshake-results.csv"
WT = "wire-tls-validation.csv"


def t1(key: str, col: str) -> float:
    return val(BC, col, scale="200", method=key)


def t1_invalid_pct(key: str) -> float:
    """Pooled invalid-promotion share from the counts. The CSV column holds it rounded to two decimals."""
    return 100.0 * t1(key, "invalid_promotions") / t1(key, "managed_edges")


# ---------------------------------------------------------------- claims
# Each claim: id, where, claim, paper value, paper tag, class, file/key, command, check.
# The check returns a list of sub-checks (label, paper, artifact, ok).

CLAIMS: list[dict] = []


def claim(cid, where, text, paper, tag, cls, source, command, status=None, note=""):
    def deco(fn):
        CLAIMS.append(dict(id=cid, where=where, text=text, paper=paper, tag=tag, cls=cls,
                           source=source, command=command, fn=fn, status=status, note=note))
        return fn
    return deco


MK = "make -C implementation"

# ---- camera-ready: disclosures

@claim("C00", "CR Sect. 4, RQ3", "H1 values are replayed and the setup cost is modelled", "disclosed",
       "", REPLAY, "canonical/paper-canonical-results.json: scope", f"{MK} paper-results")
def _():
    scope = canonical().get("scope", "").lower()
    return [sub("replay disclosure", "yes", "yes" if "replayed" in scope and "raw per-repetition logs" in scope else "no"),
            sub("modelled disclosure", "yes", "yes" if "modelled" in scope else "no")]

# ---- camera-ready: abstract and conclusion

@claim("C01", "CR Abstract, Sect. 7", "QTPO unsafe promotions, H2 200-service meshes", "0", "untagged (measured)",
       DET, f"{BC}: scale=200, method=QTKG, unsafe_promotions", f"{MK} baseline-compare")
def _():
    return [sub("QTPO unsafe n", "0", int(t1("QTKG", "unsafe_promotions")))]


@claim("C02", "CR Abstract, Sect. 7", "QTPO declared-label compliance, H2 200 services", "96.5%", "untagged (measured)",
       DET, f"{BC}: scale=200, method=QTKG, correct_promotion_pct (median over reps)", f"{MK} baseline-compare")
def _():
    return [sub("QTPO C_decl", "96.5", t1("QTKG", "correct_promotion_pct"))]


@claim("C03", "CR Abstract, Sect. 7", "service-level and opportunistic baselines, unsafe on H2", "3.4-13.7%",
       "untagged (measured)", DET, f"{BC}: scale=200, OPP and SL-CA invalid_promotions/managed_edges", f"{MK} baseline-compare",
       note="13.7 is 1640/11929 = 13.748%. The CSV column rounds it to 13.75. The paper said 13.8 before 2026-09-25.")
def _():
    return [sub("OPP (low end)", "3.4", t1_invalid_pct("OPP")),
            sub("SL-CA (high end)", "13.7", t1_invalid_pct("SL-CA"))]


@claim("C04", "CR Abstract, Sect. 7", "service-level and per-endpoint baselines (SL-CA, ConfigProfile) on sampled Alibaba graphs up to 2000 services",
       "37-60%", "untagged (measured)", DET, f"{RA}: SL-CA and ConfigProfile unsafe_promotion_pct, all six scales",
       f"{MK} baseline-compare",
       note="Resolved 2026-09-25. The sentence used to say 'these baselines', which included OPP (4.0-6.1% here).")
def _():
    v = [val(RA, "unsafe_promotion_pct", scale=str(s), method=k) for k in ("SL-CA", "ConfigProfile") for s in SCALES_REAL]
    return [sub("lowest (ConfigProfile, 2000)", "37", min(v)), sub("highest (SL-CA, 50)", "60", max(v))]


@claim("C05", "CR Abstract, Sect. 4.2", "QTPO unsafe promotions on all six Alibaba scales", "0 (0.00%)", "[M]",
       DET, f"{RA}: method=QTKG, unsafe_promotions", f"{MK} baseline-compare")
def _():
    return [sub(f"QTPO {s}", "0", int(val(RA, "unsafe_promotions", scale=str(s), method="QTKG"))) for s in SCALES_REAL]

# ---- camera-ready: Sect. 3

@claim("C06", "CR Sect. 3", "default freshness window", "300 s", "", CODE,
       "controller/internal/controllers/crd_parsing.go: freshnessSeconds = 300", "code")
def _():
    ok = grep(IMPL / "controller/internal/controllers/crd_parsing.go", r"freshnessSeconds = 300")
    return [sub("default", "300", "300" if ok else "missing")]


@claim("C07", "CR Sect. 3", "score weights w_o, w_r, w_m, w_h and hysteresis delta", "0.25, 0.40, 0.20, 0.15, 0.05",
       "", CODE, f"orchestration/config.go DefaultScoreConfig and {WS}: config=operating-point", f"{MK} weight-sweep")
def _():
    r = row(WS, config="operating-point")
    src = grep(IMPL / "controller/internal/orchestration/config.go",
               r"Wo: 0\.25, Wr: 0\.40, Wm: 0\.20, Wh: 0\.15, Delta: 0\.05")
    return [sub("w_o", "0.25", float(r["w_o"])), sub("w_r", "0.40", float(r["w_r"])),
            sub("w_m", "0.20", float(r["w_m"])), sub("w_h", "0.15", float(r["w_h"])),
            sub("delta", "0.05", float(r["delta"])),
            sub("DefaultScoreConfig", "present", "present" if src else "missing")]


@claim("C08", "CR Sect. 3, Theorems 1-2", "Coq/Rocq theorems G1, G2, G4 (and G3 in the extended version)",
       "checked", "", CODE, "formal/coq/Guarantees.v", "make -C formal/coq",
       note="Prop. 1 (weight-independence) is argued in prose and is not a Coq lemma. Coq select takes the "
            "first admissible profile in list order and never reads scores.")
def _():
    g = (ROOT / "formal/coq/Guarantees.v").read_text(encoding="utf-8")
    names = ["G1_promoted_admissible", "G2_security_class_preserved", "G3_hybrid_metadata_preserved", "G4_fail_closed"]
    out = [sub(n, "Theorem", "Theorem" if re.search(rf"Theorem {n}\b", g) else "missing") for n in names]
    admitted = sum(len(re.findall(r"\b(Admitted|admit|Axiom)\b", (ROOT / f"formal/coq/{f}").read_text(encoding="utf-8")))
                   for f in ("Model.v", "QTPO.v", "Guarantees.v", "Examples.v"))
    out.append(sub("Admitted/admit/Axiom", "0", admitted))
    return out

# ---- camera-ready: Sect. 4 setup

@claim("C09", "CR Sect. 4", "H2 seed, repetitions and corrupted-provenance share", "20260509, 30, 3%", "", CODE,
       f"implementation/Makefile, {BC}: reps, cmd/baseline-compare provBad < 0.03", f"{MK} baseline-compare",
       note="0.03 is the per-edge probability. The realised share at 200 services is 2.55%.")
def _():
    mk = grep(IMPL / "Makefile", r"--seed 20260509 --reps 30")
    gen = grep(IMPL / "controller/cmd/baseline-compare/main.go", r"provBad := rng\.Float64\(\) < 0\.03")
    return [sub("seed and reps in Makefile", "present", "present" if mk else "missing"),
            sub("reps column", "30", int(t1("QTKG", "reps"))),
            sub("3% corruption constant", "present", "present" if gen else "missing")]


@claim("C10", "CR Sect. 4 (EXT Sect. 8.1, Table 2)", "H2 meshes include requirement divergence at shared destinations and 3% corrupted provenance",
       "divergence at shared destinations, 3%", "", CODE,
       "cmd/baseline-compare generateMesh: forcePartner < 0.40, provBad < 0.03", f"{MK} baseline-compare",
       note="Resolved 2026-09-25. The papers said 'about 30%', which no output reports. The generator gives a "
            "hybrid-capped partner edge to 40% of the quantum-safe-capable destinations (21.8% of edges enter such "
            "a destination at 200 services).")
def _():
    src = IMPL / "controller/cmd/baseline-compare/main.go"
    return [sub("partner edge at shared destinations", "present",
                "present" if grep(src, r"forcePartner := tier\[dst\] == 2 && rng\.Float64\(\) < 0\.40") else "missing"),
            sub("3% corrupted provenance", "present",
                "present" if grep(src, r"provBad := rng\.Float64\(\) < 0\.03") else "missing")]


@claim("C11", "CR Sect. 4", "H1 at 200 services: valid / unpromoted / invalid", "99.1% / 0.9% / 0%",
       "untagged (replayed)", REPLAY, f"{CW}: 200-services qtkg_percent, blocked-safe-breakdown.csv unsafe-promotions",
       f"{MK} paper-results", note="0.9% is 100 - 99.1.")
def _():
    v = val(CW, "qtkg_percent", scale="200-services")
    u = val("blocked-safe-breakdown.csv", "share_percent", category="unsafe-promotions")
    return [sub("valid", "99.1", v), sub("unpromoted", "0.9", round(100 - v, 1)), sub("invalid", "0", u)]


@claim("C12", "CR Sect. 4", "H2 at 200 services: valid / unpromoted / invalid", "96.5% / 3.4% / 0%",
       "untagged (measured)", DET, f"{BC}: scale=200, QTKG correct_promotion_pct, no_promotion/managed_edges",
       f"{MK} baseline-compare",
       note="96.5 is the median of per-repetition compliance. The pooled valid share is 11526/11929 = 96.6%, "
            "and the three pooled shares are 96.6/3.4/0.")
def _():
    m = t1("QTKG", "managed_edges")
    return [sub("valid", "96.5", t1("QTKG", "correct_promotion_pct")),
            sub("unpromoted", "3.4", 100 * t1("QTKG", "no_promotion") / m),
            sub("invalid", "0", t1("QTKG", "invalid_promotions"))]

# ---- camera-ready: RQ1

for _scale, _q, _n in [("50", ("99.4", "99.1", "99.6"), ("96.9", "96.5", "97.3")),
                       ("100", ("99.3", "99.0", "99.5"), ("96.6", "96.2", "96.9")),
                       ("200", ("99.1", "98.9", "99.3"), ("96.4", "96.0", "96.7"))]:
    @claim(f"C13-{_scale}", "CR Sect. 4.1 (ext. Table 3)", f"H1 C_decl^cluster with Wilson interval, {_scale} services",
           f"QTPO {_q[0]} [{_q[1]}, {_q[2]}], NoKG/NoProv {_n[0]} [{_n[1]}, {_n[2]}]", "untagged (replayed)", REPLAY,
           f"{CW}: {_scale}-services", f"{MK} paper-results")
    def _(s=_scale, q=_q, n=_n):
        r = row(CW, scale=f"{s}-services")
        cols = ["qtkg_percent", "qtkg_wilson_low", "qtkg_wilson_high",
                "qtpo_nokg_percent", "qtpo_nokg_wilson_low", "qtpo_nokg_wilson_high"]
        return [sub(c, p, float(r[c])) for c, p in zip(cols, q + n)]


@claim("C14", "CR Sect. 4.1", "archived run recorded zero invalid promotions", "0", "untagged (replayed)", REPLAY,
       "blocked-safe-breakdown.csv: unsafe-promotions, fault-injection-results.csv: unsafe_promotions",
       f"{MK} paper-results")
def _():
    fi = sum(int(r["unsafe_promotions"]) for r in rows("fault-injection-results.csv"))
    return [sub("breakdown", "0", val("blocked-safe-breakdown.csv", "share_percent", category="unsafe-promotions")),
            sub("fault injection", "0", fi)]


@claim("C15", "CR Sect. 4.1 (ext. Table 11)", "median selection latency 50 to 200 services", "12 ms to 45 ms", "[M]",
       REPLAY, f"{LT}: median_ms", f"{MK} paper-results")
def _():
    return [sub("50", "12", val(LT, "median_ms", scale="50-services")),
            sub("200", "45", val(LT, "median_ms", scale="200-services"))]


@claim("C16", "CR Sect. 4.1", "50- and 100-service meshes agree with Table 1 within", "1.2 points", "", DERIVED,
       f"{BC}: all methods, correct_promotion_pct and unsafe_promotion_pct", f"{MK} baseline-compare")
def _():
    worst = 0.0
    for _, key in METHODS:
        for col in ("correct_promotion_pct", "unsafe_promotion_pct"):
            ref = val(BC, col, scale="200", method=key)
            for s in ("50", "100"):
                worst = max(worst, abs(val(BC, col, scale=s, method=key) - ref))
    return [sub("max difference", "1.2", worst, worst <= 1.2 + 0.05)]


@claim("C17", "CR Sect. 4.1", "per-service/per-endpoint baselines unsafe, ConfigProfile under ELCA", "11.2-13.7%, 11.2% < 13.5%",
       "untagged (measured)", DET, f"{BC}: scale=200 ELCA, SL-CA, ConfigProfile invalid_promotions/managed_edges",
       f"{MK} baseline-compare")
def _():
    return [sub("ConfigProfile", "11.2", t1_invalid_pct("ConfigProfile")),
            sub("ELCA", "13.5", t1_invalid_pct("ELCA")),
            sub("SL-CA", "13.7", t1_invalid_pct("SL-CA"))]


@claim("C18", "CR Sect. 4.1", "NoKG/NoProv gap to QTPO", "roughly 3%", "untagged (measured)", DET,
       f"{BC}: scale=200 NoKG unsafe_promotion_pct", f"{MK} baseline-compare")
def _():
    v = t1("NoKG", "unsafe_promotion_pct")
    return [sub("NoKG unsafe", "roughly 3", v, 2.5 <= v <= 3.5)]

# ---- camera-ready: Table 1

_T1 = {  # paper Table 1: valid %, invalid %, invalid n, no promotion n, retained n
    "QTKG": ("96.5", "0.0", "0", "403", "0"),
    "NoKG": ("96.5", "2.5", "303", "100", "0"),
    "OPP": ("96.5", "3.4", "403", "0", "0"),
    "ConfigProfile": ("85.7", "11.2", "1336", "304", "0"),
    "Static": ("86.7", "13.1", "1557", "0", "0"),
    "ELCA": ("86.1", "13.5", "1614", "0", "0"),
    "SL-CA": ("85.7", "13.7", "1640", "0", "0"),
}
for _label, _key in METHODS:
    @claim(f"T1-{_key}", "CR Table 1 (ext. Table 4)", f"H2 200-service comparison, {_label}",
           " / ".join(_T1[_key]), "[M]", DET,
           f"{BC}: scale=200, method={_key}: correct_promotion_pct, invalid_promotions/managed_edges, "
           f"invalid_promotions, no_promotion, retained_active", f"{MK} baseline-compare",
           note=("Valid (%) is the median over repetitions and Invalid (%) is pooled, as the paper now states." if _key in ("ELCA", "SL-CA", "Static", "ConfigProfile") else ""))
    def _(k=_key):
        p = _T1[k]
        return [sub("correct_promotion_pct (median)", p[0], t1(k, "correct_promotion_pct")),
                sub("invalid % (pooled, from counts)", p[1], t1_invalid_pct(k)),
                sub("invalid_promotions", p[2], t1(k, "invalid_promotions")),
                sub("no_promotion", p[3], t1(k, "no_promotion")),
                sub("retained_active", p[4], t1(k, "retained_active"))]


@claim("T1-pooled", "CR Table 1 (ext. Table 4)", "pooled observations and repetitions", "11,929 (30 repetitions)", "[M]",
       DET, f"{BC}: scale=200 managed_edges, reps", f"{MK} baseline-compare")
def _():
    return [sub(f"{k} managed", "11929", int(t1(k, "managed_edges"))) for _, k in METHODS] + \
           [sub("reps", "30", int(t1("QTKG", "reps")))]

# ---- camera-ready: RQ2

@claim("C19", "CR Sect. 4.2", "Alibaba bucket size", "16,247 services, 48,746 edges", "", TOPOLOGY,
       "fixtures/real-topologies/alibaba/alibaba_full_16247svc.edges", "none (committed input)")
def _():
    e = edges("full_16247svc")
    return [sub("services", "16247", len({a for a, _ in e} | {b for _, b in e})),
            sub("unique edges", "48746", len(set(e)))]


@claim("C20", "CR Sect. 4.2 (ext. Table 6)", "unique edges per repetition and pooled observations",
       "50 to 3,692 unique, 1,500 to 110,760 pooled", "", TOPOLOGY,
       f"alibaba_<N>.edges line counts, {RA}: managed_edges", f"{MK} baseline-compare")
def _():
    return [sub("edges at 50", "50", len(edges("50"))), sub("edges at 2000", "3692", len(edges("2000"))),
            sub("pooled at 50", "1500", int(val(RA, "managed_edges", scale="50", method="QTKG"))),
            sub("pooled at 2000", "110760", int(val(RA, "managed_edges", scale="2000", method="QTKG")))]


@claim("C21", "CR Sect. 4.2 (EXT Sect. 8.5)", "SL-CA and ConfigProfile: real rates are about four to five times the synthetic ones at equal size",
       "about 4x to 5x", "", DERIVED, f"{RA} and {BC}: unsafe_promotion_pct at 50, 100, 200", f"{MK} baseline-compare",
       note="Ratios are 3.95-4.08 for SL-CA and 4.56-4.80 for ConfigProfile. Resolved 2026-09-25 (was 'three to four times').")
def _():
    out = []
    for k in ("SL-CA", "ConfigProfile"):
        for s in ("50", "100", "200"):
            r = val(RA, "unsafe_promotion_pct", scale=s, method=k) / val(BC, "unsafe_promotion_pct", scale=s, method=k)
            out.append(sub(f"{k} {s}", "about 4-5", round(r, 2), 3.9 <= r <= 5.0))
    return out


@claim("C22", "CR Sect. 4.2 (ext. Table 6)", "SL-CA and ConfigProfile fall monotonically from 50 to 2000 services",
       "59.87% and 56.60% to 40.15% and 37.19%", "[M]", DET, f"{RA}: SL-CA, ConfigProfile", f"{MK} baseline-compare")
def _():
    out = [sub("SL-CA 50", "59.87", val(RA, "unsafe_promotion_pct", scale="50", method="SL-CA")),
           sub("ConfigProfile 50", "56.60", val(RA, "unsafe_promotion_pct", scale="50", method="ConfigProfile")),
           sub("SL-CA 2000", "40.15", val(RA, "unsafe_promotion_pct", scale="2000", method="SL-CA")),
           sub("ConfigProfile 2000", "37.19", val(RA, "unsafe_promotion_pct", scale="2000", method="ConfigProfile"))]
    for k in ("SL-CA", "ConfigProfile"):
        seq = [val(RA, "unsafe_promotion_pct", scale=str(s), method=k) for s in SCALES_REAL]
        out.append(sub(f"{k} monotone", "yes", "yes" if all(a > b for a, b in zip(seq, seq[1:])) else "no"))
    return out


@claim("C23", "CR Sect. 4.2", "NoKG/NoProv on the Alibaba graphs", "near 3%", "untagged (measured)", DET,
       f"{RA}: NoKG unsafe_promotion_pct", f"{MK} baseline-compare")
def _():
    v = [val(RA, "unsafe_promotion_pct", scale=str(s), method="NoKG") for s in SCALES_REAL]
    return [sub("min", "near 3", min(v), min(v) >= 2.5), sub("max", "near 3", max(v), max(v) <= 3.5)]


@claim("C24", "CR Sect. 4.2, Fig. 2(a) bars (ext. Table 6 note)", "share of edges into the top hub",
       "98.0% to 54.1%, 6.1% in the full trace", "", TOPOLOGY, "alibaba_<N>.edges", "python3 scripts/make_camera_figures.py")
def _():
    return [sub("50", "98.0", hub_share("50")), sub("100", "98.0", hub_share("100")),
            sub("200", "93.9", hub_share("200")), sub("500", "83.9", hub_share("500")),
            sub("1000", "71.5", hub_share("1000")), sub("2000", "54.1", hub_share("2000")),
            sub("full trace", "6.1", hub_share("full_16247svc"))]


@claim("C25", "CR Sect. 4.2, Fig. 2(b) (ext. Table 7)", "mass staleness: QTPO unsafe at every level, C_decl range",
       "0 at every level, 99.03% to 9.87%", "[M]", DET, f"{ST}: method=QTKG", f"{MK} staleness-sweep")
def _():
    q = [r for r in rows(ST) if r["method"] == "QTKG"]
    return [sub("max QTPO unsafe n", "0", max(int(r["unsafe_promotions"]) for r in q)),
            sub("C_decl at 0%", "99.03", val(ST, "compliance_pct_median", stale_fraction="0.00", method="QTKG")),
            sub("C_decl at 90%", "9.87", val(ST, "compliance_pct_median", stale_fraction="0.90", method="QTKG"))]


@claim("C26", "CR Sect. 4.2, Fig. 2(b) (ext. Table 7)", "NoKG/NoProv unsafe at 25/50/90% corruption",
       "24.75/49.59/89.22%", "untagged (measured)", DET, f"{ST}: method=NoKG", f"{MK} staleness-sweep")
def _():
    return [sub(f, p, val(ST, "unsafe_promotion_pct", stale_fraction=f, method="NoKG"))
            for f, p in (("0.25", "24.75"), ("0.50", "49.59"), ("0.90", "89.22"))]


@claim("C27", "CR Sect. 4.2", "real 200-service topology under staleness: QTPO unsafe", "0", "untagged (measured)", DET,
       f"{SR}: method=QTKG", f"{MK} staleness-sweep")
def _():
    return [sub("max QTPO unsafe n", "0", max(int(r["unsafe_promotions"]) for r in rows(SR) if r["method"] == "QTKG"))]


@claim("C28", "CR Fig. 2(a)", "unsafe-promotion lines, 7 methods x 6 scales", "plotted", "[M]", DET,
       f"{RA}: unsafe_promotion_pct", "python3 scripts/make_camera_figures.py")
def _():
    n = sum(1 for _, k in METHODS for s in SCALES_REAL if row(RA, scale=str(s), method=k))
    return [sub("points", "42", n)]

# ---- camera-ready: RQ3

@claim("C29", "CR Sect. 4.3 (ext. Table 10)", "OpenSSL version and handshake count", "OpenSSL 3.6.2, 300 handshakes",
       "[M]", TIMING, f"{HS}: openssl_version, handshakes", f"{MK} pq-handshake")
def _():
    out = []
    for r in rows(HS):
        out.append(sub(f"{r['mode']} handshakes", "300", int(r["handshakes"])))
        out.append(sub(f"{r['mode']} OpenSSL", "3.6.2", "3.6.2" if "OpenSSL 3.6.2" in r["openssl_version"] else r["openssl_version"]))
    return out


@claim("C30", "CR Sect. 4.3 (ext. Table 10)", "median handshake time X25519 / X25519MLKEM768 / ML-KEM-768",
       "8.746 / 8.850 / 8.759 ms", "[M]", TIMING, f"{HS}: median_ms", f"{MK} pq-handshake")
def _():
    return [sub(m, p, val(HS, "median_ms", mode=m))
            for m, p in (("classical_tls13", "8.746"), ("hybrid_mlkem", "8.850"), ("pure_pq", "8.759"))]


@claim("C31", "CR Sect. 4.3, Fig. 2(c) solid bars", "handshake overhead over X25519", "+1.2% / +0.2%", "[M]", TIMING,
       f"{HS}: rel_overhead_pct", f"{MK} pq-handshake",
       note="rel_overhead_pct is computed from unrounded medians. From the rounded medians the pure PQ value is 0.15%.")
def _():
    return [sub("hybrid", "1.2", val(HS, "rel_overhead_pct", mode="hybrid_mlkem")),
            sub("pure PQ", "0.2", val(HS, "rel_overhead_pct", mode="pure_pq"))]


@claim("C32", "CR Sect. 4.3 (ext. Table 10)", "end-to-end wire trials per profile", "30/30", "[M]", TIMING,
       f"{WT}: successes/trials", f"{MK} wire-tls")
def _():
    return [sub(r["profile"], "30/30", f"{r['successes']}/{r['trials']}") for r in rows(WT)]


@claim("C33", "CR Sect. 4.3, Fig. 2(c) hatched bars (ext. Tables 8, 9)", "modelled aggregate setup cost",
       "8-12% hybrid, 12-15% pure PQ", "[P]", REPLAY, f"{SC}: setup_cost_percent_low/high", f"{MK} paper-results")
def _():
    return [sub("hybrid low", "8", val(SC, "setup_cost_percent_low", mode="hybrid")),
            sub("hybrid high", "12", val(SC, "setup_cost_percent_high", mode="hybrid")),
            sub("pure low", "12", val(SC, "setup_cost_percent_low", mode="pure-pqc")),
            sub("pure high", "15", val(SC, "setup_cost_percent_high", mode="pure-pqc"))]


@claim("C34", "CR Sect. 4.3", "modelled share of the aggregate", "roughly 7-15 of 8-15 points", "[P]", TIMING,
       f"{SC} minus {HS}", "none", note="Aggregate minus measured term gives 6.8-14.8 points. It uses the host "
       "timing of the handshake term.")
def _():
    lo = val(SC, "setup_cost_percent_low", mode="hybrid") - val(HS, "rel_overhead_pct", mode="hybrid_mlkem")
    hi = val(SC, "setup_cost_percent_high", mode="pure-pqc") - val(HS, "rel_overhead_pct", mode="pure_pq")
    return [sub("low", "7", lo), sub("high", "15", hi)]


@claim("C35", "CR Sect. 4.3 (ext. Tables 8, 9)", "rollout time and failure/rollback rates",
       "4-7 min and 0.0%/0.0% hybrid, 8-12 min and 1.1%/0.6% pure PQ", "[M]", REPLAY, f"{SC}: rollout and rate columns",
       f"{MK} paper-results")
def _():
    h, p = row(SC, mode="hybrid"), row(SC, mode="pure-pqc")
    return [sub("hybrid time", "4-7", f"{h['rollout_time_min_low']}-{h['rollout_time_min_high']}"),
            sub("hybrid fail", "0.0", float(h["fail_rate_percent"])),
            sub("hybrid rollback", "0.0", float(h["rollback_rate_percent"])),
            sub("pure time", "8-12", f"{p['rollout_time_min_low']}-{p['rollout_time_min_high']}"),
            sub("pure fail", "1.1", float(p["fail_rate_percent"])),
            sub("pure rollback", "0.6", float(p["rollback_rate_percent"]))]

# ---- camera-ready: Sect. 5

@claim("C36", "CR Sect. 5 (ext. Table A10)", "weight sweep: largest selection change, C_decl and unsafe at every point",
       "up to 20.5% of promoted edges, 96.5%, 0", "[M]", DET, f"{WS}: selection_changed_pct, correct_promotion_pct, unsafe_promotions",
       f"{MK} weight-sweep",
       note="selection_changed_pct is a share of promoted edges, as both papers now say.")
def _():
    r = rows(WS)
    return [sub("max selection change", "20.5", max(float(x["selection_changed_pct"]) for x in r)),
            sub("min C_decl", "96.5", min(float(x["correct_promotion_pct"]) for x in r)),
            sub("max C_decl", "96.5", max(float(x["correct_promotion_pct"]) for x in r)),
            sub("max unsafe n", "0", max(int(x["unsafe_promotions"]) for x in r))]

# ---- extended version: tables and headline claims not already covered

@claim("E01", "EXT Table 1", "software versions and repetition counts",
       "rdflib 7.6.0, SQLite 3.53.2, networkx 3.6.1, 15 iterations, 5 repeats", "", TIMING,
       "datamodel-benchmark.csv: engine/notes/repeats, scale-stress-results.csv: iterations",
       f"{MK} datamodel-benchmark scale-stress")
def _():
    dm = rows("datamodel-benchmark.csv")
    text = " ".join(r["engine"] + " " + r["notes"] for r in dm)
    return [sub("rdflib", "7.6.0", "7.6.0" if "rdflib-7.6.0" in text else text),
            sub("SQLite", "3.53.2", "3.53.2" if "sqlite 3.53.2" in text else text),
            sub("networkx", "3.6.1", "3.6.1" if "networkx 3.6.1" in text else text),
            sub("repeats", "5", int(dm[0]["repeats"])),
            sub("scale-stress iterations", "15", int(rows("scale-stress-results.csv")[0]["iterations"]))]


@claim("E02", "EXT Table 3", "H1 compliance of Static Istio, OPA, Manual, Greedy and the Online Boutique row",
       "75.1/90.3/92.4/96.7 ... OB 99.3 [97.9, 99.8]", "[M]", REPLAY, f"{CW}: all rows", f"{MK} paper-results")
def _():
    exp = {"50-services": ("75.1", "90.3", "92.4", "96.7"), "100-services": ("73.6", "89.1", "91.8", "96.1"),
           "200-services": ("72.7", "88.5", "91.3", "95.8"), "online-boutique-11-edges": ("76.4", "90.7", "92.9", "96.4")}
    out = []
    for s, vals in exp.items():
        r = row(CW, scale=s)
        for c, p in zip(("static_istio_percent", "opa_percent", "manual_percent", "greedy_percent"), vals):
            out.append(sub(f"{s} {c}", p, float(r[c])))
    ob = row(CW, scale="online-boutique-11-edges")
    out += [sub("OB QTPO", "99.3", float(ob["qtkg_percent"])), sub("OB low", "97.9", float(ob["qtkg_wilson_low"])),
            sub("OB high", "99.8", float(ob["qtkg_wilson_high"]))]
    return out


_T6 = {  # extended Table 6: QTPO, NoKG, OPP, ELCA, Static, ConfigProfile, SL-CA
    50: ("0.00", "3.07", "6.07", "17.27", "11.93", "56.60", "59.87"),
    100: ("0.00", "3.47", "4.72", "15.28", "10.20", "54.75", "58.25"),
    200: ("0.00", "3.19", "4.14", "14.81", "12.15", "51.07", "54.34"),
    500: ("0.00", "3.01", "5.56", "15.80", "12.03", "48.94", "52.03"),
    1000: ("0.00", "2.79", "4.03", "11.96", "9.02", "41.48", "44.30"),
    2000: ("0.00", "2.89", "5.34", "13.13", "9.04", "37.19", "40.15"),
}
for _s, _v in _T6.items():
    @claim(f"E03-{_s}", "EXT Table 6, CR Fig. 2(a)", f"unsafe rate on the Alibaba subgraph with {_s} services",
           " / ".join(_v), "[M]", DET, f"{RA}: scale={_s}, unsafe_promotion_pct", f"{MK} baseline-compare")
    def _(s=_s, v=_v):
        keys = ("QTKG", "NoKG", "OPP", "ELCA", "Static", "ConfigProfile", "SL-CA")
        return [sub(k, p, val(RA, "unsafe_promotion_pct", scale=str(s), method=k)) for k, p in zip(keys, v)]


@claim("E04", "EXT Table 6 note, Sect. 8.5", "top-5% fan-in share and degree statistics",
       "100/100/98.1/92.8/83.5/74.6% vs 52.6%, degree median 3 and max 2971", "", TOPOLOGY, "alibaba_<N>.edges",
       "none (committed input)")
def _():
    e = edges("full_16247svc")
    deg = Counter()
    for a, b in set(e):
        deg[a] += 1
        deg[b] += 1
    exp = ("100", "100", "98.1", "92.8", "83.5", "74.6")
    out = [sub(f"top 5% at {s}", p, top5_share(str(s))) for s, p in zip(SCALES_REAL, exp)]
    out += [sub("top 5% full trace", "52.6", top5_share("full_16247svc")),
            sub("median degree", "3", statistics.median(deg.values())), sub("max degree", "2971", max(deg.values()))]
    return out


@claim("E05", "EXT Table 7", "mass staleness rows 0, 5, 10, 25, 75, 90%", "C_decl, blocked-safe, unsafe", "[M]", DET,
       f"{ST}", f"{MK} staleness-sweep")
def _():
    exp = {"0.00": ("99.03", "1.00", "0.00", "0.00"), "0.05": ("94.16", "5.97", "0.00", "4.99"),
           "0.10": ("89.14", "10.89", "0.00", "9.97"), "0.25": ("74.43", "25.57", "0.00", "24.75"),
           "0.75": ("24.81", "75.20", "0.00", "74.35"), "0.90": ("9.87", "90.13", "0.00", "89.22")}
    out = []
    for f, (c, b, u, n) in exp.items():
        out += [sub(f"{f} C_decl", c, val(ST, "compliance_pct_median", stale_fraction=f, method="QTKG")),
                sub(f"{f} blocked", b, val(ST, "blocked_safe_pct_median", stale_fraction=f, method="QTKG")),
                sub(f"{f} QTPO unsafe", u, val(ST, "unsafe_promotion_pct", stale_fraction=f, method="QTKG")),
                sub(f"{f} NoKG unsafe", n, val(ST, "unsafe_promotion_pct", stale_fraction=f, method="NoKG"))]
    return out


@claim("E06", "EXT Sect. 8.5", "real 200-service topology: C_decl 100.00% to 9.91%, NoKG 89.20% at 90%",
       "100.00 / 9.91 / 89.20", "[M]", DET, f"{SR}", f"{MK} staleness-sweep")
def _():
    return [sub("C_decl 0%", "100.00", val(SR, "compliance_pct_median", stale_fraction="0.00", method="QTKG")),
            sub("C_decl 90%", "9.91", val(SR, "compliance_pct_median", stale_fraction="0.90", method="QTKG")),
            sub("NoKG 90%", "89.20", val(SR, "unsafe_promotion_pct", stale_fraction="0.90", method="NoKG"))]


@claim("E07", "EXT Tables 8, 9", "perturbation envelope and calibration bias", "6-15% / 9-19%, +7-9% CPU and +4-5% wire",
       "[P]", REPLAY, "setup-cost-perturbation.csv, oqs-calibration-bias.csv", f"{MK} paper-results")
def _():
    h = row("setup-cost-perturbation.csv", term="hybrid_setup_cost_percent")
    p = row("setup-cost-perturbation.csv", term="pure_pqc_setup_cost_percent")
    c = row("oqs-calibration-bias.csv", bias_term="cpu")
    w = row("oqs-calibration-bias.csv", bias_term="wire")
    return [sub("hybrid envelope", "6-15", f"{h['low_value']}-{h['high_value']}"),
            sub("pure envelope", "9-19", f"{p['low_value']}-{p['high_value']}"),
            sub("CPU bias", "7-9", f"{c['low_percent']}-{c['high_percent']}"),
            sub("wire bias", "4-5", f"{w['low_percent']}-{w['high_percent']}")]


@claim("E08", "EXT Table 10", "wire p50 per profile", "9.337 / 9.310 / 9.213 ms", "[M]", TIMING, f"{WT}: p50_ms",
       f"{MK} wire-tls")
def _():
    return [sub(m, p, val(WT, "p50_ms", profile=m))
            for m, p in (("classical_tls13", "9.337"), ("hybrid_mlkem", "9.310"), ("pure_pq", "9.213"))]


@claim("E09", "EXT Table 11, Fig. 3(b)", "H1 latency table", "12.0/18.0/4.0/11.4-12.7/4.1/2.8/12.0 ...", "[M]", REPLAY,
       f"{LT}: all columns", f"{MK} paper-results")
def _():
    exp = {"50-services": ("12.0", "18.0", "4.0", "11.4", "12.7", "4.1", "2.8", "12.0"),
           "100-services": ("24.0", "33.0", "7.0", "22.9", "25.2", "9.3", "5.9", "24.0"),
           "200-services": ("45.0", "61.0", "11.0", "43.1", "46.8", "18.6", "11.7", "45.0")}
    cols = ("median_ms", "p95_ms", "iqr_ms", "median_ci_low", "median_ci_high", "query_ms", "rules_ms", "controller_ms")
    return [sub(f"{s} {c}", p, val(LT, c, scale=s)) for s, vals in exp.items() for c, p in zip(cols, vals)]


@claim("E10", "EXT Sect. 8.7, Table A14", "controller footprint at 200 services (archived run)",
       "below 1 vCPU and 600 MB", "", REPLAY, "canonical record: latency.footprint_at_200_services", f"{MK} paper-results",
       note="Resolved 2026-09-25. The extended version used to list 0.62 vCPU / 410 MB and 0.48 vCPU / 530 MB, "
            "which are not in the artifact.")
def _():
    f = canonical()["latency"]["footprint_at_200_services"]
    return [sub("controller CPU", "<1", f["controller_cpu_vcpu"]), sub("controller RSS", "<600", f["controller_rss_mb"])]


@claim("E11", "EXT Table 12, Fig. 3(c)", "semantic-core stress timings", "1.484/0.016/1.540/2.832 ... 4.360/0.039/4.511/8.883",
       "[M]", TIMING, "scale-stress-results.csv", f"{MK} scale-stress")
def _():
    exp = {"500": ("1.484", "0.016", "1.540", "2.832"), "1000": ("2.450", "0.023", "2.535", "4.647"),
           "2000": ("4.360", "0.039", "4.511", "8.883")}
    cols = ("query_build_ms", "rule_filter_ms", "controller_est_ms", "alloc_mb")
    return [sub(f"{s} {c}", p, val("scale-stress-results.csv", c, scale=s)) for s, v in exp.items() for c, p in zip(cols, v)]


@claim("E12", "EXT Table 12", "semantic-core counts: edges, triples, promoted/blocked",
       "1000/6.4k/936/64, 2000/12.7k/1889/111, 4000/25.4k/3758/242", "[M]", DET, "scale-stress-results.csv",
       f"{MK} scale-stress")
def _():
    exp = {"500": ("1000", 6390, "936", "64"), "1000": ("2000", 12736, "1889", "111"), "2000": ("4000", 25432, "3758", "242")}
    out = []
    for s, (e, t, p, b) in exp.items():
        r = row("scale-stress-results.csv", scale=s)
        out += [sub(f"{s} edges", e, int(r["communication_edges"])), sub(f"{s} triples", str(t), int(r["triples"])),
                sub(f"{s} promoted", p, int(r["promoted_edges"])), sub(f"{s} blocked", b, int(r["blocked_edges"]))]
    return out


@claim("E13", "EXT Table 13", "fault injection: affected edges and unsafe / blocked / preserved per scenario",
       "stale 2: 0/2/0, contradictory 1: 0/1/0, outage 3: 0/3/1, mesh mismatch 2: 0/2/1, policy conflict 2: 0/2/1",
       "[M]", REPLAY, "fault-injection-results.csv", f"{MK} paper-results",
       note="Resolved 2026-09-25. The mesh-mismatch and policy-conflict rows used to read 1 and 0/1/1, 1 and 0/1/0.")
def _():
    exp = {"stale-fact": ("2", "0/2/0"), "contradictory-fact": ("1", "0/1/0"), "graph-outage": ("3", "0/3/1"),
           "mesh-mismatch": ("2", "0/2/1"), "policy-conflict": ("2", "0/2/1")}
    out = []
    for s, (a, t) in exp.items():
        r = row("fault-injection-results.csv", scenario=s)
        got = f"{r['unsafe_promotions']}/{r['blocked_edges']}/{r['preserved_active']}"
        out += [sub(f"{s} affected", a, int(r["affected_edges"])), sub(f"{s} u/b/p", t, got)]
    return out


@claim("E14", "EXT Table 14", "blocked-safe breakdown shares", "29/18/14/31/8/0%", "[M]", REPLAY,
       "blocked-safe-breakdown.csv", f"{MK} paper-results")
def _():
    exp = (("unsupported-endpoints", "29"), ("stale-facts", "18"), ("contradictory-facts", "14"),
           ("no-feasible-profile", "31"), ("policy-conflict", "8"), ("unsafe-promotions", "0"))
    return [sub(c, p, val("blocked-safe-breakdown.csv", "share_percent", category=c)) for c, p in exp]


@claim("E15", "EXT Table A10", "weight sweep rows", "96.47 / 0.00 / 20.48, 0.04, 0.00", "[M]", DET, WS, f"{MK} weight-sweep")
def _():
    exp = {"operating-point": "0.00", "w_o-50%": "20.48", "w_o+50%": "0.04", "w_r-50%": "0.04", "w_r+50%": "20.48",
           "w_m-50%": "0.00", "w_m+50%": "0.00", "w_h-50%": "0.00", "w_h+50%": "0.00", "delta-50%": "0.00",
           "delta+50%": "0.00", "delta=0": "0.00"}
    out = []
    for c, ch in exp.items():
        r = row(WS, config=c)
        out += [sub(f"{c} C_decl", "96.47", float(r["correct_promotion_pct"])),
                sub(f"{c} unsafe", "0.00", float(r["unsafe_promotion_pct"])),
                sub(f"{c} changed", ch, float(r["selection_changed_pct"]))]
    out.append(sub("active retained", "73.40", val(WS, "active_retained_pct", config="operating-point")))
    return out


@claim("E16", "EXT Sect. 8.8", "Online Boutique: C_decl, latency median/p95, unsafe, and NoKG/NoProv unsafe",
       "99.3%, 9 ms, 21 ms, 0, and 3.6% (12 of 330)", "untagged", REPLAY,
       "online-boutique-rq-stats.csv, online-boutique-ablation.csv", f"{MK} paper-results baseline-compare")
def _():
    r = rows("online-boutique-rq-stats.csv")[0]
    a = row("online-boutique-ablation.csv", method="NoKG")
    return [sub("C_decl", "99.3", float(r["compliance_percent_median"])), sub("median", "9", float(r["latency_ms_median"])),
            sub("p95", "21", float(r["latency_ms_p95"])), sub("unsafe", "0", int(r["unsafe_promotions"])),
            sub("NoKG unsafe %", "3.6", float(a["unsafe_promotion_pct"])),
            sub("NoKG unsafe n", "12", int(a["unsafe_promotions"])), sub("NoKG managed", "330", int(a["managed_edges"]))]


@claim("E17", "EXT Sect. 9", "case-study setup cost on Online Boutique", "9.4%", "[P]", REPLAY,
       "online-boutique-rq-stats.csv: setup_cost_percent", f"{MK} paper-results")
def _():
    return [sub("setup cost", "9.4", float(rows("online-boutique-rq-stats.csv")[0]["setup_cost_percent"]))]


@claim("E18", "EXT Sect. 8.4", "NoKG/NoProv invalid-promotion range over H2, Alibaba and Online Boutique",
       "2.5-3.6%", "untagged (measured)", DET, f"{BC}, {RA}, online-boutique-ablation.csv: NoKG", f"{MK} baseline-compare")
def _():
    v = [float(r["unsafe_promotion_pct"]) for f in (BC, RA, "online-boutique-ablation.csv") for r in rows(f)
         if r["method"] == "NoKG"]
    return [sub("min", "2.5", min(v)), sub("max", "3.6", max(v))]


@claim("E19", "EXT Table A3", "Online Boutique decision trace", "QuantumSafeProfile, score 0.4675, checkoutToPayment-mtls/-tls",
       "", DET, "online-boutique-decision-trace.json", f"{MK} decision-trace")
def _():
    t = json.loads((PROCESSED / "online-boutique-decision-trace.json").read_text(encoding="utf-8"))
    sel = [c for c in t["candidates"] if c["name"] == t["selected_profile"]][0]
    return [sub("selected", "QuantumSafeProfile", t["selected_profile"]), sub("score", "0.4675", float(sel["score"])),
            sub("required", "quantum_safe", t["required_class"]),
            sub("PeerAuthentication", "checkoutToPayment-mtls", t["peer_authentication_name"]),
            sub("DestinationRule", "checkoutToPayment-tls", t["destination_rule_name"])]


@claim("E20", "EXT Table A4", "data-model microbenchmark", "networkx 5.9/0.95/6.8, SQLite 7.1/2.5/9.7, rdflib 95.3/918.4/1013.8 ms",
       "untagged (measured)", TIMING, "datamodel-benchmark.csv", f"{MK} datamodel-benchmark")
def _():
    exp = {"property-graph-networkx": ("5.9", "0.95", "6.8"), "relational-sqlite": ("7.1", "2.5", "9.7"),
           "rdf-rdflib-7.6.0": ("95.3", "918.4", "1013.8")}
    out = []
    for e, (l, q, t) in exp.items():
        r = row("datamodel-benchmark.csv", engine=e)
        out += [sub(f"{e} load", l, float(r["load_ms"])), sub(f"{e} query", q, float(r["query_ms"])),
                sub(f"{e} total", t, float(r["total_ms"]))]
    return out


@claim("E21", "EXT Table A4", "all three encodings return the same admissible edges", "1889", "", DET,
       "datamodel-benchmark.csv: admissible_edges, reference_admissible", f"{MK} datamodel-benchmark")
def _():
    return [sub(r["engine"], "1889", int(r["admissible_edges"])) for r in rows("datamodel-benchmark.csv")] + \
           [sub("reference", "1889", int(rows("datamodel-benchmark.csv")[0]["reference_admissible"]))]


@claim("E22", "EXT Table A2", "controller scale/churn harness: unsafe promotions", "n/a (not run)", "", CODE,
       "controller-scale-results.csv, controller-churn-results.csv: status", f"{MK} controller-harness",
       note="Resolved 2026-09-25 (the table used to print 0). The 0 and the reconcile_p50_ms values 12/24/45 are "
            "still constants in run-controller-scale-churn.py, and the paper no longer reports them.")
def _():
    out = []
    for f in ("controller-scale-results.csv", "controller-churn-results.csv"):
        st = "+".join(sorted({x["status"] for x in rows(f)}))
        out.append(sub(f"{f} status", "not run", "not run" if st == "not-run-kind-unavailable" else st))
    return out


# ---------------------------------------------------------------- runner

def evaluate(c: dict, skip_timing: bool) -> tuple[str, list]:
    if c["status"] == "UNTRACED":
        return "UNTRACED", []
    if skip_timing and c["cls"] == TIMING:
        return "SKIP", []
    try:
        subs = c["fn"]()
    except (KeyError, FileNotFoundError, ValueError, IndexError) as exc:
        return "FAIL", [("error", "", str(exc), False)]
    ok = all(s[3] for s in subs)
    if c["status"] == "WARN":
        return "WARN", subs
    return ("PASS" if ok else "FAIL"), subs


def markdown() -> None:
    print("| ID | Paper location | Claim | Paper value | Paper tag | Evidence class | Result file and key | Command | Status | Note |")
    print("|---|---|---|---|---|---|---|---|---|---|")
    for c in CLAIMS:
        status, _ = evaluate(c, False)
        cells = [c["id"], c["where"], c["text"], c["paper"], c["tag"] or "none", c["cls"],
                 f"`{c['source']}`", f"`{c['command']}`" if c["command"] not in ("none", "code") else c["command"],
                 status, c["note"]]
        print("| " + " | ".join(str(x).replace("|", "/") for x in cells) + " |")


def main() -> None:
    global PROCESSED
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--processed", default=str(DEFAULT_PROCESSED),
                    help="folder with the result CSVs (default: the committed outputs/processed)")
    ap.add_argument("--skip-timing", action="store_true", help="skip host-dependent timing claims")
    ap.add_argument("--markdown", action="store_true", help="print the claim matrix as a Markdown table")
    ap.add_argument("--verbose", action="store_true", help="print every sub-check")
    args = ap.parse_args()
    PROCESSED = Path(args.processed).resolve()
    if args.markdown:
        markdown()
        return
    counts: Counter = Counter()
    nsub = 0
    for c in CLAIMS:
        status, subs = evaluate(c, args.skip_timing)
        counts[status] += 1
        nsub += len(subs)
        print(f"  [{status}] {c['id']} {c['where']}: {c['text']} (paper: {c['paper']})")
        for label, paper, actual, ok in subs:
            if args.verbose or not ok:
                print(f"         {'ok  ' if ok else 'DIFF'} {label}: paper {paper}, artifact {actual}")
        if c["note"] and status in ("WARN", "UNTRACED"):
            print(f"         note: {c['note']}")
    print(f"Claims: {len(CLAIMS)} ({nsub} values checked). " +
          ", ".join(f"{k} {counts[k]}" for k in ("PASS", "WARN", "UNTRACED", "SKIP", "FAIL")))
    print(f"Result folder: {PROCESSED.relative_to(ROOT) if PROCESSED.is_relative_to(ROOT) else PROCESSED}")
    sys.exit(1 if counts["FAIL"] else 0)


if __name__ == "__main__":
    main()
