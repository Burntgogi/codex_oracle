"""Scan public Git files (or an explicit package directory); never print matched values."""
import argparse
import re
import subprocess
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parents[1])
parser.add_argument('--package', action='store_true', help='scan every package file, without Git exclusions')
args = parser.parse_args()
root = args.root.resolve()
patterns = {
    'machine path': rb'(?i)[A-Z]:[\\/](?:Users|Work)[\\/][A-Za-z0-9_-]+',
    'durable private conversation': rb'https://chatgpt\.com/(?:g/[^/\s]+/)?c/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}',
    'credential': rb'(?:sk-(?:proj-|svcacct-)?[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[A-Z0-9]{16}|-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----)',
}
if args.package:
    paths = sorted(p for p in root.rglob('*') if p.is_file())
else:
    raw = subprocess.check_output(['git', '-C', str(root), 'ls-files', '--cached', '--others', '--exclude-standard', '-z'])
    paths = sorted({root / p.decode() for p in raw.split(b'\0') if p})
failures = []
checked = 0
for path in paths:
    if not path.is_file():
        continue
    relative = path.relative_to(root).as_posix()
    if any(part in {'.work', 'sessions', '.codex_oracle'} for part in path.relative_to(root).parts) or path.name in {'auth.json', 'cookies.json', 'config.toml', '.env'}:
        failures.append((relative, 'private artifact'))
    data = path.read_bytes()
    checked += 1
    for label, pattern in patterns.items():
        if re.search(pattern, data):
            failures.append((relative, label))
for path, label in failures:
    print(f'FAIL {path}: {label}')
print(f'{checked} public files scanned; {len(failures)} findings. Pattern scan only; history is not scanned.')
raise SystemExit(bool(failures))
