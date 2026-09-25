#!/usr/bin/env bash
# Smoke test of the artifact (about 1 minute on a laptop, plus a one-time Go module
# download of about 220 MB).
#
# 1. Go unit tests of the controller (gates behind G1 to G4, mesh and status writers).
# 2. H2 comparison of camera-ready Table 1 (make baseline-compare), compared byte for
#    byte with the released CSV files.
# 3. Online Boutique fixture and decision trace, compared byte for byte.
# 4. Canonical replay of the H1 and modelled values, compared byte for byte.
# 5. Coq/Rocq proofs when Rocq 9 is installed.
# 6. scripts/check-claims.py on the released result files.
#
# The released result files are restored at the end. Regenerated files are kept in
# $QTKG_OUT (default: a new temporary folder, printed at the end).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
OUT="${QTKG_OUT:-$(mktemp -d "${TMPDIR:-/tmp}/qtkg-smoke.XXXXXX")}"
mkdir -p "$OUT"
# shellcheck source=common.sh
. "$ROOT/scripts/common.sh"
snapshot_released
trap restore_released EXIT

echo "[1/6] Go unit tests"
make -C implementation/controller test

echo "[2/6] H2 comparison (camera-ready Table 1)"
make -s -C implementation baseline-compare

echo "[3/6] Online Boutique decision trace"
make -s -C implementation decision-trace

echo "[4/6] Canonical replay of H1 and modelled values"
make -s -C implementation paper-results

echo "[5/6] Coq/Rocq proofs"
coq_build

restore_released
trap - EXIT
echo "[6/6] Comparison with the released files"
status=0
compare_files processed baseline-comparison.csv baseline-comparison-real-alibaba.csv \
  online-boutique-ablation.csv online-boutique-decision-trace.json \
  fault-injection-results.csv blocked-safe-breakdown.csv compliance-wilson.csv latency-results.csv \
  setup-cost-results.csv setup-cost-perturbation.csv oqs-calibration-bias.csv \
  online-boutique-rq-stats.csv paper-results-summary.md || status=1
compare_files fixtures online-boutique.ttl || status=1
check_claims() {  # $1 = report file, remaining arguments go to check-claims.py
  local report="$1"
  shift
  if ! python3 scripts/check-claims.py "$@" > "$report"; then
    cat "$report"
    status=1
  fi
  tail -2 "$report"
}
echo "check-claims on the released files (report: $OUT/claims-released.txt):"
check_claims "$OUT/claims-released.txt"
echo "check-claims on the regenerated files, timing claims skipped (report: $OUT/claims-regenerated.txt):"
check_claims "$OUT/claims-regenerated.txt" --processed "$OUT/regenerated/processed" --skip-timing
python3 scripts/make_tables.py --processed "$OUT/regenerated/processed" > "$OUT/table1.txt"
echo "Regenerated Table 1 written to $OUT/table1.txt"
echo "Regenerated files: $OUT/regenerated"
if [ "$status" -ne 0 ]; then
  echo "FAIL: a deterministic output differs from the released file"
  exit 1
fi
echo "PASS: smoke test"
