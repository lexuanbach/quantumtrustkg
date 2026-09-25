# Reproducibility

This file gives the environment, the exact commands, the runtimes measured on the test host, what is deterministic, what cannot be rerun, and how to check outputs. `README.md` has the short version and `CLAIM_MATRIX.md` maps every paper number to its file.

## 1. Test environment

| Item | Value |
|---|---|
| OS | macOS 26.5.2 (build 25F84), arm64 |
| Hardware | Apple M5 Max, 18 cores, 64 GB RAM |
| Go | 1.26.1 (`go.mod` requires 1.22, dependencies pinned by `go.sum`, controller-runtime 0.19.4, apimachinery 0.31.0) |
| Python | 3.14.6 with the packages of `requirements.txt` (matplotlib 3.10.9, rdflib 7.6.0, networkx 3.6.1) |
| SQLite (Python stdlib) | 3.53.2 |
| OpenSSL | 3.6.2 (7 Apr 2026), default provider, native ML-KEM |
| Rocq | 9.1.0 (`rocq compile`). The paper states 9.1.1. Coq 8.20 fails on `From Stdlib` as expected |
| LaTeX | TeX Live 2020 (used by `make_camera_figures.py` for Computer Modern text) |
| Make, bash | GNU Make 3.81, bash 3.2.57 |

Linux was not tested. The scripts use only POSIX tools, GNU Make, bash, Go, Python and OpenSSL.

## 2. Setup

From the artifact root:

```sh
python3 -m venv ../qtkg-venv                       # outside the artifact folder
. ../qtkg-venv/bin/activate
pip install -r requirements.txt                    # 6 s
(cd implementation/controller && go mod download)  # 6 s, about 220 MB, needs network once
```

Optional: put Rocq 9 on `PATH` (for example `export PATH=$HOME/.opam/<switch>/bin:$PATH`). `formal/coq/Makefile` uses `rocq compile` when `rocq` is on `PATH`, otherwise `coqc`.

## 3. Commands and runtimes

Times are wall-clock on the test host with warm Go build caches unless marked cold. Expect a few times longer on an older laptop.

| Step | Command | Time | Output |
|---|---|---|---|
| Package check | `bash scripts/verify.sh` | under 1 s | PASS lines, see `EXPECTED_RESULTS.md` |
| Smoke test | `bash scripts/run-smoke.sh` | 15 to 17 s cold, 6 s warm | SAME lines and `PASS: smoke test` |
| Claim check | `python3 scripts/check-claims.py` | under 1 s | `Claims: 74 ... FAIL 0` |
| Full local run | `bash scripts/run-full.sh` | 30 s cold, 25 s warm | SAME lines and `PASS: full local reproduction` |
| Status report | `bash scripts/run-full-evidence.sh` | 3 s | `evidence-summary.md` in the output folder |
| Unit tests | `make -C implementation/controller test` | 3 s warm, 10 s cold | `ok` for two packages |
| H2 comparison (Table 1, Alibaba, Online Boutique ablation) | `make -C implementation baseline-compare` | 1 s | three CSV files |
| Weight sweep | `make -C implementation weight-sweep` | under 1 s | `weight-sweep-results.csv` |
| Mass staleness | `make -C implementation staleness-sweep` | 1 s | two CSV files |
| H3 stress | `make -C implementation scale-stress` | 1 s | fixtures `scale-{500,1000,2000}.ttl` and `scale-stress-results.csv` |
| Decision trace | `make -C implementation decision-trace` | under 1 s | fixture `online-boutique.ttl` and the trace JSON |
| Canonical replay | `make -C implementation paper-results` | under 1 s | eight CSV files and `paper-results-summary.md` |
| Data-model benchmark | `make -C implementation datamodel-benchmark` | 6 s | `datamodel-benchmark.csv` |
| Handshake timing | `make -C implementation pq-handshake` | 10 s | `pq-tls-handshake-results.csv` |
| Wire validation | `make -C implementation wire-tls` | 2 s | `wire-tls-validation.csv` |
| Availability probe | `make -C implementation pq-tls` | under 1 s | `pq-tls-benchmark-results.csv` |
| Scale/churn rows | `make -C implementation controller-harness` | under 1 s | two CSV files with `not-run-kind-unavailable` |
| Proofs | `make -C formal/coq` | 1 s | four `.vo` files (`make -C formal/coq clean` removes them) |
| Fig. 2 | `python3 scripts/make_camera_figures.py --out /tmp/fig2.pdf` | 4 s | PDF |
| Tables | `python3 scripts/make_tables.py --extended` | under 1 s | Table 1, extended Tables 6 and 7 as text |

The single `make` targets overwrite the committed files in `implementation/experiments/outputs/processed/` and `implementation/semantics/fixtures/`. The three `run-*.sh` scripts save those folders first, keep the regenerated copies in `$QTKG_OUT/regenerated/` (default: a new temporary folder), and restore the released files at the end.

## 4. What is deterministic

