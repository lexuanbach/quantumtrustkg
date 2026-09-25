# Expected results

Output folders below are the temporary folders printed by the scripts (or `$QTKG_OUT`).

## `bash scripts/verify.sh`

```
PASS: 194 manifest entries
PASS: package hygiene and local-path checks
Claims: 74 (414 values checked). PASS 74, WARN 0, UNTRACED 0, SKIP 0, FAIL 0
PASS: camera-ready PDF has 10 pages
PASS: artifact verification complete
```

## `bash scripts/run-smoke.sh`

- Step 1: `ok` for `internal/controllers` and `tests/unit` (60 Go tests in total).
- Step 5: four `rocq compile` lines, or `SKIPPED: Coq/Rocq 9.x not found`.
- Step 6: `SAME` for 13 result files and for `fixtures/online-boutique.ttl`, then the claim check on the released files (`PASS 74, WARN 0, UNTRACED 0, SKIP 0, FAIL 0`), the check on the regenerated files (`PASS 65, WARN 0, UNTRACED 0, SKIP 9, FAIL 0`) and `PASS: smoke test`.
- `table1.txt` in the output folder equals camera-ready Table 1:

```
                             QTPO       NKG/NP          OPP  ConfigProf.       Static         ELCA        SL-CA
Valid (%)                    96.5         96.5         96.5         85.7         86.7         86.1         85.7
Invalid (%)                   0.0          2.5          3.4         11.2         13.1         13.5         13.7
Invalid n                       0          303          403         1336         1557         1614         1640
No promotion n                403          100            0          304            0            0            0
Retained-active n               0            0            0            0            0            0            0
Pooled observations 11929 (30 repetitions)
```

## `bash scripts/run-full.sh`

- `SAME` for 18 result files (16 when `kind` is installed) and for the four fixtures `online-boutique.ttl`, `scale-500.ttl`, `scale-1000.ttl`, `scale-2000.ttl`.
- `check-claims` on the regenerated files with timing skipped: `Claims: 74 (369 values checked). PASS 65, WARN 0, UNTRACED 0, SKIP 9, FAIL 0`.
- The second regenerated check (all claims) lists the timing claims that differ on this host: C30, C31, C34 (handshake), E08 (wire p50), E11 (H3 timings), E20 (data-model timings). This is expected. Values in the neighbourhood of the paper are normal: handshake medians 8 to 11 ms with overheads of a few percent either way, wire p50 9 to 11 ms, H3 totals 1 to 6 ms, rdflib query about 1 s.
- `results-figure.pdf` looks the same as Fig. 2 of the camera-ready (identical at 200 dpi with LaTeX). `results-figure-regenerated.pdf` differs only in the two solid bars of panel (c).
- `tables.txt` holds Table 1 and extended Tables 6 and 7 (Table 7 also shows the 50% row, which the paper omits).
- `evidence-summary.md`: `controller_tests`, `scale_stress_csv`, `online_boutique_trace`, `wire_tls_validation`, `coq_rocq_checks`, `paper_results_canonical_replay` passed, `pq_tls_availability` unavailable (known probe limitation, README item 7), `controller_scale_churn_harness` skipped.
- Last line: `PASS: full local reproduction`.

## Values that must match (from the committed files)

| Paper item | Value |
|---|---|
| Table 1, QTPO | 96.5% valid, 0 invalid, 403 not promoted, 0 retained, 11,929 observations |
| Table 1, baselines invalid | NKG/NP 2.5, OPP 3.4, ConfigProf. 11.2, Static 13.1, ELCA 13.5, SL-CA 13.7 (%) |
| Fig. 2(a) | QTPO 0.00 at all six scales, SL-CA 59.87 to 40.15, ConfigProf. 56.60 to 37.19, hub share 98.0 to 54.1% |
| Fig. 2(b) | QTPO 0 unsafe at every level, NoKG/NoProv 24.75/49.59/89.22% at 25/50/90%, C_decl 99.03 to 9.87% |
| Fig. 2(c) | measured 1.2 and 0.2 (host-dependent), modelled 8-12 and 12-15 |
| Sect. 4.1 H1 (replayed) | 99.4 [99.1, 99.6], 99.3 [99.0, 99.5], 99.1 [98.9, 99.3], NoKG/NoProv 96.9, 96.6, 96.4, latency 12 to 45 ms |
| Sect. 4.3 | medians 8.746/8.850/8.759 ms (host-dependent), 30/30 trials, rollout 4-7 and 8-12 min, 0.0/0.0 and 1.1/0.6% |
| Sect. 5 | selection change up to 20.48% of promoted edges, C_decl 96.47%, 0 unsafe at every sweep point |
| Proofs | G1_promoted_admissible, G2_security_class_preserved, G3_hybrid_metadata_preserved, G4_fail_closed compile with no axioms |

## When a tool is missing

- No Rocq 9: `SKIPPED: Coq/Rocq 9.x not found` and the rest passes.
- OpenSSL older than 3.5: `run-full.sh` prints a NOTE and skips the handshake and wire runs. The committed files stay in use.
- No matplotlib or rdflib: a NOTE, and the figure or data-model run is skipped.
- No LaTeX: Fig. 2 is drawn with `--no-tex`. Fonts differ and the numbers are the same.
