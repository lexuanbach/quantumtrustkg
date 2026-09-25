# Reproducibility Notes

Development notes. The commands behind the paper results are in the top-level
`README.md` and `REPRODUCIBILITY.md`, which take precedence where these notes differ.

These notes document what can be reproduced from the current artifact. The
controller and semantic checks are runnable; the PQ data-plane setup-cost numbers
remain model-driven setup-cost estimates because mainstream Istio/Envoy does
not yet expose a uniform native PQ negotiation path.

## Prerequisites

- Go 1.22 or newer for the controller tests
- Python 3 for deterministic scale-fixture generation
- Coq/Rocq 9.1.1 for the abstract invariant development
- Optional: OpenSSL 3.5 or newer with native ML-KEM for the handshake timing
  and the wire validation
- Optional: `kind` for full-controller scale/churn runs beyond fixture-derived
  cardinality checks

## Checks

To collect the main evidence in one place, run:

```sh
python3 implementation/experiments/scripts/run-evidence-summary.py \
  --out implementation/experiments/outputs/processed/evidence-summary.json \
  --markdown-out implementation/experiments/outputs/processed/evidence-summary.md
```

The summary runs available checks and reports missing `kind`, OpenSSL/OQS
groups, optional liboqs/Envoy images, or Coq/Rocq as skipped or unavailable
evidence rather than inferred measurements.

```sh
cd implementation/controller
go test ./...
cd ../..

make -C formal/coq

```

## Scale Stress Reproduction

The 500/1000/2000-service runs are semantic-control-plane stress checks. They do not
start Kubernetes, Fuseki, or Istio.

```sh
make -C implementation scale-stress
```

The target generates `scale-500.ttl`, `scale-1000.ttl` and `scale-2000.ttl` with seed
20260509 and edge factor 2.0, then runs `cmd/scale-stress` with 15 iterations.

## Online Boutique Decision Trace

The Online Boutique fixture is generated deterministically and checked through
the same Turtle parser used by the controller tests.

```sh
python3 implementation/tools/generators/generate_online_boutique_fixture.py \
  --out implementation/semantics/fixtures/online-boutique.ttl

cd implementation/controller
go run ./cmd/decision-trace \
  --fixture online-boutique.ttl \
  --source Checkout \
  --destination Payment \
  --out ../experiments/outputs/processed/online-boutique-decision-trace.json
```

The expected trace requires `quantum_safe` for `Checkout -> Payment`, rejects
`HybridProfile` at the policy/security-class gate, selects
`QuantumSafeProfile`, and records the `PeerAuthentication` and
`DestinationRule` names that would be synthesized by the controller.

## Bounded PQ TLS Evidence

The native TLS runner does not claim Envoy/Istio integration. It records whether
the local OpenSSL installation exposes classical, hybrid ML-KEM, and pure
ML-KEM groups. Hosts without `oqs-provider` produce explicit unavailable rows.

```sh
python3 implementation/experiments/scripts/run-pq-tls-benchmark.py \
  --out implementation/experiments/outputs/processed/pq-tls-benchmark-results.csv
```

The wire-validation runner starts a local OpenSSL TLS 1.3 client/server pair
and records the negotiated group for any exposed group. By default it tests
`X25519`, `X25519MLKEM768`, and `MLKEM768`. Optional `envoy-kind` mode records
missing liboqs/Envoy or `kind` support explicitly and does not turn stock Istio
into a PQ negotiation claim.

```sh
python3 implementation/experiments/scripts/run-wire-tls-validation.py \
  --out implementation/experiments/outputs/processed/wire-tls-validation.csv
```

## Full-Controller Scale/Churn Harness

The scale/churn runner is designed for `kind` environments with the controller,
Fuseki, CRDs, and Istio CRDs installed. On hosts without `kind`, it writes
`not-run-kind-unavailable` rows instead of fabricating latency measurements.

```sh
python3 implementation/experiments/scripts/run-controller-scale-churn.py \
  --scale-out implementation/experiments/outputs/processed/controller-scale-results.csv \
  --churn-out implementation/experiments/outputs/processed/controller-churn-results.csv
```

## Semantic Assets

The controller loads and checks these artifact files:

- `implementation/semantics/ontology/quantumtrustkg.ttl`
- `implementation/semantics/queries/candidate_protocols.rq`
- `implementation/semantics/queries/active_policies.rq`
- `implementation/semantics/queries/stale_facts.rq`
- `implementation/semantics/queries/migration_edges.rq`
- `implementation/semantics/constraints/assignment.shacl.ttl`
- `implementation/semantics/constraints/policy.shacl.ttl`
- `implementation/semantics/constraints/provenance.shacl.ttl`
- `implementation/semantics/fixtures/finance-scenario.ttl`
- `implementation/semantics/fixtures/stale-finance-scenario.ttl`
- `implementation/semantics/fixtures/conflicting-finance-scenario.ttl`
- `implementation/semantics/fixtures/online-boutique.ttl`
- `implementation/semantics/fixtures/scale-500.ttl`
- `implementation/semantics/fixtures/scale-1000.ttl`

The SHACL files are validation aids for fixtures and ontology examples. Runtime
promotion safety is enforced by explicit controller checks, not by a full SHACL
engine.

## Profile-to-Istio Walkthrough

Hybrid internal edge:

- Fixture edge: `frontendToAuth` from `Frontend` to `Auth`
- Required class: `hybrid`
- Selected profile: `HybridProfile`
- `PeerAuthentication`: `frontendToAuth-mtls`, destination selector `app: Auth`,
  mTLS mode `STRICT`, profile annotation `HybridProfile`
- `DestinationRule`: `frontendToAuth-tls`, host
  `Auth.finance.svc.cluster.local`, TLS mode `ISTIO_MUTUAL`, profile annotation
  `HybridProfile`
- Status reason after success: `assignment-applied`

Quantum-safe regulated edge:

- Fixture edge: `authToPayment` from `Auth` to `Payment`
- Required class: `quantum_safe` from PCI and regulated-boundary context
- Selected profile: `QuantumSafeProfile`
- `PeerAuthentication`: `authToPayment-mtls`, destination selector
  `app: Payment`, mTLS mode `STRICT`, security-level annotation
  `quantum_safe`
- `DestinationRule`: `authToPayment-tls`, host
  `Payment.finance.svc.cluster.local`, TLS mode `ISTIO_MUTUAL`, reduced
  connection limit for higher-cost setup
- Status reason after success: `assignment-applied`

Rollback behavior:

- A staged migration records promoted edges as pending health confirmation.
- If the generated mesh health marker reports an unhealthy edge, the controller
  moves the migration to `Rollback`, clears the pending batch, and does not
  advance the next rollout window.
- During graph outage, new promotions are blocked. Existing active assignments
  are preserved and reported with `graph-unavailable-preserved-active`.

## Provenance Boundary

Current provenance integrity is based on operator/CRD provenance, observed
Kubernetes state, timestamps, freshness windows, and contradiction checks. The
artifact does not verify signed workload capability claims, bind claims to
SPIFFE/SPIRE identities, or integrate cert-manager/SDS certificate material.
Those are future hardening steps, not current guarantees.
