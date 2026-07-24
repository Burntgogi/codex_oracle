![codex_oracle pixel-art banner](assets/codex_oracle-title.png)

# codex_oracle

A local Codex plugin that sends focused work context to **GPT-5.6 Sol + Pro** on ChatGPT Web and
brings planning, design, and code-review feedback back into Codex.

[![Developer Preview](https://img.shields.io/badge/status-developer_preview-f3a6ca)](RELEASE_NOTES.en.md)
[![Release v0.1.2](https://img.shields.io/badge/release-v0.1.2-315BFF)](RELEASE_NOTES.en.md)
[![Windows verified](https://img.shields.io/badge/Windows-live_verified-2f80ed)](RELEASE_NOTES.en.md#verification-results)
[![macOS dry-run](https://img.shields.io/badge/macOS-dry--run_only-8a78d6)](RELEASE_NOTES.en.md#known-limitations)
[![MIT License](https://img.shields.io/badge/license-MIT-3aa675)](LICENSE)

[한국어](README.md) · **English** · [Quick install](#quick-install) · [How it works](#how-it-works) ·
[Support](#support) · [Security](#security)

> **Developer Preview:** Intended for developers and early users who are comfortable installing and
> validating a GitHub-hosted plugin in the Codex app.

## What does it do?

- Turns the current Codex task and selected files into a focused review prompt.
- Lets the external `@Chrome` plugin open a fresh ChatGPT conversation in the user's signed-in Chrome.
- Returns the GPT-5.6 Sol `Pro` answer to Codex for planning, design, and review decisions.

## How it works

```text
Codex task + selected files
          │
          ▼
  codex_oracle prepares context
          │  bundle remains inside workspaceRoot
          ▼
 @Chrome → ChatGPT Web → GPT-5.6 Sol + Pro
          │
          ▼
  consult_finalize → Codex feedback
```

The default live path is `consult_prepare` → `@Chrome` → `consult_finalize`.
`codex_oracle` does not embed Chrome or read browser credentials.

## Requirements

- Codex app
- `chrome@openai-bundled` plugin
- The user's Chrome session signed into ChatGPT
- Account access to GPT-5.6 Sol with `Pro`
- Windows x64, macOS Apple Silicon, or macOS Intel

## Quick install

The recommended route is to ask a Codex agent:

> Install the `v0.1.2` developer preview from `Burntgogi/codex_oracle`. Verify checksums and target
> paths first, show me the dry-run, and wait for my approval before installing it into my personal
> marketplace. Confirm that `codex_oracle@personal` and `chrome@openai-bundled` are enabled, then
> validate the first call with a small dry-run in a new task.

For a manual install, pin the release tag instead of following moving `main`:

```powershell
git clone --branch v0.1.2 --depth 1 https://github.com/Burntgogi/codex_oracle.git
cd codex_oracle
.\scripts\install-personal.ps1
# Review the reported paths and approve, then:
.\scripts\install-personal.ps1 -Apply
codex plugin add codex_oracle@personal
.\scripts\verify-installed.ps1
```

On macOS, inspect the dry-run with `sh scripts/install-personal.sh`, then run
`sh scripts/install-personal.sh --apply` after approval. See the [agent install guide](docs/AGENT_INSTALL.md)
for the complete Windows/macOS and update procedures.

## First use

Invoke `codex_oracle` from Codex with one focused question and only the files needed for review.

```text
Use codex_oracle to send this implementation plan to GPT-5.6 Sol + Pro and review its critical
blockers and improvements. Check the file scope with a dry-run, run it in a fresh ChatGPT
conversation through @Chrome, and bring the result back into this Codex task.
```

When files are included, pass the active Codex workspace as `workspaceRoot`. A small architecture
brief and a few relevant files are more reliable than a large, unfocused bundle.

## Support

| Platform | Artifact | Validation status |
| --- | --- | --- |
| Windows x64 | `bin/codex_oracle.exe` | Install, MCP smoke, and Chrome live E2E completed |
| macOS Apple Silicon | `bin/codex_oracle_darwin_arm64` | Cross-compile, Mach-O, and dry-run completed |
| macOS Intel | `bin/codex_oracle_darwin_amd64` | Cross-compile, Mach-O, and dry-run completed |

The macOS artifacts have not run on physical macOS hardware. The Windows binary is unsigned; the
macOS binaries are unsigned and not notarized, so OS or organization trust controls may require a
direct user decision.

## Validation status

`v0.1.2` is a developer preview verified with Go tests and static checks, reproducible builds for
three targets, checksums, package smoke tests, installed Windows checks, and a Chrome-assisted
`GPT-5.6 Sol + Pro` live E2E. See the [release notes](RELEASE_NOTES.en.md) for changes and limits.

## Security

- It does not request or persist `OPENAI_API_KEY`, Chrome cookies, or login credentials.
- Selected files are resolved only inside the explicit `workspaceRoot`; path or link drift fails closed.
- Stop for login, CAPTCHA, human verification, wrong-domain state, or unexpected sensitive files.
- Never attach tokens, private prompts, private repository content, or user files to public issues.

See [SECURITY.md](SECURITY.md) for vulnerability reporting and the exact trust boundaries.

## Previous 0.1 releases

`0.1.0-preview` and `0.1.1-preview` remain as previous developer-preview history. New installs
should pin the currently verified `v0.1.2` tag.

## Acknowledgements

This project was inspired by [steipete/oracle](https://github.com/steipete/oracle) and its practical
workflow of selecting useful work context, asking another model for a review, and bringing the result
back into the original task. Thanks to its maintainers and contributors for making that work public.

## License

[MIT License](LICENSE)
