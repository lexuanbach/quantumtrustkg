#!/usr/bin/env python3
from pathlib import Path
import sys

root = Path(__file__).resolve().parents[1]
bad_dirs = {'.git','.hg','.svn','__pycache__','.pytest_cache','.mypy_cache','.gocache','.venv','.venv_ad','venv','node_modules'}
bad_suffixes = ('.aux','.bbl','.bcf','.blg','.fdb_latexmk','.fls','.glob','.log','.out','.run.xml','.synctex.gz','.toc','.vo','.vok','.vos','.pyc')
bad_names = {'.DS_Store'}
bad = []
local = []
for p in root.rglob('*'):
    rel = p.relative_to(root)
    if rel.parts and rel.parts[0] == '.git':
        continue
    approved_log = rel.as_posix() == 'deon/results/ladder.log'
    if any(part in bad_dirs for part in rel.parts) or p.name in bad_names or (p.is_file() and p.name.endswith(bad_suffixes) and not approved_log):
        bad.append(str(rel))
    if p.is_file() and rel.as_posix() != 'scripts/check-hygiene.py' and p.stat().st_size <= 10_000_000:
        try: text = p.read_text(encoding='utf-8')
        except (UnicodeDecodeError, OSError): continue
        if any(m in text for m in ('/Users/', '/home/', 'Desktop/', '/private/tmp/', '/private/var/')):
            local.append(str(rel))
if bad or local:
    for x in bad: print('FORBIDDEN:', x)
    for x in local: print('LOCAL PATH:', x)
    sys.exit(1)
print('PASS: package hygiene and local-path checks')
