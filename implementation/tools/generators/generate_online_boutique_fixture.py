#!/usr/bin/env python3
"""Write the deterministic Online Boutique fixture used by the evaluation.

The fixture describes the eleven services of the Online Boutique demo (Google
Cloud Platform microservices) and the 11 calls between them, the profiles they
support, three policies (default internal, partner and PCI) and one provenance
observation. Checkout and Payment are under the PCI policy, which means the edge from
Checkout to Payment requires a quantum-safe profile. Shipping is under the
partner policy. The topology is the real call structure of the demo, and the
capability sets and policies are chosen for the example. They are declared labels
and not measurements.

The text is a constant, which means the output is byte-identical on every run. It is
consumed by cmd/decision-trace (online-boutique-decision-trace.json, the worked
example of the extended version), by cmd/baseline-compare (which keeps its own
11-edge model in generateOnlineBoutique) and by tests/unit/online_boutique_test.go.
"""

from __future__ import annotations

import argparse
from pathlib import Path


FIXTURE = """@prefix : <http://quantumtrustkg.io/ontology#> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .

:Frontend a :Service ;
  :subjectTo :DefaultInternalPolicy ;
  :supportsProfile :HybridProfile, :QuantumSafeProfile .

:ProductCatalog a :Service ;
  :subjectTo :DefaultInternalPolicy ;
  :supportsProfile :HybridProfile, :QuantumSafeProfile .

:Currency a :Service ;
  :subjectTo :DefaultInternalPolicy ;
  :supportsProfile :HybridProfile .

:Cart a :Service ;
  :subjectTo :DefaultInternalPolicy ;
  :supportsProfile :HybridProfile, :QuantumSafeProfile .

:RedisCart a :Service ;
  :subjectTo :DefaultInternalPolicy ;
  :supportsProfile :HybridProfile .

:Checkout a :Service ;
  :subjectTo :PCIPolicy ;
  :supportsProfile :HybridProfile, :QuantumSafeProfile .

:Payment a :Service ;
  :subjectTo :PCIPolicy ;
  :supportsProfile :HybridProfile, :QuantumSafeProfile .

:Shipping a :Service ;
  :subjectTo :PartnerPolicy ;
  :supportsProfile :HybridProfile, :QuantumSafeProfile .

:Email a :Service ;
  :subjectTo :DefaultInternalPolicy ;
  :supportsProfile :HybridProfile .

:Recommendation a :Service ;
  :subjectTo :DefaultInternalPolicy ;
  :supportsProfile :HybridProfile .

:Ad a :Service ;
  :subjectTo :DefaultInternalPolicy ;
  :supportsProfile :HybridProfile .

:HybridProfile a :Protocol ;
  :hasSecurityCategory "hybrid" ;
  :hasOverheadScore "1.2"^^xsd:decimal .

:QuantumSafeProfile a :Protocol ;
  :hasSecurityCategory "quantum_safe" ;
  :hasOverheadScore "1.5"^^xsd:decimal .

:DefaultInternalPolicy a :Policy ;
  :requiresLevel "hybrid" .

:PartnerPolicy a :Policy ;
  :requiresLevel "hybrid" .

:PCIPolicy a :Policy ;
  :requiresLevel "quantum_safe" .

:frontendToCheckout a :Communication ;
  :sourceService :Frontend ;
  :destinationService :Checkout ;
  :crossesBoundary "internal" ;
  :assignedProtocol :HybridProfile .

:frontendToProductCatalog a :Communication ;
  :sourceService :Frontend ;
  :destinationService :ProductCatalog ;
  :crossesBoundary "internal" ;
  :assignedProtocol :HybridProfile .

:frontendToCart a :Communication ;
  :sourceService :Frontend ;
  :destinationService :Cart ;
  :crossesBoundary "internal" ;
  :assignedProtocol :HybridProfile .

:frontendToCurrency a :Communication ;
  :sourceService :Frontend ;
  :destinationService :Currency ;
  :crossesBoundary "internal" ;
  :assignedProtocol :HybridProfile .

:checkoutToPayment a :Communication ;
  :sourceService :Checkout ;
  :destinationService :Payment ;
  :crossesBoundary "regulated" ;
  :assignedProtocol :HybridProfile .

:checkoutToCart a :Communication ;
  :sourceService :Checkout ;
  :destinationService :Cart ;
  :crossesBoundary "internal" ;
  :assignedProtocol :HybridProfile .

:checkoutToShipping a :Communication ;
  :sourceService :Checkout ;
  :destinationService :Shipping ;
  :crossesBoundary "partner" ;
  :assignedProtocol :HybridProfile .

:checkoutToEmail a :Communication ;
  :sourceService :Checkout ;
  :destinationService :Email ;
  :crossesBoundary "internal" ;
  :assignedProtocol :HybridProfile .

:cartToRedisCart a :Communication ;
  :sourceService :Cart ;
  :destinationService :RedisCart ;
  :crossesBoundary "internal" ;
  :assignedProtocol :HybridProfile .

:recommendationToProductCatalog a :Communication ;
  :sourceService :Recommendation ;
  :destinationService :ProductCatalog ;
  :crossesBoundary "internal" ;
  :assignedProtocol :HybridProfile .

:frontendToAd a :Communication ;
  :sourceService :Frontend ;
  :destinationService :Ad ;
  :crossesBoundary "internal" ;
  :assignedProtocol :HybridProfile .

:onlineBoutiqueObservation a :Observation ;
  :observedBy "online-boutique-fixture" ;
  :observedAt "2026-05-10T09:00:00Z"^^xsd:dateTime ;
  :freshUntil "2027-05-10T09:10:00Z"^^xsd:dateTime .
"""


def main() -> None:
    """Write the fixture to --out."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", required=True, help="Output Turtle path")
    args = parser.parse_args()
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(FIXTURE, encoding="utf-8")


if __name__ == "__main__":
    main()
