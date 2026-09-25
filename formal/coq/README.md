# QuantumTrustKG Coq/Rocq Formalization

This directory contains a small Coq/Rocq development for the paper-level
QuantumTrustKG orchestration guarantees. It formalizes an abstract typed
control-plane model, not the Go implementation and not RDF/SPARQL/SHACL
semantics.

## Build

The development was checked with Rocq/Coq 9.1.1 and only uses the standard
library.

```sh
make -C formal/coq
```

## Files

- `Model.v`: services, edges, policies, profiles, provenance, rollout state,
  graph availability, derived requirements, and admissibility predicates.
- `QTPO.v`: abstract QTPO selection, direct promotion, hybrid fallback, and
  blocking outcomes.
- `Guarantees.v`: checked versions of the paper guarantees G1--G4.
- `Examples.v`: small finance-style scenarios for successful promotion,
  stale-fact blocking, conflicting-fact blocking, and regulated requirements.

## Proof Boundary

The proofs certify orchestration-layer invariants over the abstract model:
promoted profiles pass the modeled gates, direct promotions preserve the
declared security class, hybrid fallback preserves recorded robustness metadata,
and bad graph/provenance/rollout/mesh gates fail closed.

The development intentionally does not prove primitive-level cryptographic
security, does not mechanize RDF/SPARQL/SHACL execution, and does not prove a
refinement theorem for the Go controller.
