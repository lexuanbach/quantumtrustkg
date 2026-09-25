#!/usr/bin/env python3
"""Local TLS 1.3 wire validation for the Checkout to Payment edge (RQ3, Sect. 4.3).

The controller of the paper decides profiles from declared metadata. The
declared-label compliance metric of Sect. 4 and the guarantees of Sect. 3 say
nothing about the bytes on the wire. This script observes the wire instead. For
each of three groups (X25519, X25519MLKEM768, MLKEM768) it starts openssl s_server
on a free loopback port, restricted to that group, with a one-day self-signed
RSA-2048 certificate. It then runs --trials handshakes (30 by default) with
openssl s_client, also restricted to that group. A trial counts as a success only
if s_client exits with code 0 and the group it reports contains the requested
group name. The reported group comes from the Server Temp Key line of the
s_client output. If that line is absent the script searches the output for the
expected group name. The edge label Checkout->Payment is a fixed tag on the rows
and does not select a workload. All handshakes go to a local server.

Timing. p50_ms and p95_ms are computed over the successful trials, and each
sample is the wall-clock time of a whole s_client process. They include process
start-up and are not comparable with the timings of run-pq-tls-handshake.py,
whose sample count and options differ. The p95 uses linear interpolation.

Status values: passed (all trials negotiated the group), partial, failed-handshake,
failed-server-start, failed-cert-generation, and unavailable-* when OpenSSL, the
OQS provider or the group is missing. For the two post-quantum profiles the script
requires both a listed group and either the oqsprovider or an ML-KEM or Kyber
group name in the group list. A missing capability is reported and never inferred
from the controller model. The group list is read from openssl list -tls-groups,
with -groups as a fallback for older builds. The mode envoy-kind runs nothing and
only writes status rows that say whether the optional liboqs/Envoy scenario would
be possible. The paper does not claim that stock Istio negotiates post-quantum TLS.

Output: wire-tls-validation.csv, the source of the 30/30 end-to-end success count
per profile that RQ3 reports (Sect. 4.3). run-evidence-summary.py reads this file
in its wire_tls_validation step. Limitation: the validation covers one host, one
OpenSSL build and a single-hop TLS connection. It is evidence about which group
the TLS library negotiates, and it does not observe a service mesh.
"""

from __future__ import annotations

import argparse
import csv
import re
import shutil
import socket
import statistics
import subprocess
import tempfile
import time
from pathlib import Path


PROFILES = [
    ("Checkout->Payment", "classical_tls13", "X25519"),
    ("Checkout->Payment", "hybrid_mlkem", "X25519MLKEM768"),
    ("Checkout->Payment", "pure_pq", "MLKEM768"),
]


def run(cmd: list[str], *, timeout: int = 15, input_text: str | None = None) -> tuple[int, str]:
    """Run a command with a timeout and return (exit code, stdout plus stderr).

    A missing binary or a timeout returns code 127 with the error text. Callers
    treat both cases as an unavailable capability.
    """
    try:
        proc = subprocess.run(
            cmd,
            input=input_text,
            text=True,
            capture_output=True,
            timeout=timeout,
            check=False,
        )
    except (FileNotFoundError, subprocess.TimeoutExpired) as exc:
        return 127, str(exc)
    return proc.returncode, (proc.stdout + proc.stderr).strip()


def openssl_version() -> str:
    """First line of openssl version, or openssl-not-found."""
    code, out = run(["openssl", "version"])
    return out.splitlines()[0] if code == 0 and out else "openssl-not-found"


def provider_summary() -> str:
    """Names of the loaded OpenSSL providers joined by semicolons, or unavailable."""
    code, out = run(["openssl", "list", "-providers"])
    if code != 0:
        return "unavailable"
    providers: list[str] = []
    for line in out.splitlines():
        line = line.strip()
        if line and not line.startswith("Providers:") and ":" not in line:
            providers.append(line.split()[0])
    return ";".join(providers) if providers else "unknown"


def supported_groups() -> set[str]:
    """TLS group names that the local openssl lists, plus X25519 when openssl exists.

    X25519 is added by hand so that the classical profile does not depend on how
    a given build formats its group listing."""
    groups: set[str] = set()
    # Newer OpenSSL (3.5+) exposes TLS groups, including native ML-KEM, via
    # "openssl list -tls-groups" as a colon-separated line. Older builds used
    # "openssl list -groups" with one token per line. Parse whichever responds.
    for subcmd in ("-tls-groups", "-groups"):
        code, out = run(["openssl", "list", subcmd])
        if code != 0 or not out:
            continue
        for line in out.splitlines():
            line = line.strip()
            if not line:
                continue
            for token in re.split(r"[\s:]+", line):
                if token:
                    groups.add(token)
    if shutil.which("openssl"):
        groups.add("X25519")
    return groups


