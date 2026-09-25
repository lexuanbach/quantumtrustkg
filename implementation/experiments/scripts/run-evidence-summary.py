#!/usr/bin/env python3
"""Run the artifact checks and write a status summary (evidence-summary.json and
evidence-summary.md).

The summary is a status report and not a results table. It runs eight steps, and
each ends as passed, failed, skipped or unavailable:

  controller_tests                 go test for the controller, including the tests
                                   for the gates behind G1 to G4
  scale_stress_csv                 cmd/scale-stress on the 500- and 1000-service
                                   fixtures (extended version, H3)
  online_boutique_trace            regenerates the fixture and the decision trace
                                   of Checkout to Payment
  pq_tls_availability              run-pq-tls-benchmark.py
  wire_tls_validation              run-wire-tls-validation.py
  controller_scale_churn_harness   run-controller-scale-churn.py (needs kind)
  coq_rocq_checks                  make -C formal/coq (needs Rocq 9.x)
  paper_results_canonical_replay   run-paper-results.py

A step whose tool is missing is marked unavailable or skipped. A missing
dependency is never converted into a measurement. The script exits with status 1
only if a step failed. It does not run the H2 harnesses of Table 1 or the
staleness sweep. Those are separate Makefile targets (baseline-compare,
staleness-sweep). Output: outputs/processed/evidence-summary.json and .md.
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import shutil
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[3]
PROCESSED = ROOT / "implementation" / "experiments" / "outputs" / "processed"


def display_command(command: list[str]) -> str:
    """Return the command as text with the interpreter and the root path shortened."""
    text = " ".join(command)
    text = text.replace(sys.executable, "python3")
    return text.replace(str(ROOT) + "/", "")


def run_command(
    name: str,
    command: list[str],
    *,
    cwd: Path = ROOT,
    timeout: int = 180,
    missing: str | None = None,
) -> dict[str, Any]:
    """Run one step and return its record (name, status, command, duration, return code, note, output tail).
    If the tool named in missing is not on PATH the step is unavailable, and a timeout is a failure.
    """
    if missing is not None and shutil.which(missing) is None:
        return {
            "name": name,
            "status": "unavailable",
            "command": display_command(command),
            "duration_s": 0.0,
            "returncode": None,
            "note": f"{missing} is not available",
            "output_tail": "",
        }

    start = time.monotonic()
    try:
        proc = subprocess.run(
            command,
            cwd=cwd,
            text=True,
            capture_output=True,
            timeout=timeout,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        return {
            "name": name,
            "status": "failed",
            "command": display_command(command),
            "duration_s": round(time.monotonic() - start, 3),
            "returncode": None,
            "note": f"timed out after {timeout}s",
            "output_tail": (exc.stdout or "")[-1200:] + (exc.stderr or "")[-1200:],
        }

    output = (proc.stdout + proc.stderr).strip()
    return {
        "name": name,
        "status": "passed" if proc.returncode == 0 else "failed",
        "command": display_command(command),
        "duration_s": round(time.monotonic() - start, 3),
        "returncode": proc.returncode,
        "note": "",
        "output_tail": output[-1600:],
    }


def read_csv(path: Path) -> list[dict[str, str]]:
    """Return the rows of a CSV file, or an empty list if it does not exist."""
    if not path.exists():
        return []
    with path.open(newline="", encoding="utf-8") as fh:
        return list(csv.DictReader(fh))


def count_fixture_items(path: Path) -> tuple[int, int]:
    """Count the service and communication declarations in a Turtle fixture."""
    text = path.read_text(encoding="utf-8")
    services = text.count(" a :Service ;")
    edges = text.count(" a :Communication ;")
    return services, edges


def add_artifacts(step: dict[str, Any], *paths: Path) -> dict[str, Any]:
    """Attach the paths of the existing output files to a step record."""
    step["artifacts"] = [str(path.relative_to(ROOT)) for path in paths if path.exists()]
    return step


def summarize_scale_stress(step: dict[str, Any], out: Path) -> dict[str, Any]:
    """Add the row count and scales of the scale-stress CSV to the step."""
    rows = read_csv(out)
    if rows:
        step["rows"] = len(rows)
        step["scales"] = [row.get("scale", "") for row in rows]
    return add_artifacts(step, out)


def summarize_trace(step: dict[str, Any], fixture: Path, trace_out: Path) -> dict[str, Any]:
    """Add the fixture size and the traced edge with its selected profile to the step."""
    if fixture.exists():
        services, edges = count_fixture_items(fixture)
        step["fixture_services"] = services
        step["fixture_edges"] = edges
    if trace_out.exists():
        trace = json.loads(trace_out.read_text(encoding="utf-8"))
        step["trace_edge"] = f"{trace.get('source_service')}->{trace.get('destination_service')}"
        step["selected_profile"] = trace.get("selected_profile")
    return add_artifacts(step, fixture, trace_out)


def summarize_pq_tls(step: dict[str, Any], out: Path) -> dict[str, Any]:
    """Mark the step unavailable when any group row says so."""
    rows = read_csv(out)
    if rows:
        statuses = sorted({row.get("status", "") for row in rows})
        step["tls_statuses"] = statuses
        if any(status.startswith("unavailable") for status in statuses):
            step["status"] = "unavailable"
            step["note"] = "one or more OpenSSL/OQS groups are unavailable"
    return add_artifacts(step, out)


def summarize_wire_tls(step: dict[str, Any], out: Path) -> dict[str, Any]:
    """Derive the step status from the wire-validation rows.
    All unavailable gives unavailable, any failed handshake gives failed, and partial availability adds a note.
    """
    rows = read_csv(out)
    if rows:
        statuses = sorted({row.get("status", "") for row in rows})
        negotiated = sorted({row.get("negotiated_group", "") for row in rows if row.get("negotiated_group", "")})
        step["wire_tls_statuses"] = statuses
        step["negotiated_groups"] = negotiated
        if all(status.startswith("unavailable") for status in statuses):
            step["status"] = "unavailable"
            step["note"] = "native TLS wire validation is unavailable in this environment"
        elif any(status.startswith("failed") for status in statuses):
            step["status"] = "failed"
            step["note"] = "one or more native TLS handshakes failed"
        elif any(status.startswith("unavailable") for status in statuses):
            step["note"] = "some native TLS groups are unavailable"
    return add_artifacts(step, out)


def summarize_controller_harness(step: dict[str, Any], scale_out: Path, churn_out: Path) -> dict[str, Any]:
    """Mark the step skipped when the harness rows say that kind was unavailable."""
    statuses = set()
    for path in (scale_out, churn_out):
        for row in read_csv(path):
            statuses.add(row.get("status", ""))
    step["harness_statuses"] = sorted(statuses)
    if any("kind-unavailable" in status for status in statuses):
        step["status"] = "skipped"
        step["note"] = "kind is unavailable, so full-controller timing rows were not run"
    return add_artifacts(step, scale_out, churn_out)


def coq_compiler() -> str:
    """Return the Coq compiler command: the COQC variable, else "rocq compile", else "coqc"."""
    if os.environ.get("COQC"):
        return os.environ["COQC"]
    if shutil.which("rocq"):
        return "rocq compile"
    return "coqc"


def coq_version() -> str:
    """Return the first line of the compiler's --version output, or "" if it cannot run."""
    try:
        out = subprocess.run(
            coq_compiler().split() + ["--version"], capture_output=True, text=True, timeout=30
        )
    except (OSError, subprocess.SubprocessError):
        return ""
    return (out.stdout or out.stderr).strip().splitlines()[0] if (out.stdout or out.stderr).strip() else ""


