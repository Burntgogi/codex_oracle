# Agent Install Guide

This guide is written for a Codex app agent installing `codex_oracle` from this release repository.

This is a developer preview. The Windows binary is unsigned, macOS binaries are not notarized, and
macOS execution has not been verified on physical macOS hardware in this development environment.

## Install Model

Use a dry-run-first install flow. Installing a Codex plugin can touch user-level plugin folders and
marketplace files, so the agent must identify the exact target paths and get explicit user approval
before writing.

Default personal-install targets:

- Windows plugin copy: `%USERPROFILE%\.codex\local-marketplaces\personal\plugins\codex_oracle`
- macOS plugin copy: `$HOME/.codex/local-marketplaces/personal/plugins/codex_oracle`
- Windows marketplace: `%USERPROFILE%\.codex\local-marketplaces\personal\.agents\plugins\marketplace.json`
- macOS marketplace: `$HOME/.codex/local-marketplaces/personal/.agents/plugins/marketplace.json`

The scripts below do not write unless `-Apply` or `--apply` is provided.

## Required Companion Plugin

`codex_oracle` does not embed Chrome. Live GPT 5.5 + `Pro 확장` use requires:

- Codex app
- `@Chrome` plugin: `chrome@openai-bundled`
- the user's ordinary Chrome session signed into ChatGPT

The MCP handoff declares this dependency as:

```json
{
  "orchestration": {
    "mode": "external-chrome-plugin",
    "requiredPlugin": "chrome@openai-bundled",
    "requiresSignedInUserSession": true,
    "ownsBrowserAutomation": false,
    "allowAutomaticSubmission": true,
    "requiresSeparateSendConfirmation": false
  },
  "submission": {
    "mode": "inline",
    "promptText": "...text to place in the composer...",
    "externalSubmissionAuthorizedByInvocation": true,
    "requiresSeparateSendConfirmation": false
  }
}
```

## Legacy Plugin Policy

Earlier development builds used `codex-oracle-go@oracle-local`. That plugin id is not the preview
release target. If `codex plugin list` shows both names, validate and invoke only
`codex_oracle@personal`.

For a clean preview validation run, remove the legacy installed entry before live E2E:

```powershell
codex plugin remove codex-oracle-go@oracle-local --json
```

Do not delete the old marketplace or source tree unless the user explicitly asks for repository
cleanup. Removing the installed entry is enough to prevent ordinary Codex invocation confusion.

## Preflight

Run from the release repository root on Windows:

```powershell
.\scripts\smoke-mcp.ps1
.\scripts\smoke-marketplace-plan.ps1
.\scripts\verify-installed.ps1 -PluginRoot . -SkipCodexList
```

Expected:

- `MCP smoke passed`
- `Marketplace plan smoke passed`
- `Installed plugin verification passed: <release repo>`

## Windows Personal Install

Dry-run:

```powershell
.\scripts\install-personal.ps1
```

Show the user the `pluginCopyTarget` and `marketplacePath` fields. Ask for explicit approval:

```text
Approve copying this plugin to <pluginCopyTarget> and updating <marketplacePath>?
```

After approval:

```powershell
.\scripts\install-personal.ps1 -Apply
codex plugin add codex_oracle@personal
.\scripts\verify-installed.ps1
```

If the current Codex thread still exposes a stale tool namespace or an old plugin id after
install/remove operations, start a fresh Codex thread or reload the app plugin context before live
validation. `codex plugin list` is the source of truth for installed/enabled state.

If the target already exists, stop and ask before overwriting. Only after approval:

```powershell
.\scripts\install-personal.ps1 -Apply -Force
```

## macOS Personal Install

The macOS scripts require `python3` for JSON-safe marketplace updates and verification. If it is
not available, stop and use a managed marketplace or an app-native installer instead of hand-editing
JSON with shell text tools.

Dry-run:

```sh
sh scripts/install-personal.sh
```

Show the user the `pluginCopyTarget`, `marketplacePath`, and `mcpTemplate` fields. Ask for explicit
approval before writing.

After approval:

```sh
sh scripts/install-personal.sh --apply
codex plugin add codex_oracle@personal
sh scripts/verify-installed.sh "$HOME/.codex/local-marketplaces/personal/plugins/codex_oracle"
```

The macOS installer chooses the MCP template by architecture:

- `arm64` or `aarch64`: `.mcp.macos-arm64.json`
- `x86_64` or `amd64`: `.mcp.macos-amd64.json`

When macOS hardware is unavailable, a Windows agent can still check the expected dry-run shape:

```powershell
.\scripts\simulate-macos-dry-run.ps1 -Arch arm64
.\scripts\simulate-macos-dry-run.ps1 -Arch x86_64
```

This does not execute the macOS binary. It verifies the manifest, macOS MCP templates, Mach-O
binary magic, and the dry-run target paths that the macOS installer should report.

If the target already exists, stop and ask before overwriting. Only after approval:

```sh
sh scripts/install-personal.sh --apply --force
```

## Scale-Friendly Install Plan

The current release is ready for agent-assisted personal installs. For broad public or organization
distribution, move from ad hoc local-copy install to a managed release channel:

- Publish versioned release artifacts for Windows amd64, macOS arm64, and macOS amd64.
- Publish `SHA256SUMS.txt` and verify checksums before install.
- Sign the Windows binary and sign/notarize macOS binaries.
- Use a managed marketplace entry or organization-controlled plugin source instead of per-user
  local marketplace edits.
- Keep `@Chrome` as an explicit companion dependency, not an embedded browser controller.
- Keep legacy direct-CDP live `consult` disabled by default; it is diagnostic-only and not the
  preview release path.
- Make the installer idempotent: detect existing version, report drift, and update only after
  user or organization policy approval.
- Keep release validation evidence outside the public repository unless it becomes user-facing
  installation guidance.

## First Live Use

1. Confirm `codex_oracle@personal` is installed and enabled through `codex plugin list`.
2. Confirm `@Chrome` is installed and enabled.
3. Confirm `codex-oracle-go@oracle-local` is not installed/enabled for this validation run.
4. Confirm the user is signed into ChatGPT in their ordinary Chrome profile.
5. Run `consult` with `dryRun:true`.
6. Run `consult_prepare`.
7. Use `@Chrome` to create a fresh ChatGPT conversation.
8. Select GPT 5.5 and confirm `Pro 확장`.
9. Submit according to `handoff.submission.mode`. For `inline`, send
   `handoff.submission.promptText`. For `pasted-text-attachment`, paste/attach `handoff.prompt`,
   save or close the pasted-text editor modal, then send `handoff.submission.promptText`. A user
   `codex_oracle` invocation with prompt/files authorizes this matching ChatGPT submission, so do
   not ask for a second send confirmation. ZIP/file upload is legacy direct-CDP diagnostic material
   and is not the preferred Chrome-assisted preview path.
10. Capture the answer and call `consult_finalize`.
11. Use `session_delete` if local session metadata should be removed after the answer is returned
    to Codex.

Keep the first live use intentionally small. A dry-run can prove that many files resolve, but
ChatGPT Web may reject or stall on large inline prompts. If the bundle is large, reduce the file set
to a structure brief plus the few files needed for the question, then run `consult_prepare` again.

Stop for login, CAPTCHA, repeated human-verification loops, wrong-domain state, or unexpected
sensitive files. Do not bypass them.
