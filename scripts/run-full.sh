#!/usr/bin/env bash
# Full local reproduction (about 2 to 4 minutes on a laptop after the Go modules are
# downloaded). It needs no cluster, no network and no credentials.
#
#  1. Evidence summary (run-evidence-summary.py), written to $OUT.
#  2. Deterministic runs: Go unit tests, H2 comparison (camera-ready Table 1 and the
#     Alibaba rows of Fig. 2(a)), weight sweep, mass staleness (Fig. 2(b)), H3
#     semantic-core stress, Online Boutique trace, canonical replay (H1 and modelled
#     values). The CSV files are compared byte for byte with the released ones.
#  3. Host-dependent runs: data-model microbenchmark (needs rdflib), OpenSSL
#     handshake timing and wire validation (need OpenSSL 3.5 or newer), OpenSSL
#     availability probe. check-claims.py compares their values with the paper.
#  4. Controller scale/churn rows (writes rows only, see REPRODUCIBILITY.md).
#  5. Coq/Rocq proofs when Rocq 9 is installed.
#  6. Fig. 2 (scripts/make_camera_figures.py, needs matplotlib) and the tables.
#  7. check-claims.py on the regenerated files (timing claims skipped) and on the
#     released files.
#
# The released result files are restored at the end. Regenerated files are kept in
# $QTKG_OUT (default: a new temporary folder, printed at the end).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
OUT="${QTKG_OUT:-$(mktemp -d "${TMPDIR:-/tmp}/qtkg-full.XXXXXX")}"
mkdir -p "$OUT"
# shellcheck source=common.sh
. "$ROOT/scripts/common.sh"
snapshot_released
trap restore_released EXIT
optional=()

step() { echo; echo "== $1"; }

step "[1/7] Evidence summary"
python3 implementation/experiments/scripts/run-evidence-summary.py \
  --out "$OUT/evidence-summary.json" --markdown-out "$OUT/evidence-summary.md" \
  || optional+=("evidence summary reported a failed step, see $OUT/evidence-summary.md")

step "[2/7] Deterministic runs"
make -C implementation controller-tests
make -s -C implementation baseline-compare weight-sweep staleness-sweep scale-stress decision-trace paper-results

step "[3/7] Host-dependent runs"
if python3 -c "import rdflib, networkx" >/dev/null 2>&1; then
  make -s -C implementation datamodel-benchmark
else
  optional+=("datamodel-benchmark skipped: rdflib/networkx missing (pip install -r requirements.txt)")
fi
if openssl list -tls-groups 2>/dev/null | grep -qi mlkem768; then
  make -s -C implementation pq-handshake wire-tls
else
  optional+=("pq-handshake and wire-tls skipped: OpenSSL without native ML-KEM (needs 3.5 or newer)")
fi
make -s -C implementation pq-tls

step "[4/7] Controller scale/churn rows"
make -s -C implementation controller-harness

step "[5/7] Coq/Rocq proofs"
coq_build

# draw_figure NAME draws Fig. 2 from the files currently in outputs/processed.
draw_figure() {
  if ! python3 -c "import matplotlib" >/dev/null 2>&1; then
    optional+=("Fig. 2 skipped: matplotlib missing (pip install -r requirements.txt)")
  elif command -v latex >/dev/null 2>&1; then
    python3 scripts/make_camera_figures.py --out "$OUT/$1"
  else
    python3 scripts/make_camera_figures.py --out "$OUT/$1" --no-tex
    optional+=("Fig. 2 drawn with --no-tex because latex is missing (fonts differ from the paper)")
  fi
}

step "[6/7] Fig. 2 and tables"
echo "Fig. 2 from the regenerated files (panel (c) solid bars use this host's handshake timing):"
draw_figure results-figure-regenerated.pdf
python3 scripts/make_tables.py --extended > "$OUT/tables.txt"

restore_released
trap - EXIT
echo "Fig. 2 from the released files (should look identical to Fig. 2 of the camera-ready):"
draw_figure results-figure.pdf

step "[7/7] Comparison"
status=0
deterministic=(baseline-comparison.csv baseline-comparison-real-alibaba.csv online-boutique-ablation.csv
  weight-sweep-results.csv staleness-sweep-results.csv staleness-sweep-real-alibaba.csv
  online-boutique-decision-trace.json fault-injection-results.csv blocked-safe-breakdown.csv
  compliance-wilson.csv latency-results.csv setup-cost-results.csv setup-cost-perturbation.csv
  oqs-calibration-bias.csv online-boutique-rq-stats.csv paper-results-summary.md)
if ! command -v kind >/dev/null 2>&1; then
  deterministic+=(controller-scale-results.csv controller-churn-results.csv)
fi
echo "Byte-for-byte comparison of deterministic outputs:"
compare_files processed "${deterministic[@]}" || status=1
compare_files fixtures online-boutique.ttl scale-500.ttl scale-1000.ttl scale-2000.ttl || status=1
echo "check-claims on the regenerated files, timing claims skipped:"
python3 scripts/check-claims.py --processed "$OUT/regenerated/processed" --skip-timing > "$OUT/claims-regenerated.txt" \
  || { cat "$OUT/claims-regenerated.txt"; status=1; }
tail -2 "$OUT/claims-regenerated.txt"
echo "check-claims on the regenerated files, all claims (timing values differ by host):"
python3 scripts/check-claims.py --processed "$OUT/regenerated/processed" > "$OUT/claims-regenerated-timing.txt" || true
grep -E "^[[:space:]]+\[FAIL\]" "$OUT/claims-regenerated-timing.txt" || true
echo "check-claims on the released files:"
python3 scripts/check-claims.py > "$OUT/claims-released.txt" || { cat "$OUT/claims-released.txt"; status=1; }
tail -2 "$OUT/claims-released.txt"
for note in "${optional[@]+"${optional[@]}"}"; do echo "NOTE: $note"; done
echo "Outputs: $OUT (regenerated/, results-figure.pdf, results-figure-regenerated.pdf, tables.txt, evidence-summary.md, claims-*.txt)"
if [ "$status" -ne 0 ]; then
  echo "FAIL: see the DIFF or FAIL lines above"
  exit 1
fi
echo "PASS: full local reproduction"
