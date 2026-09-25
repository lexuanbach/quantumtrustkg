#!/usr/bin/env python3
from pathlib import Path
import hashlib, sys
root = Path(__file__).resolve().parents[1]
manifest = root / 'MANIFEST.sha256'
expected = {}
for line in manifest.read_text(encoding='utf-8').splitlines():
    if not line.strip(): continue
    digest, rel = line.split('  ', 1)
    expected[rel] = digest
actual_files = {p.relative_to(root).as_posix() for p in root.rglob('*') if p.is_file() and p != manifest and p.relative_to(root).parts[0] != '.git'}
ok = actual_files == set(expected)
if not ok:
    for x in sorted(actual_files-set(expected)): print('UNMANIFESTED:', x)
    for x in sorted(set(expected)-actual_files): print('MISSING:', x)
for rel, digest in sorted(expected.items()):
    p = root / rel
    if p.is_file():
        got = hashlib.sha256(p.read_bytes()).hexdigest()
        if got != digest: print('HASH MISMATCH:', rel); ok = False
print(f"{'PASS' if ok else 'FAIL'}: {len(expected)} manifest entries")
sys.exit(0 if ok else 1)
