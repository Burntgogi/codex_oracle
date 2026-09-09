"""Transactional file installation used by install-personal.sh."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import sys
import uuid


def install(source, target, marketplace, name, template, force):
    source, target, marketplace = map(Path, (source, target, marketplace))
    source, target = source.resolve(), target.resolve()
    if source == target or source in target.parents or target in source.parents:
        raise ValueError('Plugin source and destination must not overlap.')
    if target.exists() and not force:
        raise ValueError('Target already exists. Re-run with --force after approval.')
    data = json.loads(marketplace.read_text(encoding='utf-8-sig')) if marketplace.exists() else {}
    entry = {'name': 'codex_oracle', 'source': {'source': 'local', 'path': './plugins/codex_oracle'},
             'policy': {'installation': 'AVAILABLE', 'authentication': 'ON_INSTALL'}, 'category': 'Productivity'}
    plugins, replaced = [], False
    for plugin in data.get('plugins', []):
        if plugin.get('name') != 'codex_oracle':
            plugins.append(plugin)
        elif not replaced:
            plugins.append(entry)
            replaced = True
    if not replaced:
        plugins.append(entry)
    data.update(name=data.get('name') or name, interface=data.get('interface') or {'displayName': 'Personal'}, plugins=plugins)
    payload = json.dumps(data, indent=2) + '\n'
    token = uuid.uuid4().hex
    stage = target.parent / ('.oracle-stage-' + token)
    backup = target.parent / ('.oracle-backup-' + token)
    pending = marketplace.parent / ('.oracle-marketplace-' + token + '.json')
    market_backup = marketplace.parent / ('.oracle-marketplace-backup-' + token + '.json')
    roots = {'.codex-plugin', 'assets', 'bin', 'doc', 'docs', 'scripts', 'skills', '.mcp.json',
             '.mcp.windows.json', '.mcp.macos-amd64.json', '.mcp.macos-arm64.json', 'README.md',
             'README.en.md', 'RELEASE_NOTES.md', 'RELEASE_NOTES.en.md', 'CHANGELOG.md', 'LICENSE',
             'SECURITY.md', 'SHA256SUMS.txt'}
    moved_old = installed_new = False
    try:
        stage.mkdir(parents=True)
        for item in source.iterdir():
            if item.name not in roots:
                continue
            if item.is_symlink() or (item.is_dir() and any(p.is_symlink() for p in item.rglob('*'))):
                raise ValueError('Symlink source is not supported.')
            if item.is_dir():
                shutil.copytree(item, stage / item.name)
            else:
                shutil.copy2(item, stage / item.name)
        for file in stage.rglob('*'):
            if file.is_file():
                original = source / file.relative_to(stage)
                if hashlib.sha256(file.read_bytes()).digest() != hashlib.sha256(original.read_bytes()).digest():
                    raise ValueError('Staged file differs from source.')
        json.loads((stage / '.codex-plugin/plugin.json').read_text(encoding='utf-8-sig'))
        json.loads((stage / template).read_text(encoding='utf-8-sig'))
        shutil.copy2(stage / template, stage / '.mcp.json')
        for binary in (stage / 'bin').glob('codex_oracle*'):
            binary.chmod(binary.stat().st_mode | 0o111)
        marketplace.parent.mkdir(parents=True, exist_ok=True)
        pending.write_text(payload, encoding='utf-8')
        if marketplace.exists():
            shutil.copy2(marketplace, market_backup)
        if target.exists():
            target.rename(backup)
            moved_old = True
        stage.rename(target)
        installed_new = True
        os.replace(pending, marketplace)
    except BaseException:
        if installed_new:
            shutil.rmtree(target)
        if moved_old:
            backup.rename(target)
        raise
    finally:
        if stage.exists():
            shutil.rmtree(stage)
        if pending.exists():
            pending.unlink()
    if moved_old:
        print(f'Previous installation retained at {backup}')
    print(f'Installed codex_oracle plugin files to {target}')
    print(f'Next: codex plugin add codex_oracle@{data["name"]}')


if __name__ == '__main__':
    install(*sys.argv[1:6], force=sys.argv[6] == '1')
