#!/usr/bin/env python3
"""Print the result tables of the paper from the result CSV files.

Camera-ready Table 1 (H2 comparison, 200-service meshes) is printed in the
paper's layout and rounding. With --extended the script also prints extended
Table 6 (unsafe rate on the Alibaba subgraphs) and extended Table 7 (mass
staleness). The values come only from the CSV files in the result folder.

Usage:
  python3 scripts/make_tables.py [--processed DIR] [--extended]
"""
from __future__ import annotations

import argparse
import csv
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULT = ROOT / "implementation/experiments/outputs/processed"

# Paper column order of Table 1 and the method keys used in the CSV files.
T1 = [("QTPO", "QTKG"), ("NKG/NP", "NoKG"), ("OPP", "OPP"), ("ConfigProf.", "ConfigProfile"),
      ("Static", "Static"), ("ELCA", "ELCA"), ("SL-CA", "SL-CA")]
T6 = [("QTPO", "QTKG"), ("NoKG/NoProv", "NoKG"), ("OPP", "OPP"), ("ELCA", "ELCA"), ("Static", "Static"),
      ("ConfigProf.", "ConfigProfile"), ("SL-CA", "SL-CA")]


def read(folder: Path, name: str) -> list[dict[str, str]]:
    with (folder / name).open(newline="", encoding="utf-8") as fh:
        return list(csv.DictReader(fh))


def half_up(x: float, nd: int) -> str:
    """Round half up to nd decimals."""
    q = 10 ** nd
    return f"{int(x * q + 0.5 + 1e-9) / q:.{nd}f}"


def table1(folder: Path) -> None:
    rows = {r["method"]: r for r in read(folder, "baseline-comparison.csv") if r["scale"] == "200"}
    print("Camera-ready Table 1: H2, 200-service meshes, 30 reps, seed 20260509")
    print(f"{'':20}" + "".join(f"{label:>13}" for label, _ in T1))
    # Valid (%) is the median over repetitions (CSV column). Invalid (%) is pooled and is
    # computed from the counts, because the CSV column is already rounded to two decimals.
    valid = [half_up(float(rows[k]["correct_promotion_pct"]), 1) for _, k in T1]
    invalid = [half_up(100 * float(rows[k]["invalid_promotions"]) / float(rows[k]["managed_edges"]), 1) for _, k in T1]
    print(f"{'Valid (%)':20}" + "".join(f"{c:>13}" for c in valid))
    print(f"{'Invalid (%)':20}" + "".join(f"{c:>13}" for c in invalid))
    for label, col in (("Invalid n", "invalid_promotions"), ("No promotion n", "no_promotion"),
                       ("Retained-active n", "retained_active")):
        print(f"{label:20}" + "".join(f"{rows[k][col]:>13}" for _, k in T1))
    pooled = {rows[k]["managed_edges"] for _, k in T1}
    reps = {rows[k]["reps"] for _, k in T1}
    print(f"{'Pooled observations':20}{', '.join(sorted(pooled))} ({', '.join(sorted(reps))} repetitions)")
    print("Valid (%) is the median over repetitions. Invalid (%) is pooled over repetitions.")


def table6(folder: Path) -> None:
    data = read(folder, "baseline-comparison-real-alibaba.csv")
    scales = sorted({int(r["scale"]) for r in data})
    print("\nExtended Table 6: unsafe promotion rate (%) on the Alibaba subgraphs")
    print(f"{'Services':>8}{'Pooled':>9}" + "".join(f"{label:>13}" for label, _ in T6))
    for s in scales:
        by = {r["method"]: r for r in data if int(r["scale"]) == s}
        print(f"{s:>8}{by['QTKG']['managed_edges']:>9}" + "".join(f"{by[k]['unsafe_promotion_pct']:>13}" for _, k in T6))


def table7(folder: Path) -> None:
    data = read(folder, "staleness-sweep-results.csv")
    print("\nExtended Table 7: mass staleness, 200-service mesh (the paper omits the 50% row)")
    print(f"{'Stale':>6}{'QTPO C_decl':>13}{'QTPO blk-safe':>15}{'QTPO unsafe':>13}{'NoKG unsafe':>13}")
    for f in sorted({r["stale_fraction"] for r in data}, key=float):
        q = next(r for r in data if r["stale_fraction"] == f and r["method"] == "QTKG")
        n = next(r for r in data if r["stale_fraction"] == f and r["method"] == "NoKG")
        print(f"{round(100 * float(f)):>5}%{q['compliance_pct_median']:>13}{q['blocked_safe_pct_median']:>15}"
              f"{q['unsafe_promotion_pct']:>13}{n['unsafe_promotion_pct']:>13}")


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--processed", default=str(DEFAULT), help="folder with the result CSV files")
    ap.add_argument("--extended", action="store_true", help="also print extended Tables 6 and 7")
    args = ap.parse_args()
    folder = Path(args.processed)
    table1(folder)
    if args.extended:
        table6(folder)
        table7(folder)


if __name__ == "__main__":
    main()
