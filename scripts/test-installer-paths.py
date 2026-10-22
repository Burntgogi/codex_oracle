"""Regression tests using synthetic plugins and marketplaces only."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("installer_paths", SCRIPTS / "install-macos.py")
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)


class InstallerPathsTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="oracle-paths-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root / "source"
        (self.source / ".codex-plugin").mkdir(parents=True)
        (self.source / ".codex-plugin/plugin.json").write_text(
            '{"name":"codex_oracle","interface":{"category":"Productivity"}}', encoding="utf-8")
        for template in (".mcp.windows.json", ".mcp.macos-arm64.json"):
            (self.source / template).write_text("{}", encoding="utf-8")

    def invoke(self, kind, target, market):
        if kind == "python":
            installer.install(self.source, target, market, "fixture", ".mcp.macos-arm64.json", False)
            return json.loads(market.read_text(encoding="utf-8"))
        script = "marketplace-plan.ps1" if kind == "plan" else "install-personal.ps1"
        args = ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(SCRIPTS / script),
                "-PluginRoot", str(self.source), "-MarketplacePath", str(market),
                "-PluginDestinationRoot", str(target.parent), "-MarketplaceName", "fixture"]
        args.append("-AsJson" if kind == "plan" else "-Apply")
        # Do not inherit PowerShell 7's module path when launching Windows PowerShell.
        environment = {key: value for key, value in os.environ.items() if key.upper() != "PSMODULEPATH"}
        result = subprocess.run(args, capture_output=True, text=True, env=environment)
        if result.returncode:
            raise ValueError(result.stderr)
        if kind == "plan":
            return json.loads(result.stdout)["proposedMarketplace"]
        return json.loads(market.read_text(encoding="utf-8-sig"))

    def check_paths(self, kind):
        for index, (layout, relative) in enumerate((
            (".agents/plugins/marketplace.json", "plugins/codex_oracle"),
            (".agents/plugins/marketplace.json", "custom folder/nested/codex_oracle"),
            ("marketplace.json", "custom folder/codex_oracle"),
        )):
            with self.subTest(kind=kind, layout=layout, relative=relative):
                market_root = self.root / str(index)
                market = market_root / layout
                target = market_root / relative
                result = self.invoke(kind, target, market)
                entry = result["plugins"][0]
                self.assertEqual(entry["source"]["path"], "./" + relative)
                self.assertEqual((market_root / entry["source"]["path"]).resolve(), target.resolve())
                if kind != "plan":
                    self.assertTrue((target / ".codex-plugin/plugin.json").is_file())
                else:
                    self.assertFalse(market.exists())
                    self.assertFalse(target.exists())

    def check_outside(self, kind):
        market_root = self.root / "market"
        market = market_root / ".agents/plugins/marketplace.json"
        # A sibling sharing the root prefix is still outside the marketplace.
        target = self.root / "market-other/plugins/codex_oracle"
        with self.assertRaisesRegex(ValueError, "inside the marketplace root"):
            self.invoke(kind, target, market)
        self.assertFalse(market.exists())
        self.assertFalse(target.parent.exists())
        self.assertFalse(market_root.exists())

    def test_python_paths(self):
        self.check_paths("python")

    def test_python_rejects_outside_before_writing(self):
        self.check_outside("python")

    @unittest.skipUnless(os.name == "nt", "Windows PowerShell installer")
    def test_windows_paths(self):
        self.check_paths("powershell")

    @unittest.skipUnless(os.name == "nt", "Windows PowerShell installer")
    def test_windows_rejects_outside_before_writing(self):
        self.check_outside("powershell")

    @unittest.skipUnless(os.name == "nt", "Windows PowerShell planner")
    def test_plan_paths(self):
        self.check_paths("plan")

    @unittest.skipUnless(os.name == "nt", "Windows PowerShell planner")
    def test_plan_rejects_outside_before_writing(self):
        self.check_outside("plan")


if __name__ == "__main__":
    unittest.main()
