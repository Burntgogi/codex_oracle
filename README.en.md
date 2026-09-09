![codex_oracle pixel-art banner](assets/codex_oracle-title-astra.png)

<h1 align="center">codex_oracle</h1>

<p align="center">
  A local Codex plugin that sends focused work context to <strong>GPT-6 Astra + Pro</strong><br>
  on ChatGPT Web and brings planning, design, and code-review feedback back into Codex.
</p>

<p align="center">
  <a href="RELEASE_NOTES.en.md"><img src="https://img.shields.io/badge/status-developer_preview-f3a6ca" alt="Developer Preview"></a>
  <a href="RELEASE_NOTES.en.md"><img src="https://img.shields.io/badge/release-v0.1.6-315BFF" alt="Release v0.1.6"></a>
  <a href="docs/VERIFICATION.md"><img src="https://img.shields.io/badge/Windows-live_verified-2f80ed" alt="Windows verified"></a>
  <a href="docs/VERIFICATION.md"><img src="https://img.shields.io/badge/macOS-dry--run_only-8a78d6" alt="macOS dry-run"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-3aa675" alt="MIT License"></a>
</p>

<p align="center">
  <a href="README.md">한국어</a> · <strong>English</strong> · <a href="#quick-install">Quick install</a> ·
  <a href="#how-it-works">How it works</a> · <a href="#support">Support</a> · <a href="#security">Security</a>
</p>

> **Developer Preview:** Intended for developers and early users who are comfortable installing and
> validating a GitHub-hosted plugin in the Codex app.

## What does it do?

- Turns the current Codex task and selected files into a focused review prompt.
- Lets the external `@Chrome` plugin open a fresh ChatGPT conversation in the user's signed-in Chrome.
- Returns the GPT-6 Astra `Pro` answer to Codex for planning, design, and review decisions.

## How it works

```text
Codex task + selected files
          │
          ▼
  codex_oracle prepares context
          │  bundle remains inside workspaceRoot
          ▼
 @Chrome → ChatGPT Web → GPT-6 Astra + Pro
          │
          ▼
  consult_finalize → Codex feedback
```

The default live path is `consult_prepare` → `@Chrome` → `consult_checkpoint` → `consult_finalize`.
Checkpoint the durable conversation URL to recover the same conversation with `reattach` after interruption.
Wait in the same tab while a temporary `WEB:` URL becomes durable; never resubmit the prompt.
`codex_oracle` does not embed Chrome or read browser credentials.

## Requirements

- Codex app
- `chrome@openai-bundled` plugin
- The user's Chrome session signed into ChatGPT
- Account access to GPT-6 Astra with `Pro`
- Windows x64, macOS Apple Silicon, or macOS Intel

## Quick install

The recommended route is to ask a Codex agent:

> Install the `v0.1.6` developer preview from `Burntgogi/codex_oracle`. Verify checksums and target
> paths first, show me the dry-run, and wait for my approval before installing it into my personal
> marketplace. Confirm that `codex_oracle@personal` and `chrome@openai-bundled` are enabled, then
> validate the first call with a small dry-run in a new task.

For a manual install, pin the release tag instead of following moving `main`:

```powershell
git clone --branch v0.1.6 --depth 1 https://github.com/Burntgogi/codex_oracle.git
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
Use codex_oracle to send this implementation plan to GPT-6 Astra + Pro and review its critical
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

`v0.1.6` passed Go tests, static checks, Windows execution, macOS cross-compilation, checksums and
recovery smoke tests. After installation and a Codex restart, 0.1.6 completed a real `6 Pro` request,
answer capture and session finalization. Forced interruption during generation was tested on 0.1.5
with the same web implementation. Native macOS execution remains unverified.
See the [verification record](docs/VERIFICATION.md) for evidence and limits.

## Security

Sessions store metadata by default, without prompt or answer bodies. Set
`ORACLE_SESSION_CONTENT_MODE=full` only when local content retention is needed.
Use `session_delete` to remove a local session; ChatGPT web history is separate.

- It does not request or persist `OPENAI_API_KEY`, Chrome cookies, or login credentials.
- Selected files are resolved only inside the explicit `workspaceRoot`; path or link drift fails closed.
- Stop for login, CAPTCHA, human verification, wrong-domain state, or unexpected sensitive files.
- Never attach tokens, private prompts, private repository content, or user files to public issues.

See [SECURITY.md](SECURITY.md) for vulnerability reporting and the exact trust boundaries.

## Previous 0.1 releases

`0.1.0-preview` and `0.1.1-preview` remain as previous developer-preview history. New installs
should pin the currently verified `v0.1.6` tag.

## Acknowledgements

This project was inspired by [steipete/oracle](https://github.com/steipete/oracle) and its practical
workflow of selecting useful work context, asking another model for a review, and bringing the result
back into the original task. Thanks to its maintainers and contributors for making that work public.

## License

[MIT License](LICENSE)
