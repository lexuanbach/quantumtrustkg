#!/usr/bin/env python3
"""Replay the recorded in-cluster (H1) and modelled numbers of the paper as CSV.

This is a canonical-replay exporter and not a reproduction. It reads
experiments/canonical/paper-canonical-results.json and writes one CSV per result
table so that readers can diff the artifact against the paper. It re-emits
recorded values and does not re-measure them. The artifact holds neither the raw
per-repetition H1 logs nor an automated in-cluster driver, which means these numbers
cannot be regenerated from it. The paper says the same for H1: its in-cluster
figures are re-emitted from a recorded run of an earlier release (Sect. 4).

What the replay covers, with the result file each part writes into
experiments/outputs/processed/ (all are [M] or [P] values of Sect. 4):

  compliance-wilson.csv         in-cluster compliance with Wilson intervals (RQ1)
  latency-results.csv           selection latency per scale (Sect. 4.1 quotes the medians)
  fault-injection-results.csv   failure-injection scenarios (extended version)
  blocked-safe-breakdown.csv    reasons for blocked-safe edges (extended version)
  setup-cost-results.csv        modelled setup-cost and measured rollout ranges (RQ3)
  setup-cost-perturbation.csv   the +-25% envelope of the cost model (RQ3)
  oqs-calibration-bias.csv      bias terms of the cost calibration (RQ3)
  online-boutique-rq-stats.csv  statistics of the Online Boutique example
  paper-results-summary.md      an index of the files above

Everything the artifact can regenerate (H2, the weight sweep, H3, H4 and the
decision trace) comes from the Go commands and the other scripts. This file
does not produce them. The compliance metric in these files is the declared-label compliance of
Sect. 4. It scores control-plane decisions and not negotiated cryptography.
"""

from __future__ import annotations

import argparse
import csv
import json
from pathlib import Path
from typing import Any, Iterable


ROOT = Path(__file__).resolve().parents[3]
CANONICAL = ROOT / "implementation" / "experiments" / "canonical" / "paper-canonical-results.json"
PROCESSED = ROOT / "implementation" / "experiments" / "outputs" / "processed"


def write_csv(path: Path, fieldnames: list[str], rows: Iterable[dict[str, Any]]) -> int:
    """Write rows to path with the given column order and return the row count."""
    path.parent.mkdir(parents=True, exist_ok=True)
    count = 0
    with path.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=fieldnames, lineterminator="\n")
        writer.writeheader()
        for row in rows:
            writer.writerow(row)
            count += 1
    return count


def emit_fault_injection(data: dict[str, Any], out: Path) -> int:
    """Write the failure-injection scenarios (extended version)."""
    fields = [
        "scenario",
        "fault_description",
        "affected_edges",
        "unsafe_promotions",
        "blocked_edges",
        "preserved_active",
        "recovery",
        "status_reason",
    ]
    return write_csv(out, fields, data["fault_injection"]["scenarios"])


def emit_blocked_safe_breakdown(data: dict[str, Any], out: Path) -> int:
    """Write the share of each blocked-safe reason at 200 services (extended version)."""
    fields = ["category", "share_percent", "status_reason", "remediation"]
    return write_csv(out, fields, data["blocked_safe_breakdown_200_services"]["categories"])


def emit_compliance_wilson(data: dict[str, Any], out: Path) -> int:
    """Write in-cluster compliance per scale with Wilson intervals for QTPO and QTPO-NoKG (RQ1)."""
    fields = [
        "scale",
        "qtkg_percent",
        "qtkg_wilson_low",
        "qtkg_wilson_high",
        "qtpo_nokg_percent",
        "qtpo_nokg_wilson_low",
        "qtpo_nokg_wilson_high",
        "static_istio_percent",
        "opa_percent",
        "manual_percent",
        "greedy_percent",
    ]
    return write_csv(out, fields, data["compliance_wilson"]["rows"])


