#!/usr/bin/env bash
# Whole-artifact status report (extended version, Table A1, last row).
#
# Runs implementation/experiments/scripts/run-evidence-summary.py. It runs the
# controller tests, the H3 scale-stress command, the Online Boutique trace, the
# OpenSSL availability probe, the wire validation, the controller scale/churn rows,
# the Coq/Rocq build and the canonical replay. Each step ends as passed, failed,
# skipped or unavailable. A missing tool is reported and never turned into a number.
# The summary does not run the H2 harnesses of Table 1 or Fig. 2 (use run-full.sh).
#
# The report is written to $QTKG_OUT/evidence-summary.json and .md (default: a new
# temporary folder). The released result files, which the summary steps overwrite,
# are restored at the end. The released evidence-summary.json and .md in
# implementation/experiments/outputs/processed are the report of the packaging run.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
OUT="${QTKG_OUT:-$(mktemp -d "${TMPDIR:-/tmp}/qtkg-evidence.XXXXXX")}"
mkdir -p "$OUT"
# shellcheck source=common.sh
. "$ROOT/scripts/common.sh"
snapshot_released
trap restore_released EXIT

rc=0
python3 implementation/experiments/scripts/run-evidence-summary.py \
  --out "$OUT/evidence-summary.json" --markdown-out "$OUT/evidence-summary.md" || rc=$?
echo
cat "$OUT/evidence-summary.md"
echo "Report: $OUT/evidence-summary.md"
exit "$rc"
