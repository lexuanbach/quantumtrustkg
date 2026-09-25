# QuantumTrustKG implementation

Source code, inputs and result files of the artifact for *QuantumTrustKG: Provenance-Aware Orchestration for Quantum-Safe Microservices* (ICSOC 2026). The setup, the commands per paper item, the runtimes and the known differences between code and paper are in the top-level `README.md` and `REPRODUCIBILITY.md`. Run all commands from the artifact root.

## Layout

| Path | Purpose |
|---|---|
| `Makefile` | One target per experiment (`make -C implementation help`) |
| `controller/` | Go module: controller, H2 harness, H3 stress, decision trace, unit tests |
| `semantics/` | Ontology, SHACL shapes, SPARQL queries, Turtle fixtures |
| `tools/generators/` | Deterministic generators of the scale and Online Boutique fixtures |
| `experiments/baselines/`, `experiments/configs/` | Baseline and workload descriptions (documentation. No program reads them) |
| `experiments/fixtures/real-topologies/alibaba/` | Alibaba edge lists and `PROVENANCE.md` |
| `experiments/scripts/` | Python runners (OpenSSL, data model, canonical replay, evidence summary, scale/churn rows) |
| `experiments/canonical/` | Recorded H1 and cost-model values, replayed by `make paper-results` |
| `experiments/outputs/processed/` | Committed result files behind the paper numbers |
| `deploy/`, `api/` | Kubernetes/Istio manifests and CRDs (reference material, see README known differences 3 and 6) |
| `docs/` | Design notes from development |
| `plan.md` | Engineering plan from development (historical) |

## Make targets

| Target | Paper item | Output in `experiments/outputs/processed/` |
|---|---|---|
| `controller-tests` | unit tests of the gates behind G1 to G4 | console |
| `baseline-compare` | camera-ready Table 1, Fig. 2(a), Online Boutique ablation | `baseline-comparison.csv`, `baseline-comparison-real-alibaba.csv`, `online-boutique-ablation.csv` |
| `weight-sweep` | Sect. 5 weight sweep, extended Table A10 | `weight-sweep-results.csv` |
| `staleness-sweep` | Fig. 2(b), extended Table 7 | `staleness-sweep-results.csv`, `staleness-sweep-real-alibaba.csv` |
| `scale-stress` | extended Table 12 (H3, 500/1000/2000 services, 15 iterations) | `scale-stress-results.csv` and the three scale fixtures |
| `decision-trace` | extended Table A3 | `online-boutique-decision-trace.json` and `online-boutique.ttl` |
| `paper-results` | H1 and modelled values (replayed) | eight CSV files and `paper-results-summary.md` |
| `datamodel-benchmark` | extended Table A4 (needs rdflib and networkx) | `datamodel-benchmark.csv` |
| `pq-handshake` | Sect. 4.3 handshake medians (needs OpenSSL 3.5+) | `pq-tls-handshake-results.csv` |
| `wire-tls` | Sect. 4.3 30/30 wire trials (needs OpenSSL 3.5+) | `wire-tls-validation.csv` |
| `pq-tls` | availability probe (the paper does not use it) | `pq-tls-benchmark-results.csv` |
| `controller-harness` | status rows of the kind harness, never run | `controller-scale-results.csv`, `controller-churn-results.csv` |
| `coq` | proofs (needs Rocq 9) | compiled files in `formal/coq` |
| `evidence` | status report | `evidence-summary.json`, `evidence-summary.md` |
| `clean` | deletes the committed result files, avoid it | none |

Every target writes into the committed folder. `scripts/run-smoke.sh` and `scripts/run-full.sh` keep the committed files and put regenerated copies in a separate folder.

## Canonical replay

`experiments/scripts/run-paper-results.py` re-emits the values of `experiments/canonical/paper-canonical-results.json`: the archived in-cluster (H1) compliance, latency, fault-injection and blocked-safe values, the modelled setup cost, its perturbation envelope and calibration bias, and the Online Boutique statistics. These values were recorded with an earlier controller release. The per-repetition logs and the in-cluster driver are not part of the artifact. The record still uses the submission title and its section numbers (section 8 is the evaluation). `CLAIM_MATRIX.md` maps each value to the current paper.

## Limits of the prototype

- Stock Envoy/Istio enforces strict mTLS and carries the selected profile as metadata. It does not negotiate post-quantum TLS. The OpenSSL runners measure the TLS library on one host.
- Topology comes from the service annotations `quantumtrustkg.io/calls` and `quantumtrustkg.io/trust-boundary` (`controller/internal/controllers/inventory.go`).
- Capability provenance is operational (source, observation time, freshness window, contradiction note). Signed claims bound to SPIFFE/SPIRE identities are future work.
- Missing tools (`kind`, OpenSSL with ML-KEM, Rocq 9) are reported as unavailable or skipped and never turned into numbers.