def summarize_coq(step: dict[str, Any]) -> dict[str, Any]:
    """Mark the Coq step skipped when no Rocq 9 compiler is available."""
    if shutil.which(coq_compiler().split()[0]) is None:
        step["status"] = "skipped"
        step["note"] = "neither rocq nor coqc is available"
        return step
    version = coq_version()
    if "version 8." in version:
        # The development imports Stdlib and needs Rocq 9.x. A version 8 compiler
        # counts as a missing tool and does not count as a failed proof.
        step["status"] = "skipped"
        step["note"] = (
            f"unavailable: found '{version}'; the proofs need Rocq 9.x "
            '(set COQC="rocq compile")'
        )
    return step


def summarize_paper_results(step: dict[str, Any], out_dir: Path) -> dict[str, Any]:
    """List the files written by the canonical replay."""
    artifacts = [
        out_dir / "fault-injection-results.csv",
        out_dir / "blocked-safe-breakdown.csv",
        out_dir / "compliance-wilson.csv",
        out_dir / "latency-results.csv",
        out_dir / "setup-cost-results.csv",
        out_dir / "setup-cost-perturbation.csv",
        out_dir / "oqs-calibration-bias.csv",
        out_dir / "online-boutique-rq-stats.csv",
        out_dir / "paper-results-summary.md",
    ]
    return add_artifacts(step, *artifacts)


def write_outputs(summary: dict[str, Any], json_path: Path, markdown_path: Path) -> None:
    """Write the JSON summary and the Markdown table."""
    json_path.parent.mkdir(parents=True, exist_ok=True)
    json_path.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8")

    lines = [
        "# QuantumTrustKG Evidence Summary",
        "",
        f"Generated: {summary['generated_at']}",
        "",
        "| Step | Status | Note | Artifacts |",
        "|---|---|---|---|",
    ]
    for step in summary["steps"]:
        artifacts = ", ".join(step.get("artifacts", []))
        lines.append(
            f"| {step['name']} | {step['status']} | {step.get('note', '')} | {artifacts} |"
        )
    lines.extend(
        [
            "",
            "Skipped or unavailable rows indicate missing local support such as kind,",
            "OpenSSL/OQS groups, optional liboqs/Envoy images, or Coq/Rocq.",
            "They are not extrapolated measurements.",
            "",
        ]
    )
    markdown_path.write_text("\n".join(lines), encoding="utf-8")


