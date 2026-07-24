# codex_oracle Release Notes

[한국어](RELEASE_NOTES.md) · [English](RELEASE_NOTES.en.md)

## 0.1.2-preview

`v0.1.2` is a developer preview that sends focused Codex work context through the external
`@Chrome` plugin to **GPT-5.6 Sol + Pro** on ChatGPT Web and returns the review to Codex.

### Highlights

- Reworked the GitHub landing page around the product workflow and split Korean and English READMEs.
- Added Korean and English release notes while preserving previous `0.1.x` preview history.
- Fixed the default web target at GPT-5.6 Sol with `Pro`; `gpt-5.5-pro` remains input compatibility only.
- Added explicit `workspaceRoot` boundaries to `consult` and `consult_prepare`.
- Rejects outside directories and glob bases before traversal, binds snapshots to the rooted handle,
  and fails closed when paths or links change.
- Rebuilt Windows amd64, macOS amd64, and macOS arm64 binaries with SHA-256 checksums.

### Requirements

- Codex app
- `chrome@openai-bundled`
- The user's Chrome session signed into ChatGPT
- Account access to GPT-5.6 Sol with `Pro`

### Verification results

- Go 1.26.4 tests, vet, and formatting checks
- CGO-disabled trimpath builds for Windows amd64 and Darwin amd64/arm64
- Release-package and installed-package MCP smoke checks
- `codex_oracle@personal` Windows installation and cache parity
- GPT-5.6 Sol + Pro Chrome-assisted live E2E through `consult_finalize`
- macOS arm64/x86_64 MCP templates, Mach-O format, and dry-run checks

### Known limitations

- The Darwin artifacts have not run on physical macOS hardware.
- The Windows binary is unsigned; the macOS binaries are unsigned and not notarized.
- ChatGPT Web UI, model entitlement, login state, CAPTCHA, and Cloudflare are external dependencies.
- `@Chrome` is a required companion plugin and is not embedded in `codex_oracle`.

## Previous 0.1 releases

### 0.1.1-preview

- Introduced GPT-5.6 Sol + Pro web handoffs and fail-closed model/Pro-mode checks.
- Predates the explicit `workspaceRoot` and rooted file-snapshot hardening in `v0.1.2`.

### 0.1.0-preview

- Initial developer preview with Windows/macOS Go binaries, personal-marketplace installers, and
  the `@Chrome`-assisted handoff workflow.