def emit_latency(data: dict[str, Any], out: Path) -> int:
    """Write median, p95 and stage timings of the selection latency per scale."""
    fields = [
        "scale",
        "median_ms",
        "p95_ms",
        "iqr_ms",
        "median_ci_low",
        "median_ci_high",
        "query_ms",
        "rules_ms",
        "controller_ms",
    ]
    return write_csv(out, fields, data["latency"]["rows"])


def emit_setup_cost(data: dict[str, Any], out: Path) -> int:
    """Write the setup-cost ranges and the rollout time, failure and rollback rates (RQ3)."""
    fields = [
        "mode",
        "setup_cost_percent_low",
        "setup_cost_percent_high",
        "rollout_time_min_low",
        "rollout_time_min_high",
        "fail_rate_percent",
        "rollback_rate_percent",
    ]
    return write_csv(out, fields, data["setup_cost"]["rows"])


def emit_perturbation(data: dict[str, Any], out: Path) -> int:
    """Write the +-25% perturbation of the calibrated setup-cost terms (RQ3)."""
    fields = [
        "term",
        "low_value",
        "high_value",
        "compliance_unchanged",
        "controller_latency_unchanged",
        "note",
    ]
    perturbation = data["setup_cost"]["perturbation_plus_minus_25_percent"]
    rows = [
        {
            "term": "hybrid_setup_cost_percent",
            "low_value": perturbation["hybrid_range_percent"][0],
            "high_value": perturbation["hybrid_range_percent"][1],
            "compliance_unchanged": perturbation["compliance_unchanged"],
            "controller_latency_unchanged": perturbation["controller_latency_unchanged"],
            "note": "+/-25% on calibrated CPU/wire terms",
        },
        {
            "term": "pure_pqc_setup_cost_percent",
            "low_value": perturbation["pure_pqc_range_percent"][0],
            "high_value": perturbation["pure_pqc_range_percent"][1],
            "compliance_unchanged": perturbation["compliance_unchanged"],
            "controller_latency_unchanged": perturbation["controller_latency_unchanged"],
            "note": "+/-25% on calibrated CPU/wire terms",
        },
    ]
    return write_csv(out, fields, rows)


def emit_oqs_calibration(data: dict[str, Any], out: Path) -> int:
    """Write the CPU and wire bias terms of the cost calibration (RQ3)."""
    fields = ["bias_term", "low_percent", "high_percent", "non_optimistic", "mesh_scale_extrapolation"]
    bias = data["setup_cost"]["oqs_calibration_bias"]
    rows = [
        {
            "bias_term": "cpu",
            "low_percent": bias["cpu_positive_bias_percent_low"],
            "high_percent": bias["cpu_positive_bias_percent_high"],
            "non_optimistic": bias["non_optimistic"],
            "mesh_scale_extrapolation": bias["mesh_scale_extrapolation"],
        },
        {
            "bias_term": "wire",
            "low_percent": bias["wire_positive_bias_percent_low"],
            "high_percent": bias["wire_positive_bias_percent_high"],
            "non_optimistic": bias["non_optimistic"],
            "mesh_scale_extrapolation": bias["mesh_scale_extrapolation"],
        },
    ]
    return write_csv(out, fields, rows)


def emit_online_boutique_rq(data: dict[str, Any], out: Path) -> int:
    """Write the one-row statistics of the Online Boutique example, without the description field."""
    ob = data["online_boutique_rq_stats"]
    fields = list(ob.keys())
    rows = [{k: ob[k] for k in fields}]
    fields = [k for k in fields if k != "description"]
    rows[0].pop("description", None)
    return write_csv(out, fields, rows)


