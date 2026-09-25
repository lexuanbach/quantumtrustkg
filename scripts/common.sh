# Helpers shared by run-smoke.sh, run-full.sh and run-full-evidence.sh.
#
# The Makefile targets and the Go commands write their outputs in place, into
# implementation/experiments/outputs/processed and implementation/semantics/fixtures.
# snapshot_released saves both folders before a run. restore_released copies the
# regenerated files to $OUT/regenerated, puts the released files back and removes
# Coq build products. The package therefore still matches MANIFEST.sha256 after a
# run, and the regenerated files stay available for comparison.
#
# The caller sets ROOT (artifact root) and OUT (output folder) before sourcing.

PROCESSED_REL=implementation/experiments/outputs/processed
FIXTURES_REL=implementation/semantics/fixtures
export PYTHONDONTWRITEBYTECODE=1

snapshot_released() {
  SNAP="$(mktemp -d "${TMPDIR:-/tmp}/qtkg-snapshot.XXXXXX")"
  cp -Rp "$ROOT/$PROCESSED_REL" "$SNAP/processed"
  cp -Rp "$ROOT/$FIXTURES_REL" "$SNAP/fixtures"
}

restore_released() {
  if [ -n "${SNAP:-}" ] && [ -d "$SNAP/processed" ]; then
    mkdir -p "$OUT/regenerated"
    rm -rf "$OUT/regenerated/processed" "$OUT/regenerated/fixtures"
    cp -Rp "$ROOT/$PROCESSED_REL" "$OUT/regenerated/processed"
    cp -Rp "$ROOT/$FIXTURES_REL" "$OUT/regenerated/fixtures"
    rm -rf "${ROOT:?}/$PROCESSED_REL" "${ROOT:?}/$FIXTURES_REL"
    cp -Rp "$SNAP/processed" "$ROOT/$PROCESSED_REL"
    cp -Rp "$SNAP/fixtures" "$ROOT/$FIXTURES_REL"
    rm -rf "$SNAP"
    SNAP=""
  fi
  find "$ROOT/formal/coq" \( -name '*.vo' -o -name '*.vok' -o -name '*.vos' -o -name '*.glob' \
    -o -name '.*.aux' -o -name '.lia.cache' \) -delete 2>/dev/null || true
}

# compare_files KIND NAME... compares regenerated files with the released ones.
# KIND is processed or fixtures. It prints SAME or DIFF per file and returns 1 on any DIFF.
compare_files() {
  local kind="$1" status=0 f released
  shift
  if [ "$kind" = processed ]; then released="$ROOT/$PROCESSED_REL"; else released="$ROOT/$FIXTURES_REL"; fi
  for f in "$@"; do
    if cmp -s "$OUT/regenerated/$kind/$f" "$released/$f"; then
      echo "  SAME  $kind/$f"
    else
      echo "  DIFF  $kind/$f"
      status=1
    fi
  done
  return $status
}

# coq_build compiles formal/coq with Rocq 9 (rocq compile, or coqc 9.x). It prints
# SKIPPED when only Coq 8 or no compiler is installed, since the sources use From Stdlib.
coq_build() {
  if command -v rocq >/dev/null 2>&1; then
    make -C "$ROOT/formal/coq" COQC="rocq compile"
  elif command -v coqc >/dev/null 2>&1 && coqc --version 2>&1 | grep -q 'version 9\.'; then
    make -C "$ROOT/formal/coq" COQC=coqc
  else
    echo "SKIPPED: Coq/Rocq 9.x not found (the proofs import From Stdlib and need Rocq 9)"
    return 0
  fi
}
