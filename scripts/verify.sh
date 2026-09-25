#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
export PYTHONDONTWRITEBYTECODE=1
python3 scripts/check-manifest.py
python3 scripts/check-hygiene.py
python3 scripts/check-claims.py
for pdf in paper/camera-ready.pdf paper/extended.pdf; do
  test -s "$pdf"
done
if command -v pdfinfo >/dev/null 2>&1; then
  pages="$(pdfinfo paper/camera-ready.pdf | awk '/^Pages:/{print $2}')"
  test "$pages" -le 10
  echo "PASS: camera-ready PDF has $pages pages"
else
  echo "SKIPPED: pdfinfo unavailable; PDF existence was checked"
fi
echo "PASS: artifact verification complete"
