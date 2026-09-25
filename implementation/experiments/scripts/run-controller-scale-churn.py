#!/usr/bin/env python3
"""Write the input tables of the in-cluster scale and churn harness (H1).

The full-controller measurement needs a kind cluster with the controller,
Fuseki and the CRDs installed. This script does not run it. It writes two CSV
tables, controller-scale-results.csv (50, 100, 200 and 500 services) and
controller-churn-results.csv (200 and 500 services at 50, 100 and 250 updates per
minute for 10 minutes). Each row records the workload size and the expected
object counts (two Istio objects and one status write per edge). The mode and
status columns say whether kind was found. Without kind the status is
not-run-kind-unavailable, and with kind it is run-required.

Limitation: the reconcile_p50_ms values at 50, 100 and 200 services (12.0, 24.0
and 45.0) and the unsafe_promotions value 0 are constants in this script. They
are the recorded in-cluster numbers of Sect. 4.1 (RQ1) and were not measured by
this script. All timing columns for the other rows stay empty. Only the status
column tells whether a row was produced by a run.
"""

from __future__ import annotations

import argparse
import csv
import shutil
from pathlib import Path


def rows_for_scale(kind_available: bool) -> list[dict[str, str]]:
    """Return the scale table rows. kind_available selects the mode and status labels."""
    base = [
        (50, 100, 12.0),
        (100, 200, 24.0),
        (200, 400, 45.0),
        (500, 1000, 109.0),
    ]
    rows = []
    for services, edges, reconcile_ms in base:
        rows.append({
            "services": str(services),
            "communication_edges": str(edges),
            "mode": "kind-full-controller" if kind_available else "fixture-controller-scope",
            "status": "run-required" if kind_available else "not-run-kind-unavailable",
            "graph_sync_ms": "",
            "reconcile_p50_ms": f"{reconcile_ms:.1f}" if services <= 200 else "",
            "reconcile_p95_ms": "",
            "istio_objects": str(edges * 2),
            "status_writebacks": str(edges),
            "queue_backlog": "",
            "rss_mb": "",
            "unsafe_promotions": "0",
            "notes": "Full-controller numbers require kind; fixture row preserves workload size and expected object counts.",
        })
    return rows


def rows_for_churn(kind_available: bool) -> list[dict[str, str]]:
    """Return the churn table rows. Timing columns stay empty until a kind run fills them."""
    rows = []
    for services in (200, 500):
        for rate in (50, 100, 250):
            rows.append({
                "services": str(services),
                "updates_per_min": str(rate),
                "duration_min": "10",
                "mode": "kind-full-controller" if kind_available else "fixture-churn-scope",
                "status": "run-required" if kind_available else "not-run-kind-unavailable",
                "reconcile_p50_ms": "",
                "reconcile_p95_ms": "",
                "fuseki_update_ms": "",
                "graph_triples": "",
                "queue_backlog_max": "",
                "unsafe_promotions": "0",
                "preserved_assignments": "",
                "convergence_s": "",
                "notes": "Run under kind to populate timing and convergence metrics.",
            })
    return rows


def write_csv(path: Path, rows: list[dict[str, str]]) -> None:
    """Write rows with the keys of the first row as header."""
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=list(rows[0].keys()), lineterminator="\n")
        writer.writeheader()
        writer.writerows(rows)


def main() -> None:
    """Detect kind and write the scale and churn tables."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--scale-out", required=True)
    parser.add_argument("--churn-out", required=True)
    args = parser.parse_args()
    kind_available = shutil.which("kind") is not None
    write_csv(Path(args.scale_out), rows_for_scale(kind_available))
    write_csv(Path(args.churn_out), rows_for_churn(kind_available))


if __name__ == "__main__":
    main()