def main() -> None:
    """Run all steps in order, write the summary and exit with status 1 if any step failed."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", default=str(PROCESSED / "evidence-summary.json"))
    parser.add_argument("--markdown-out", default=str(PROCESSED / "evidence-summary.md"))
    args = parser.parse_args()

    scale_out = PROCESSED / "scale-stress-results.csv"
    trace_out = PROCESSED / "online-boutique-decision-trace.json"
    pq_out = PROCESSED / "pq-tls-benchmark-results.csv"
    wire_tls_out = PROCESSED / "wire-tls-validation.csv"
    controller_scale = PROCESSED / "controller-scale-results.csv"
    controller_churn = PROCESSED / "controller-churn-results.csv"
    boutique_fixture = ROOT / "implementation" / "semantics" / "fixtures" / "online-boutique.ttl"

    steps: list[dict[str, Any]] = []

    steps.append(run_command("controller_tests", ["make", "-C", "implementation/controller", "test"], missing="go"))
    steps.append(
        summarize_scale_stress(
            run_command(
                "scale_stress_csv",
                [
                    "go",
                    "run",
                    "./cmd/scale-stress",
                    "--fixtures",
                    "../semantics/fixtures/scale-500.ttl,../semantics/fixtures/scale-1000.ttl,../semantics/fixtures/scale-2000.ttl",
                    "--iterations",
                    "7",
                    "--out",
                    "../experiments/outputs/processed/scale-stress-results.csv",
                ],
                cwd=ROOT / "implementation" / "controller",
                missing="go",
            ),
            scale_out,
        )
    )
    trace_step = run_command(
        "online_boutique_trace",
        [
            "sh",
            "-c",
            (
                f"{sys.executable} ../tools/generators/generate_online_boutique_fixture.py "
                "--out ../semantics/fixtures/online-boutique.ttl && "
                "go run ./cmd/decision-trace --fixture online-boutique.ttl "
                "--source Checkout --destination Payment "
                "--out ../experiments/outputs/processed/online-boutique-decision-trace.json"
            ),
        ],
        cwd=ROOT / "implementation" / "controller",
        missing="go",
    )
    steps.append(summarize_trace(trace_step, boutique_fixture, trace_out))
    steps.append(
        summarize_pq_tls(
            run_command(
                "pq_tls_availability",
                [
                    sys.executable,
                    "implementation/experiments/scripts/run-pq-tls-benchmark.py",
                    "--out",
                    str(pq_out.relative_to(ROOT)),
                ],
            ),
            pq_out,
        )
    )
    steps.append(
        summarize_wire_tls(
            run_command(
                "wire_tls_validation",
                [
                    sys.executable,
                    "implementation/experiments/scripts/run-wire-tls-validation.py",
                    "--out",
                    str(wire_tls_out.relative_to(ROOT)),
                ],
            ),
            wire_tls_out,
        )
    )
    steps.append(
        summarize_controller_harness(
            run_command(
                "controller_scale_churn_harness",
                [
                    sys.executable,
                    "implementation/experiments/scripts/run-controller-scale-churn.py",
                    "--scale-out",
                    str(controller_scale.relative_to(ROOT)),
                    "--churn-out",
                    str(controller_churn.relative_to(ROOT)),
                ],
            ),
            controller_scale,
            controller_churn,
        )
    )
    steps.append(
        summarize_coq(
            run_command(
                "coq_rocq_checks",
                ["make", "-C", "formal/coq", f"COQC={coq_compiler()}"],
                timeout=120,
                missing=coq_compiler().split()[0],
            )
        )
    )
    steps.append(
        summarize_paper_results(
            run_command(
                "paper_results_canonical_replay",
                [
                    sys.executable,
                    "implementation/experiments/scripts/run-paper-results.py",
                ],
            ),
            PROCESSED,
        )
    )

    counts: dict[str, int] = {}
    for step in steps:
        counts[step["status"]] = counts.get(step["status"], 0) + 1

    summary = {
        "generated_at": datetime.now(timezone.utc).replace(microsecond=0).isoformat(),
        "repo_root": ".",
        "summary": counts,
        "steps": steps,
    }
    write_outputs(summary, Path(args.out), Path(args.markdown_out))

    if counts.get("failed", 0):
        sys.exit(1)


if __name__ == "__main__":
    main()