- **Byte-identical on rerun:** `baseline-comparison.csv`, `baseline-comparison-real-alibaba.csv`, `online-boutique-ablation.csv`, `weight-sweep-results.csv`, `staleness-sweep-results.csv`, `staleness-sweep-real-alibaba.csv`, `online-boutique-decision-trace.json`, the eight replayed files, the two scale/churn status files (when `kind` is absent), and the fixtures `online-boutique.ttl` and `scale-{500,1000,2000}.ttl`. `run-full.sh` checks each of them with `cmp`.
- **Seeds:** seed 20260509 everywhere. The H2 harness uses sub-seed seed + 7919 x repetition + number of services for each mesh, and the weight sweep draws the existing active profiles with seed + 104729 x repetition + number of services. The harness clock is fixed at 2026-05-09 12:00 UTC.
- **Host-dependent:** all millisecond values (H3 stress, data-model benchmark, handshake medians and overheads, wire p50/p95) and the version strings in those files. Counts in the same files (promoted/blocked edges, triples, admissible edges, successful trials) are stable.
- **Handshake overhead variance.** Each handshake sample is the wall-clock time of one `openssl s_client` process. It includes process start-up (a bare `openssl version` takes 3.4 ms on the test host), certificate loading, connection set-up and teardown, which are large compared with the key-exchange difference. On the test host, reruns of `run-pq-tls-handshake.py --handshakes 300` gave hybrid overheads of +0.1%, +0.7%, +6.2% and +1.5%, and pure post-quantum overheads of -2.2%, 0.0%, +3.9% and -0.2%, while other jobs were running. The committed file (+1.2%, +0.2%) is one such run. Expect medians in the range of 8 to 11 ms and overheads of a few percent in either direction.

## 5. What cannot be rerun from this package

| Item | Why | What stands in for it |
|---|---|---|
| H1 in-cluster runs (Sect. 4.1 Wilson intervals, latency, rollout outcomes) | Recorded with an earlier controller release on Kubernetes, Fuseki and Istio. Driver and per-repetition logs are not included | `paper-canonical-results.json`, re-emitted by `make paper-results` |
| Modelled aggregate setup cost and its envelope | Model calibration inputs are recorded as aggregate ranges only | same canonical record |
| Full-controller scale/churn under `kind` | Never run for the paper | status rows `not-run-kind-unavailable` |
| Alibaba edge extraction | The extraction script is not included. The raw bucket is 223 MB (`PROVENANCE.md`) | committed `alibaba_*.edges` files |
| Post-quantum negotiation inside Envoy/Istio | Not claimed by the paper | OpenSSL client/server runs |

No step needs an LLM, a GPU, an API key or a credential.

## 6. Checking outputs

1. `python3 scripts/check-claims.py` compares 74 claim groups (414 values) of the camera-ready and extended papers with the committed files. It exits 1 on any mismatch. All 74 pass on this release. The script can also mark a known difference as WARN or a value without a result file as UNTRACED, with a note.
2. `python3 scripts/check-claims.py --processed <folder> --skip-timing` checks a regenerated result folder. `run-smoke.sh` and `run-full.sh` do this for their output folder.
3. `EXPECTED_RESULTS.md` lists the expected console output of each script.
4. Fig. 2: open `$QTKG_OUT/results-figure.pdf` from `run-full.sh` (drawn from the released files) next to Fig. 2 on page 7 of `paper/camera-ready.pdf`. With LaTeX installed the 200 dpi renders are identical. `results-figure-regenerated.pdf` uses this host's handshake timing in panel (c).

## 7. Fresh-copy test (2026-09-25)

The archive was checked against its `.sha256` file and unpacked into an empty folder. The test used a new virtual environment next to the artifact folder, an empty Go module cache and an empty Go build cache, and Rocq 9.1.0 on `PATH`. Every step passed.

| Command | Time | Outcome |
|---|---|---|
| `python3 -m venv ../qtkg-venv`, `pip install -r requirements.txt` | 7 s | matplotlib 3.10.9, rdflib 7.6.0, networkx 3.6.1 |
| `(cd implementation/controller && go mod download)` | 6 s | about 226 MB module cache |
| `bash scripts/verify.sh` | 0.2 s | 194 manifest entries, hygiene PASS, claims FAIL 0, camera-ready has 10 pages |
| `bash scripts/run-smoke.sh` | 15.3 s | 60 Go tests pass, 14 files SAME, `PASS: smoke test` |
| `python3 scripts/check-claims.py` | 0.1 s | 74 claim groups, 414 values, PASS 74, FAIL 0 |
| `bash scripts/run-full.sh` | 29.7 s | 22 files SAME, regenerated claims PASS 65 and FAIL 0 with 9 timing claims skipped, `PASS: full local reproduction` |
| `bash scripts/verify.sh` (after the runs) | 0.2 s | PASS, the runs left the package unchanged |
| `bash scripts/run-full-evidence.sh` | 2.9 s | 6 passed, 1 unavailable (`pq_tls_availability`), 1 skipped (kind) |
| `go build ./...`, `go vet ./...`, `go test -count=1 ./tests/unit/... ./internal/...` in `implementation/controller` | under 4 s each | no vet findings, 60 tests pass, `go.mod` and `go.sum` unchanged |
| `make -C formal/coq` | 0.3 s | four files compile. `Print Assumptions` on G1 to G4 reports "Closed under the global context" |
| Fig. 2 from the released files | 4 s | identical to Fig. 2 of the camera-ready at 200 dpi |

## 8. Packaging

`MANIFEST.sha256` lists the SHA-256 of every file except itself, sorted by path, in `sha256sum` format. `python3 scripts/check-manifest.py` checks it. The archive `QuantumTrustKG-ICSOC2026.zip` holds the folder `QuantumTrustKG-ICSOC2026/` with files only (no directory entries), sorted by path, with every file time set to 2026-09-24 00:00 (UTC+7), and was written by Info-ZIP 3.0 as `TZ=UTC zip -X -D`.
