# Evaluation Notes

Development notes. The commands behind the paper results are in the top-level
`README.md` and `REPRODUCIBILITY.md`, which take precedence where these notes differ.

This artifact evaluates QuantumTrustKG as a semantic control-plane model and
controller loop. It does not measure a production liboqs/Envoy post-quantum data
path. PQ setup-cost values in the paper are model-driven estimates layered
on real controller actions: capability ingestion, QTPO filtering/ranking, Istio
manifest synthesis, rollout gating, status writeback, and recovery behavior.

## Artifact Map

- Controller runtime: `implementation/controller/internal/controllers/`
- QTPO selection and rollout logic: `implementation/controller/internal/orchestration/`
- Istio synthesis: `implementation/controller/internal/mesh/`
- CRDs and schemas: `implementation/api/crds/`, `implementation/api/schemas/`
- Ontology: `implementation/semantics/ontology/quantumtrustkg.ttl`
- SHACL validation aids: `implementation/semantics/constraints/*.shacl.ttl`
- SPARQL snippets: `implementation/semantics/queries/*.rq`
- Scenario fixtures: `implementation/semantics/fixtures/*.ttl`
- Scale generator: `implementation/tools/generators/generate_scale_fixture.py`
- Online Boutique generator:
  `implementation/tools/generators/generate_online_boutique_fixture.py`
- Stress harness: `implementation/controller/cmd/scale-stress`
- Online Boutique decision trace:
  `implementation/controller/cmd/decision-trace`
- PQ TLS availability and wire-validation runners:
  `implementation/experiments/scripts/run-pq-tls-benchmark.py`
  `implementation/experiments/scripts/run-wire-tls-validation.py`
- Controller scale/churn harness:
  `implementation/experiments/scripts/run-controller-scale-churn.py`
- Processed stress output:
  `implementation/experiments/outputs/processed/scale-stress-results.csv`
- Baseline configs: `implementation/experiments/baselines/*.yaml`
- Unit and failure-injection tests: `implementation/controller/tests/unit/`

## Main Evidence

The 50/100/200-service compliance, latency, and migration summaries in the
paper are generated from controlled fixture/testbed runs. QTPO-NoKG is the main
ablation for isolating the graph-backed provenance join; Manual rollout remains
an appendix-only reference point because it is operator-driven and does not
represent an automated controller.

The 500/1000/2000-service stress fixtures are deterministic semantic-control-plane
checks. They exercise generated Turtle input plus the Go fixture parser/filter
path and deliberately exclude Fuseki availability, Kubernetes watches, and
Istio object churn. Use them to validate reproducibility and parser/filter
growth, not to claim full mesh-scale deployment.

The Online Boutique fixture is the primary realism anchor in this revision. It
encodes the standard service graph and declared trust boundaries for checkout,
payment, cart, shipping, email, recommendation, and ad flows. The checked
decision trace for `Checkout -> Payment` records the quantum-safe policy
requirement, the rejected hybrid candidate, the selected quantum-safe profile,
and the generated Istio object names. The finance fixture remains useful as a
domain case study and failure-injection fixture.

The native PQ evidence is intentionally bounded. The OpenSSL runners record the
local OpenSSL version, provider list, group availability, and negotiated group
when a local TLS 1.3 handshake can be completed. On hosts without
`oqs-provider`, PQ rows are emitted as unavailable rather than converted into
model-derived numbers. These rows are native TLS evidence, not a stock
Envoy/Istio data-plane measurement.

The controller scale/churn harness records whether a `kind` cluster is present.
If `kind` is unavailable, processed rows are marked `not-run-kind-unavailable`
and contain only fixture-derived object/status cardinalities. This prevents the
artifact from silently promoting semantic stress evidence into a full
Kubernetes/Istio deployment claim.

Failure-injection evidence is synthetic and fixture-based by design. The checked
cases are:

- stale capability fact: `stale-finance-scenario.ttl` and
  `TestReconcileFixtureEdgesFailsClosedOnStaleFacts`
- contradictory fact/advisory: `conflicting-finance-scenario.ttl` and
  `TestReconcileFixtureEdgesFailsClosedOnConflictingFacts`
- graph outage: `TestReconcileFixtureEdgesFailsClosedOnGraphOutageWhenRequired`
- active assignment preservation during outage:
  `TestReconcileFixtureEdgesPreservesActiveAssignmentDuringGraphOutage`
- mesh-readiness mismatch and rollback:
  `TestReconcileFixtureEdgesDoesNotAdvanceWhenPreviousBatchTurnsUnhealthy`
- strict policy vs. hybrid-only endpoints:
  `TestReconcileFixtureEdgesBlocksPolicyConflictWithoutHybridFallback`

Every failure case should produce zero unsafe promotions. A blocked edge is
reported as non-compliant in the headline compliance metric, but it is a
blocked-safe outcome rather than an accepted downgrade.

## Run Commands

From the repository root:

```sh
cd implementation/controller && go test ./...
cd ../..
make -C formal/coq
```

To run the one-shot finance fixture:

```sh
implementation/experiments/scripts/run-fixture.sh finance-scenario.ttl
```

To exercise stale or contradictory fixtures:

```sh
implementation/experiments/scripts/run-fixture.sh stale-finance-scenario.ttl
implementation/experiments/scripts/run-fixture.sh conflicting-finance-scenario.ttl
```

To regenerate the Online Boutique fixture and decision trace:

```sh
python3 implementation/tools/generators/generate_online_boutique_fixture.py \
  --out implementation/semantics/fixtures/online-boutique.ttl

cd implementation/controller
go run ./cmd/decision-trace \
  --fixture online-boutique.ttl \
  --source Checkout \
  --destination Payment \
  --out ../experiments/outputs/processed/online-boutique-decision-trace.json
cd ../..
```

To regenerate the scale fixtures and processed stress CSV:

```sh
make -C implementation scale-stress
```

The target generates `scale-500.ttl`, `scale-1000.ttl` and `scale-2000.ttl` with seed
20260509 and edge factor 2.0, then runs `cmd/scale-stress` with 15 iterations.

To record bounded native TLS and controller scale/churn availability:

```sh
python3 implementation/experiments/scripts/run-pq-tls-benchmark.py \
  --out implementation/experiments/outputs/processed/pq-tls-benchmark-results.csv

python3 implementation/experiments/scripts/run-wire-tls-validation.py \
  --out implementation/experiments/outputs/processed/wire-tls-validation.csv

python3 implementation/experiments/scripts/run-controller-scale-churn.py \
  --scale-out implementation/experiments/outputs/processed/controller-scale-results.csv \
  --churn-out implementation/experiments/outputs/processed/controller-churn-results.csv
```