def emit_summary_markdown(
    data: dict[str, Any], emitted: dict[str, tuple[Path, int]], out: Path
) -> None:
    """Write the index that lists each CSV together with the manuscript anchor it mirrors."""
    lines = [
        "# Paper-Cited Results: Canonical Replay",
        "",
        f"Source: `{CANONICAL.relative_to(ROOT)}`",
        "",
        f"Manuscript anchor: {data['section_anchor']}",
        "",
        "Each row below names a CSV produced by this exporter and the manuscript",
        "table or paragraph the CSV mirrors. The values are re-emitted from the",
        "canonical record, not re-measured: the artifact contains neither the raw",
        "per-repetition in-cluster logs nor an automated in-cluster driver.",
        "",
        "| Output | Rows | Manuscript anchor |",
        "|---|---|---|",
        f"| {emitted['fault_injection'][0].relative_to(ROOT)} | {emitted['fault_injection'][1]} | section 8 'Additional Evaluation' + appendix Table failure-injection |",
        f"| {emitted['blocked_safe'][0].relative_to(ROOT)} | {emitted['blocked_safe'][1]} | section 8 'Additional Evaluation' + appendix Table blocked-breakdown |",
        f"| {emitted['compliance'][0].relative_to(ROOT)} | {emitted['compliance'][1]} | section 8.1 RQ1 + appendix Table compliance-results |",
        f"| {emitted['latency'][0].relative_to(ROOT)} | {emitted['latency'][1]} | section 8.2 RQ2 + appendix Table latency-results |",
        f"| {emitted['setup_cost'][0].relative_to(ROOT)} | {emitted['setup_cost'][1]} | section 8.3 RQ3 + appendix Table migration-results |",
        f"| {emitted['perturbation'][0].relative_to(ROOT)} | {emitted['perturbation'][1]} | section 8.3 RQ3 +/-25% perturbation paragraph |",
        f"| {emitted['oqs_calibration'][0].relative_to(ROOT)} | {emitted['oqs_calibration'][1]} | section 8.3 RQ3 OQS-runner bias paragraph |",
        f"| {emitted['online_boutique'][0].relative_to(ROOT)} | {emitted['online_boutique'][1]} | section 8 'Additional Evaluation' Online Boutique paragraph |",
        "",
        "The weight/hysteresis sweep is regenerated by the Go harness",
        "(`baseline-compare --scenario weight-sweep`), not replayed here.",
        "",
    ]
    out.write_text("\n".join(lines), encoding="utf-8")


def main() -> None:
    """Read the canonical JSON, write all CSV files and the summary index."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--canonical", default=str(CANONICAL))
    parser.add_argument("--out-dir", default=str(PROCESSED))
    args = parser.parse_args()

    canonical = Path(args.canonical)
    out_dir = Path(args.out_dir)

    data = json.loads(canonical.read_text(encoding="utf-8"))

    emitted: dict[str, tuple[Path, int]] = {}
    emitted["fault_injection"] = (
        out_dir / "fault-injection-results.csv",
        emit_fault_injection(data, out_dir / "fault-injection-results.csv"),
    )
    emitted["blocked_safe"] = (
        out_dir / "blocked-safe-breakdown.csv",
        emit_blocked_safe_breakdown(data, out_dir / "blocked-safe-breakdown.csv"),
    )
    emitted["compliance"] = (
        out_dir / "compliance-wilson.csv",
        emit_compliance_wilson(data, out_dir / "compliance-wilson.csv"),
    )
    emitted["latency"] = (
        out_dir / "latency-results.csv",
        emit_latency(data, out_dir / "latency-results.csv"),
    )
    emitted["setup_cost"] = (
        out_dir / "setup-cost-results.csv",
        emit_setup_cost(data, out_dir / "setup-cost-results.csv"),
    )
    emitted["perturbation"] = (
        out_dir / "setup-cost-perturbation.csv",
        emit_perturbation(data, out_dir / "setup-cost-perturbation.csv"),
    )
    emitted["oqs_calibration"] = (
        out_dir / "oqs-calibration-bias.csv",
        emit_oqs_calibration(data, out_dir / "oqs-calibration-bias.csv"),
    )
    emitted["online_boutique"] = (
        out_dir / "online-boutique-rq-stats.csv",
        emit_online_boutique_rq(data, out_dir / "online-boutique-rq-stats.csv"),
    )

    emit_summary_markdown(data, emitted, out_dir / "paper-results-summary.md")


if __name__ == "__main__":
    main()
