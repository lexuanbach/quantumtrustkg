#!/usr/bin/env python3
"""Record whether the local OpenSSL exposes the groups needed for the TLS cost
measurement of RQ3 (Sect. 4.3).

The script checks for three groups: X25519 (classical TLS 1.3), X25519MLKEM768
(hybrid) and MLKEM768 (pure post-quantum). For each it writes one row with the
status available-run-required, or unavailable-* when OpenSSL, the OQS provider
or the group is missing. It measures nothing itself, which means the timing columns stay
empty. If a group is missing it emits an unavailable row and never a made-up
number.

Output: pq-tls-benchmark-results.csv, which run-evidence-summary.py reads as the
pq_tls_availability step. The handshake timings quoted in the paper (300
handshakes per group on OpenSSL 3.6.2) are in pq-tls-handshake-results.csv, which
the Makefile target pq-handshake writes with run-pq-tls-handshake.py. That script
is not in this directory. The end-to-end trials with a negotiated-group check
(30 per profile) are written by run-wire-tls-validation.py.
"""

from __future__ import annotations

import argparse
import csv
import shutil
import subprocess
from pathlib import Path


GROUPS = [
    ("classical_tls13", "X25519"),
    ("hybrid_mlkem", "X25519MLKEM768"),
    ("pure_pq", "MLKEM768"),
]


def run(cmd: list[str]) -> tuple[int, str]:
    """Run a command and return its exit code and combined output. Code 127 means the binary is missing."""
    try:
        out = subprocess.run(cmd, check=False, text=True, capture_output=True)
    except FileNotFoundError:
        return 127, ""
    return out.returncode, (out.stdout + out.stderr).strip()


def openssl_version() -> str:
    """Return the first line of openssl version, or openssl-not-found."""
    code, out = run(["openssl", "version"])
    return out.splitlines()[0] if code == 0 and out else "openssl-not-found"


def provider_summary() -> str:
    """Return the loaded OpenSSL providers joined by semicolons, or unavailable."""
    code, out = run(["openssl", "list", "-providers"])
    if code != 0:
        return "unavailable"
    providers = []
    for line in out.splitlines():
        line = line.strip()
        if line and not line.startswith("Providers:") and ":" not in line:
            providers.append(line.split()[0])
    return ";".join(providers) if providers else "unknown"


def supported_groups() -> set[str]:
    """Return the TLS groups listed by openssl. X25519 is added when openssl exists, since some builds do not list it."""
    code, out = run(["openssl", "list", "-groups"])
    if code != 0:
        groups: set[str] = set()
    else:
        groups = set()
    for line in out.splitlines():
        token = line.strip().split()[0] if line.strip() else ""
        if token:
            groups.add(token)
    if shutil.which("openssl"):
        # OpenSSL 3 TLS 1.3 builds normally support X25519 even when older
        # command variants do not expose `list -groups`.
        groups.add("X25519")
    return groups


def main() -> None:
    """Probe the environment and write one status row per group."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", required=True, help="Output CSV path")
    parser.add_argument("--duration", type=int, default=3, help="Per-group seconds")
    args = parser.parse_args()

    out_path = Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    openssl = shutil.which("openssl")
    version = openssl_version()
    providers = provider_summary()
    groups = supported_groups()
    oqs_available = "oqsprovider" in providers.lower() or any("MLKEM" in g.upper() or "KYBER" in g.upper() for g in groups)

    rows = []
    for mode, group in GROUPS:
        if not openssl:
            status = "unavailable-openssl-missing"
        elif mode != "classical_tls13" and (not oqs_available or group not in groups):
            status = "unavailable-oqs-provider-or-group-missing"
        elif mode == "classical_tls13" and group not in groups:
            status = "unavailable-classical-group-missing"
        else:
            status = "available-run-required"
        rows.append({
            "mode": mode,
            "group": group,
            "status": status,
            "openssl_version": version,
            "providers": providers,
            "duration_s": args.duration,
            "handshakes_per_s": "",
            "p50_ms": "",
            "p95_ms": "",
            "cpu_pct": "",
            "rss_mb": "",
            "notes": "Runner records availability; execute in an oqs-provider environment for PQ rows.",
        })

    with out_path.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=list(rows[0].keys()), lineterminator="\n")
        writer.writeheader()
        writer.writerows(rows)


if __name__ == "__main__":
    main()
