#!/usr/bin/env python3
"""Measured TLS 1.3 handshake time for classical, hybrid and pure post-quantum key exchange.

This is the measurement behind the measured part of the setup-cost term in RQ3
(Sect. 4.3). run-pq-tls-benchmark.py only records whether the groups exist. This
script uses them. It starts one local OpenSSL s_server and, for each of X25519
(classical), X25519MLKEM768 (hybrid) and MLKEM768 (pure post-quantum), it runs
--handshakes complete TLS 1.3 handshakes with openssl s_client against it
(200 by default, 300 in the Makefile target pq-handshake). The server offers all
three groups, and each client run restricts itself to one group with -groups.

Timing. Each sample is the wall-clock time of one s_client process, from spawn to
exit. It therefore contains process start-up and the loopback connection as well as the
handshake. Absolute values are therefore not the latency of a handshake inside a
service mesh. The comparison across groups is meaningful because every group pays
the same fixed cost. A run whose s_client exits with a non-zero code is dropped.
Per group the script reports the median (the upper middle element of the sorted
samples for an even count), the mean, and the overhead of the median over the
classical median in percent. The negotiated column is the group that s_client
printed for the first successful run, and it falls back to the requested name
when the output had no such line.

Availability. A group that is missing from openssl list -tls-groups gets the row
status unavailable-group-missing and no numbers. A group with no successful run
gets failed-no-samples. Native ML-KEM needs OpenSSL 3.5 or later, or an OQS
provider build. The server certificate is a one-day self-signed P-256 key in a
temporary directory.

Output: pq-tls-handshake-results.csv (written by make pq-handshake). In the paper
the RQ3 paragraph reports the medians and the two overhead percentages of this
file, marked measured, and states that the other setup-cost terms (certificate
and key material, staged rollout, configuration churn) are modelled and that no
aggregate migration cost is claimed. The 30/30 end-to-end trials come from
run-wire-tls-validation.py.
"""
from __future__ import annotations

import argparse
import csv
import os
import re
import shutil
import socket
import subprocess
import tempfile
import time
from pathlib import Path

GROUPS = [
    ("classical_tls13", "X25519"),
    ("hybrid_mlkem", "X25519MLKEM768"),
    ("pure_pq", "MLKEM768"),
]


def run(cmd, **kw):
    """Run a command and capture its text output. Raises if the binary is missing."""
    return subprocess.run(cmd, text=True, capture_output=True, **kw)


def available_groups() -> set[str]:
    """Return the lower-cased TLS group names that the local openssl lists."""
    out = run(["openssl", "list", "-tls-groups"]).stdout
    return {g.lower() for g in re.split(r"[\s:,]+", out.strip()) if g}


def free_port() -> int:
    """Ask the kernel for a free loopback port. It is released before s_server binds it."""
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    p = s.getsockname()[1]
    s.close()
    return p


def main() -> None:
    """Measure the handshakes for each group and write one CSV row per group."""
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", required=True)
    ap.add_argument("--handshakes", type=int, default=200)
    args = ap.parse_args()

    out_path = Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    fields = ["mode", "group", "status", "handshakes", "median_ms", "mean_ms",
              "rel_overhead_pct", "negotiated", "openssl_version"]
    version = run(["openssl", "version"]).stdout.strip()

    if not shutil.which("openssl"):
        with out_path.open("w", newline="") as fh:
            w = csv.DictWriter(fh, fieldnames=fields, lineterminator="\n")
            w.writeheader()
            w.writerow({"mode": "all", "status": "unavailable-openssl-missing"})
        return

    groups = available_groups()
    tmp = tempfile.mkdtemp(prefix="pqtls-")
    cert, key = os.path.join(tmp, "c.pem"), os.path.join(tmp, "k.pem")
    run(["openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt",
         "ec_paramgen_curve:prime256v1", "-keyout", key, "-out", cert,
         "-days", "1", "-nodes", "-subj", "/CN=localhost"])

    # -naccept stops the server after ten times the requested handshake count,
    # which leaves room for all three groups and any failed attempts.
    port = free_port()
    srv = subprocess.Popen(
        ["openssl", "s_server", "-accept", str(port), "-cert", cert, "-key", key,
         "-tls1_3", "-groups", "X25519:X25519MLKEM768:MLKEM768", "-quiet", "-naccept", str(10 * args.handshakes)],
        stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    # Give the server time to bind before the first client connects.
    time.sleep(1.0)

    results = {}
    classical_median = None
    rows = []
    try:
        for mode, group in GROUPS:
            if group.lower() not in groups:
                rows.append({"mode": mode, "group": group, "status": "unavailable-group-missing",
                             "handshakes": 0, "openssl_version": version})
                continue
            samples = []
            negotiated = ""
            for _ in range(args.handshakes):
                t0 = time.perf_counter()
                p = subprocess.run(
                    ["openssl", "s_client", "-connect", f"127.0.0.1:{port}",
                     "-tls1_3", "-groups", group, "-brief"],
                    input="", text=True, capture_output=True)
                dt = (time.perf_counter() - t0) * 1000.0
                if p.returncode == 0:
                    samples.append(dt)
                    if not negotiated:
                        m = re.search(r"Negotiated TLS1.3 group:\s*(\S+)", p.stderr)
                        if m:
                            negotiated = m.group(1)
            if not samples:
                rows.append({"mode": mode, "group": group, "status": "failed-no-samples",
                             "handshakes": 0, "openssl_version": version})
                continue
            # The median is robust to the slow outliers that process start-up adds.
            samples.sort()
            median = samples[len(samples) // 2]
            mean = sum(samples) / len(samples)
            results[mode] = median
            if mode == "classical_tls13":
                classical_median = median
            rel = "" if classical_median in (None, 0) else round(100 * (median - classical_median) / classical_median, 1)
            rows.append({"mode": mode, "group": group, "status": "measured",
                         "handshakes": len(samples), "median_ms": round(median, 3),
                         "mean_ms": round(mean, 3), "rel_overhead_pct": rel,
                         "negotiated": negotiated or group, "openssl_version": version})
    finally:
        srv.terminate()

    with out_path.open("w", newline="") as fh:
        w = csv.DictWriter(fh, fieldnames=fields, lineterminator="\n")
        w.writeheader()
        for r in rows:
            w.writerow(r)
    for r in rows:
        print(r)


if __name__ == "__main__":
    main()