def reserve_port() -> int:
    """Return a free loopback port. It is released before s_server binds it. A race
    with another process is possible but unlikely on a test host."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def generate_cert(workdir: Path) -> tuple[Path, Path, str]:
    """Create a one-day RSA-2048 self-signed certificate in workdir.

    Returns the certificate path, the key path and an error text that is empty on
    success. The certificate only has to make the handshake complete. The group
    negotiation does not depend on its key type."""
    cert = workdir / "server.crt"
    key = workdir / "server.key"
    cmd = [
        "openssl",
        "req",
        "-x509",
        "-newkey",
        "rsa:2048",
        "-keyout",
        str(key),
        "-out",
        str(cert),
        "-days",
        "1",
        "-nodes",
        "-subj",
        "/CN=localhost",
    ]
    code, out = run(cmd, timeout=30)
    return cert, key, out if code != 0 else ""


def parse_negotiated_group(output: str, expected: str) -> str:
    """Extract the negotiated group name from s_client output.

    The Server Temp Key line is the primary source. If it is missing, the first
    token equal to the expected group name is returned. An empty string means that
    no group could be read, and the trial then counts as a failure."""
    match = re.search(r"Server Temp Key:\s*([^\r\n,]+)", output)
    if match:
        return match.group(1).strip()
    for token in re.split(r"[\s,;:/()]+", output):
        if token.upper() == expected.upper():
            return token
    return ""


def percentile(values: list[float], pct: float) -> float:
    """Quantile pct (0 to 1) of values by linear interpolation, or 0.0 for no values."""
    if not values:
        return 0.0
    if len(values) == 1:
        return values[0]
    ordered = sorted(values)
    index = (len(ordered) - 1) * pct
    lower = int(index)
    upper = min(lower + 1, len(ordered) - 1)
    fraction = index - lower
    return ordered[lower] + (ordered[upper] - ordered[lower]) * fraction


def run_local_group(group: str, trials: int, cert: Path, key: Path) -> tuple[str, str, int, float, float, str]:
    """Run the handshake trials for one group against a fresh s_server.

    Returns (status, negotiated group, number of successes, median ms, p95 ms,
    tail of the last output). The server is stopped in the finally block, with a
    kill if it does not exit within two seconds."""
    port = reserve_port()
    server_cmd = [
        "openssl",
        "s_server",
        "-accept",
        str(port),
        "-cert",
        str(cert),
        "-key",
        str(key),
        "-tls1_3",
        "-groups",
        group,
        "-quiet",
    ]
    server = subprocess.Popen(server_cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    # Give the server time to bind. If it has exited by then (for example because
    # the group is not accepted), report a failed start and skip the trials.
    time.sleep(0.25)
    if server.poll() is not None:
        stdout, stderr = server.communicate()
        return "failed-server-start", "", 0, 0.0, 0.0, (stdout + stderr)[-240:]

    successes = 0
    negotiated = ""
    timings: list[float] = []
    last_output = ""
    try:
        for _ in range(trials):
            start = time.perf_counter()
            code, output = run(
                [
                    "openssl",
                    "s_client",
                    "-connect",
                    f"127.0.0.1:{port}",
                    "-servername",
                    "localhost",
                    "-tls1_3",
                    "-groups",
                    group,
                    "-verify_quiet",
                    "-brief",
                ],
                timeout=10,
                input_text="",
            )
            elapsed_ms = (time.perf_counter() - start) * 1000.0
            last_output = output
            found = parse_negotiated_group(output, group)
            if code == 0 and found and group.upper() in found.upper():
                successes += 1
                negotiated = found
                timings.append(elapsed_ms)
        if successes == trials:
            status = "passed"
        elif successes > 0:
            status = "partial"
        else:
            status = "failed-handshake"
        return status, negotiated, successes, statistics.median(timings) if timings else 0.0, percentile(timings, 0.95), last_output[-240:]
    finally:
        server.terminate()
        try:
            server.wait(timeout=2)
        except subprocess.TimeoutExpired:
            server.kill()
            server.wait(timeout=2)


def unavailable_row(edge: str, profile: str, group: str, status: str, trials: int, version: str, providers: str, note: str) -> dict[str, str]:
    return {
        "edge": edge,
        "profile": profile,
        "group": group,
        "status": status,
        "negotiated_group": "",
        "trials": str(trials),
        "successes": "0",
        "p50_ms": "",
        "p95_ms": "",
        "openssl_version": version,
        "providers": providers,
        "notes": note,
    }


def local_rows(trials: int) -> list[dict[str, str]]:
    """Run the local validation for every profile and return the CSV rows.

    A profile that cannot be tried becomes a row with no measurements and a
    status that names the missing piece."""
    openssl = shutil.which("openssl")
    version = openssl_version()
    providers = provider_summary()
    groups = supported_groups()
    oqs_available = "oqsprovider" in providers.lower() or any("MLKEM" in group.upper() or "KYBER" in group.upper() for group in groups)
    rows: list[dict[str, str]] = []

    if not openssl:
        return [
            unavailable_row(edge, profile, group, "unavailable-openssl-missing", trials, version, providers, "OpenSSL binary is not available")
            for edge, profile, group in PROFILES
        ]

    with tempfile.TemporaryDirectory(prefix="qtkg-wire-tls-") as tmp:
        cert, key, cert_error = generate_cert(Path(tmp))
        if cert_error:
            return [
                unavailable_row(edge, profile, group, "failed-cert-generation", trials, version, providers, cert_error[-160:])
                for edge, profile, group in PROFILES
            ]

        for edge, profile, group in PROFILES:
            if profile != "classical_tls13" and (not oqs_available or group not in groups):
                rows.append(
                    unavailable_row(
                        edge,
                        profile,
                        group,
                        "unavailable-oqs-provider-or-group-missing",
                        trials,
                        version,
                        providers,
                        "Local OpenSSL does not expose the requested OQS group",
                    )
                )
                continue
            if profile == "classical_tls13" and group not in groups:
                rows.append(
                    unavailable_row(edge, profile, group, "unavailable-classical-group-missing", trials, version, providers, "Requested classical group is unavailable")
                )
                continue

            status, negotiated, successes, p50, p95, note = run_local_group(group, trials, cert, key)
            rows.append(
                {
                    "edge": edge,
                    "profile": profile,
                    "group": group,
                    "status": status,
                    "negotiated_group": negotiated,
                    "trials": str(trials),
                    "successes": str(successes),
                    "p50_ms": f"{p50:.3f}" if successes else "",
                    "p95_ms": f"{p95:.3f}" if successes else "",
                    "openssl_version": version,
                    "providers": providers,
                    "notes": "Negotiated group observed by openssl s_client" if successes else note,
                }
            )
    return rows


def envoy_rows(trials: int, envoy_image: str | None) -> list[dict[str, str]]:
    """Status-only rows for the optional liboqs/Envoy scenario, which this script does not run."""
    version = openssl_version()
    providers = provider_summary()
    if not envoy_image:
        status = "unavailable-envoy-image-missing"
        note = "Provide --envoy-image for a liboqs/Envoy experiment; stock Istio is not claimed to negotiate PQ TLS"
    elif shutil.which("kind") is None:
        status = "unavailable-kind-missing"
        note = "kind is unavailable, so the optional liboqs/Envoy scenario was not run"
    else:
        status = "run-required-envoy-kind"
        note = "liboqs/Envoy image supplied; run the optional scenario before claiming Envoy PQ negotiation"
    return [unavailable_row(edge, profile, group, status, trials, version, providers, note) for edge, profile, group in PROFILES]


def main() -> None:
    """Choose the mode, build the rows and write the CSV."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", required=True)
    parser.add_argument("--trials", type=int, default=30)
    parser.add_argument("--mode", choices=["local-openssl", "envoy-kind"], default="local-openssl")
    parser.add_argument("--envoy-image")
    args = parser.parse_args()

    rows = local_rows(args.trials) if args.mode == "local-openssl" else envoy_rows(args.trials, args.envoy_image)
    out_path = Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    fieldnames = [
        "edge",
        "profile",
        "group",
        "status",
        "negotiated_group",
        "trials",
        "successes",
        "p50_ms",
        "p95_ms",
        "openssl_version",
        "providers",
        "notes",
    ]
    with out_path.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=fieldnames, lineterminator="\n")
        writer.writeheader()
        writer.writerows(rows)


if __name__ == "__main__":
    main()
