# QuantumTrustKG Evidence Summary

Generated: 2026-09-24T17:53:22+00:00

| Step | Status | Note | Artifacts |
|---|---|---|---|
| controller_tests | passed |  |  |
| scale_stress_csv | passed |  | implementation/experiments/outputs/processed/scale-stress-results.csv |
| online_boutique_trace | passed |  | implementation/semantics/fixtures/online-boutique.ttl, implementation/experiments/outputs/processed/online-boutique-decision-trace.json |
| pq_tls_availability | unavailable | one or more OpenSSL/OQS groups are unavailable | implementation/experiments/outputs/processed/pq-tls-benchmark-results.csv |
| wire_tls_validation | passed |  | implementation/experiments/outputs/processed/wire-tls-validation.csv |
| controller_scale_churn_harness | skipped | kind is unavailable, so full-controller timing rows were not run | implementation/experiments/outputs/processed/controller-scale-results.csv, implementation/experiments/outputs/processed/controller-churn-results.csv |
| coq_rocq_checks | passed |  |  |
| paper_results_canonical_replay | passed |  | implementation/experiments/outputs/processed/fault-injection-results.csv, implementation/experiments/outputs/processed/blocked-safe-breakdown.csv, implementation/experiments/outputs/processed/compliance-wilson.csv, implementation/experiments/outputs/processed/latency-results.csv, implementation/experiments/outputs/processed/setup-cost-results.csv, implementation/experiments/outputs/processed/setup-cost-perturbation.csv, implementation/experiments/outputs/processed/oqs-calibration-bias.csv, implementation/experiments/outputs/processed/online-boutique-rq-stats.csv, implementation/experiments/outputs/processed/paper-results-summary.md |

Skipped or unavailable rows indicate missing local support such as kind,
OpenSSL/OQS groups, optional liboqs/Envoy images, or Coq/Rocq.
They are not extrapolated measurements.
