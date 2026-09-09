# codex_oracle Install

`codex_oracle` is designed for local plugin packaging first. It does not require Node and does not
require an OpenAI API key for browser consults.

## Build

```powershell
.\scripts\build.ps1
```

The script runs Go tests, builds `bin\codex_oracle.exe`, and validates the plugin manifest when
the local Codex plugin validator is available.
With `-AllPlatforms` it also cross-compiles macOS Intel and Apple Silicon binaries from the same source.

## Local MCP Shape

`.mcp.json` defines:

```json
{
  "mcpServers": {
    "codex_oracle": {
      "command": "./bin/codex_oracle.exe",
      "cwd": "."
    }
  }
}
```

The `.mcp.json` environment allowlist intentionally excludes `OPENAI_API_KEY`.

## External Dependency

Chrome-assisted live use requires `@Chrome` (`chrome@openai-bundled`) as a companion plugin. The Go
plugin prepares and finalizes sessions; `@Chrome` operates the user's ordinary signed-in Chrome
session.

## Optional Environment

- `ORACLE_HOME_DIR`: local session home
- `ORACLE_CHATGPT_URL`: alternate ChatGPT URL
- `ORACLE_ENGINE`: default engine override
- `ORACLE_SESSION_CONTENT_MODE`: set to `full` only when local prompt/output/transcript artifacts
  should be persisted
- `ORACLE_BROWSER_PROFILE_DIR`: legacy direct-CDP diagnostic profile path

Default session home is `~\.codex_oracle`.

## Smoke Verification

```powershell
.\scripts\smoke-mcp.ps1
.\scripts\verify-automatic.ps1 -SkipInstalledPlugin
```

The smoke test uses temporary state and a fake Chrome path. It does not open a browser, submit a
prompt to ChatGPT, or write user-level Codex config.

## Release Verification

```powershell
.\scripts\build-dist.ps1
.\scripts\verify-installed.ps1 -PluginRoot .\dist\codex_oracle-0.1.6 -SkipCodexList
.\scripts\verify-release.ps1 -SkipManualChromeEvidence
```

`verify-installed.ps1` checks the packaged `.mcp.json`, binary, tool list, dry-run consult,
Chrome-assisted handoff fields, and `chrome@openai-bundled` dependency declaration.

The candidate includes a captured `source/` snapshot, `provenance.json`, and `SHA256SUMS.txt`.
Existing output directories are refused, not overwritten. For macOS, use the matching plugin root
at `platforms/darwin-arm64` or `platforms/darwin-amd64`; its `.mcp.json` launches `./bin/codex_oracle`.
After copying from Windows, run `chmod +x bin/codex_oracle` inside that macOS plugin root if needed.
macOS builds are cross-compiled; native execution and signing are not verified by Windows tests.

## Optional Marketplace Plan

Do not write `~\.agents\plugins\marketplace.json`, copy into `~\plugins`, or install into the
Codex plugin cache without explicit approval.

Dry-run only:

```powershell
.\scripts\marketplace-plan.ps1
.\scripts\marketplace-plan.ps1 -AsJson
.\scripts\smoke-marketplace-plan.ps1
```

The planner proposes a local marketplace entry for `codex_oracle` at `.\plugins\codex_oracle` and
reports the exact target paths without writing files.

## Legacy Diagnostics

Private-profile direct-CDP scripts are under `legacy\scripts`. Use them only when explicitly
investigating the old direct-CDP runner. They are excluded from the release package and default
verification gate.
