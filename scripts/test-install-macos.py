"""Exercise the macOS install transaction on the host filesystem (not native macOS validation)."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('installer', Path(__file__).with_name('install-macos.py'))
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)


class InstallTest(unittest.TestCase):
    def test_install_update_and_rollback(self):
        with tempfile.TemporaryDirectory(prefix='oracle-mac-install-') as temp:
            root = Path(temp)
            source, target, market = root / 'source', root / 'plugins/codex_oracle', root / 'marketplace.json'
            (source / '.codex-plugin').mkdir(parents=True)
            (source / 'bin').mkdir()
            (source / '.codex-plugin/plugin.json').write_text('{"name":"codex_oracle"}')
            (source / '.mcp.macos-arm64.json').write_text('{}')
            (source / 'bin/fixture').write_text('old')
            (source / '.env').write_text('private fixture')
            market.write_text(json.dumps({'name':'personal', 'custom':42, 'plugins':[{'name':'before'}, {'name':'codex_oracle'}, {'name':'after'}]}))
            args = (source, target, market, 'personal', '.mcp.macos-arm64.json')
            installer.install(*args, force=False)
            self.assertFalse((target / '.env').exists())
            self.assertEqual([p['name'] for p in json.loads(market.read_text())['plugins']], ['before','codex_oracle','after'])
            self.assertEqual(json.loads(market.read_text())['custom'],42)
            original_market = market.read_bytes()
            (source / 'bin/fixture').write_text('new')
            # Failure occurs after real staging and directory replacement.
            with patch.object(installer.os, 'replace', side_effect=OSError('simulated marketplace write failure')):
                with self.assertRaises(OSError):
                    installer.install(*args, force=True)
            self.assertEqual((target / 'bin/fixture').read_text(), 'old')
            self.assertEqual(market.read_bytes(), original_market)
            self.assertFalse(list(target.parent.glob('.oracle-stage-*')))
            installer.install(*args, force=True)
            self.assertEqual((target / 'bin/fixture').read_text(), 'new')
            self.assertEqual(len(list(target.parent.glob('.oracle-backup-*'))),1)
            with self.assertRaisesRegex(ValueError, 'overlap'):
                installer.install(target, target, market, 'personal', '.mcp.macos-arm64.json', True)


if __name__ == '__main__':
    unittest.main()
