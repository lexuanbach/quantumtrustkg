# Experiment assets

Inputs, runner scripts and result files of the evaluation. The commands per paper item are in the top-level `README.md`, and `CLAIM_MATRIX.md` maps every paper number to a file here. Run commands from the artifact root.

## Layout

- `baselines/`, `configs/`: descriptors of the baselines and of the finance workload. No program reads them. The baseline selection rules are functions in `controller/cmd/baseline-compare/main.go`.
- `fixtures/real-topologies/alibaba/`: edge lists sampled from the Alibaba 2022 microservices trace (`PROVENANCE.md`).
- `canonical/paper-canonical-results.json`: recorded H1 and cost-model values.
- `scripts/`:
  - `run-paper-results.py`: re-emits the canonical record as eight CSV files and `paper-results-summary.md` (`make paper-results`).
  - `run-pq-tls-handshake.py`: median TLS 1.3 handshake time for X25519, X25519MLKEM768 and MLKEM768 (`make pq-handshake`, 300 handshakes). Source of the Sect. 4.3 medians.
  - `run-wire-tls-validation.py`: 30 client/server trials per group, negotiated group read back (`make wire-tls`). Source of the 30/30 trials.
  - `run-pq-tls-benchmark.py`: availability probe (`make pq-tls`). Not used by the paper, and it reports ML-KEM as unavailable on OpenSSL 3.5+ (README known difference 7).
  - `run-datamodel-benchmark.py`: the same admissibility join in SQLite, networkx and rdflib (`make datamodel-benchmark`).
  - `run-controller-scale-churn.py`: status rows of the kind harness, which was never run (`make controller-harness`).
  - `run-evidence-summary.py`: status report of all checks (`scripts/run-full-evidence.sh`).
  - `run-fixture.sh`: one reconciliation pass of the controller over a fixture without a cluster.
- `outputs/processed/`: committed result files.

## Result files

| File | Producer | Paper item |
|---|---|---|
| `baseline-comparison.csv` | `make baseline-compare` | camera-ready Table 1 (row scale=200), extended Table 4 |
| `baseline-comparison-real-alibaba.csv` | `make baseline-compare` | Fig. 2(a), extended Table 6 |
| `online-boutique-ablation.csv` | `make baseline-compare` | extended Sect. 8.8 (NoKG/NoProv 3.6%) |
| `weight-sweep-results.csv` | `make weight-sweep` | camera-ready Sect. 5, extended Table A10 |
| `staleness-sweep-results.csv`, `staleness-sweep-real-alibaba.csv` | `make staleness-sweep` | Fig. 2(b), extended Table 7 |
| `scale-stress-results.csv` | `make scale-stress` | extended Table 12, Fig. 3(c) |
| `online-boutique-decision-trace.json` | `make decision-trace` | extended Table A3 |
| `compliance-wilson.csv`, `latency-results.csv` | `make paper-results` (replay) | camera-ready Sect. 4.1, extended Tables 3 and 11, Fig. 3(a,b) |
| `setup-cost-results.csv`, `setup-cost-perturbation.csv`, `oqs-calibration-bias.csv` | `make paper-results` (replay) | camera-ready Sect. 4.3 and Fig. 2(c), extended Tables 8 and 9 |
| `fault-injection-results.csv`, `blocked-safe-breakdown.csv` | `make paper-results` (replay) | extended Tables 13 and 14 |
| `online-boutique-rq-stats.csv` | `make paper-results` (replay) | extended Sect. 8.8 and Sect. 9 |
| `pq-tls-handshake-results.csv` | `make pq-handshake` | camera-ready Sect. 4.3 and Fig. 2(c), extended Table 10 |
| `wire-tls-validation.csv` | `make wire-tls` | camera-ready Sect. 4.3, extended Table 10 |
| `datamodel-benchmark.csv` | `make datamodel-benchmark` | extended Table A4 |
| `controller-scale-results.csv`, `controller-churn-results.csv` | `make controller-harness` | extended Table A2 (not run) |
| `pq-tls-benchmark-results.csv` | `make pq-tls` | not used |
| `paper-results-summary.md` | `make paper-results` | index of the replayed files |
| `evidence-summary.json`, `evidence-summary.md` | `scripts/run-full-evidence.sh` | extended Table A1, status report |
