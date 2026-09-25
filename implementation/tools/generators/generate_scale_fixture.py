#!/usr/bin/env python3
"""Generate deterministic scale fixtures for the semantic-core measurements.

The output is the small Turtle subset that graph.BuildEdgesFromTTL parses:
services, communication edges, three policies, three protocol profiles and one
provenance observation (fresh until 2027-05-09). Given --services, --seed and
--edge-factor the file is byte-identical. The Makefile target scale-stress writes
500-, 1000- and 2000-service fixtures with seed 20260509 and an edge factor of
2.0, which matches the two-edges-per-service ratio of the testbed.

Structure of a fixture:
  services   every tenth service is under the PCI policy (quantum-safe required),
             every fourth under the partner policy and the rest under the internal
             policy (hybrid required). Each service supports the classical and the
             hybrid profile, and the quantum-safe profile with probability 0.68 (always
             for PCI services)
  edges      a ring over all services, then random distinct pairs up to
             services * edge-factor. An edge touching a PCI service is regulated,
             an edge with a partner service or every seventh edge is cross-zone
             and the rest are internal

The fixtures feed cmd/scale-stress (H3 semantic core, extended version) and the
tests in tests/unit/fixtures_test.go. They are synthetic and carry no measurement
of their own.
"""

from __future__ import annotations

import argparse
import random
from pathlib import Path


def service_name(index: int) -> str:
    """Return the Turtle name of the service with the given index."""
    return f"Service{index:04d}"


def edge_name(index: int) -> str:
    """Return the Turtle name of the communication with the given ordinal."""
    return f"edge{index:05d}"


def service_policy(index: int) -> str:
    """Return the policy of a service: PCI for every tenth index, partner for every fourth, else internal."""
    if index % 10 == 0:
        return "PCIPolicy"
    if index % 4 == 0:
        return "PartnerPolicy"
    return "InternalPolicy"


def service_profiles(index: int, rng: random.Random) -> list[str]:
    """Return the profiles a service supports. The random draw makes the result depend on the generator state."""
    profiles = ["ClassicalProfile", "HybridProfile"]
    if index % 10 == 0 or rng.random() < 0.68:
        profiles.append("QuantumSafeProfile")
    return profiles


def edge_boundary(src: int, dst: int, ordinal: int) -> str:
    """Return the trust boundary of an edge: regulated, cross-zone or internal."""
    if src % 10 == 0 or dst % 10 == 0:
        return "regulated"
    if ordinal % 7 == 0 or src % 4 == 0 or dst % 4 == 0:
        return "cross-zone"
    return "internal"


def build_edges(services: int, edge_count: int, rng: random.Random) -> list[tuple[int, int]]:
    """Return edge_count distinct directed pairs, starting with a ring so that every service has an outgoing edge."""
    edges: list[tuple[int, int]] = []
    seen: set[tuple[int, int]] = set()

    for src in range(services):
        dst = (src + 1) % services
        pair = (src, dst)
        if pair not in seen:
            seen.add(pair)
            edges.append(pair)
        if len(edges) >= edge_count:
            return edges

    while len(edges) < edge_count:
        src = rng.randrange(services)
        dst = rng.randrange(services)
        if src == dst:
            dst = (dst + 1) % services
        pair = (src, dst)
        if pair in seen:
            continue
        seen.add(pair)
        edges.append(pair)

    return edges


def generate(services: int, seed: int, edge_factor: float) -> str:
    """Return the complete Turtle text for a mesh of the given size."""
    rng = random.Random(seed)
    edge_count = max(services - 1, int(round(services * edge_factor)))
    edges = build_edges(services, edge_count, rng)

    lines: list[str] = [
        "@prefix : <http://quantumtrustkg.io/ontology#> .",
        "",
        f":Scale{services}Scenario a :Configuration .",
        "",
        ":ClassicalProfile a :Protocol ;",
        '    :hasSecurityCategory "classical" ;',
        '    :hasOverheadScore "0.05" .',
        "",
        ":HybridProfile a :Protocol ;",
        '    :hasSecurityCategory "hybrid" ;',
        '    :hasOverheadScore "0.30" .',
        "",
        ":QuantumSafeProfile a :Protocol ;",
        '    :hasSecurityCategory "quantum_safe" ;',
        '    :hasOverheadScore "0.52" .',
        "",
        ":InternalPolicy a :Policy ;",
        '    :requiresLevel "hybrid" .',
        "",
        ":PartnerPolicy a :Policy ;",
        '    :requiresLevel "hybrid" .',
        "",
        ":PCIPolicy a :Policy ;",
        '    :requiresLevel "quantum_safe" .',
        "",
        ":ScaleObservation a :Observation ;",
        '    :observedBy "deterministic-scale-generator" ;',
        '    :observedAt "2026-05-09T12:00:00Z" ;',
        '    :freshUntil "2027-05-09T12:00:00Z" .',
        "",
    ]

    service_profile_map: dict[int, list[str]] = {}
    for index in range(services):
        profiles = service_profiles(index, rng)
        service_profile_map[index] = profiles
        profile_refs = ", ".join(f":{profile}" for profile in profiles)
        lines.extend(
            [
                f":{service_name(index)} a :Service ;",
                f"    :subjectTo :{service_policy(index)} ;",
                f"    :supportsProfile {profile_refs} .",
                "",
            ]
        )

    for ordinal, (src, dst) in enumerate(edges):
        boundary = edge_boundary(src, dst, ordinal)
        lines.extend(
            [
                f":{edge_name(ordinal)} a :Communication ;",
                f"    :sourceService :{service_name(src)} ;",
                f"    :destinationService :{service_name(dst)} ;",
                f'    :crossesBoundary "{boundary}" .',
                "",
            ]
        )

    return "\n".join(lines)


def main() -> None:
    """Parse the arguments and write the fixture."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--services", type=int, required=True)
    parser.add_argument("--seed", type=int, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--edge-factor", type=float, default=2.0)
    args = parser.parse_args()

    if args.services < 2:
        raise SystemExit("--services must be at least 2")
    if args.edge_factor <= 0:
        raise SystemExit("--edge-factor must be positive")

    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(
        generate(args.services, args.seed, args.edge_factor),
        encoding="utf-8",
    )


if __name__ == "__main__":
    main()
