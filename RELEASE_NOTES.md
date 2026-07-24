# codex_oracle Release Notes

## 0.1.1-preview

This developer preview targets an Oracle-style second-opinion workflow using ChatGPT Web,
GPT-5.6 Sol with `Pro`, through the external `@Chrome` companion plugin.

## What Is Included

- Codex plugin manifest and skill.
- Go MCP server binary for Windows amd64.
- Cross-compiled Go MCP server binaries for macOS arm64 and macOS amd64.
- Dry-run-first install scripts.
- SHA-256 checksums.
- Live E2E checklist and Windows live E2E evidence.

## Requirements

- Codex app.
- `chrome@openai-bundled` installed and enabled.
- A Chrome session signed into ChatGPT.
- Account access and rollout availability for GPT-5.6 Sol with `Pro`.

Compatibility: callers may still provide the input-only `gpt-5.5-pro` alias, but prepared handoffs
always target `gpt-5.6-sol-pro`.

## Preview Limitations

- Windows binary is unsigned.
- macOS binaries are not signed or notarized.
- macOS support is implemented and dry-run verified, but physical macOS execution has not been
  verified in this development environment.
- The plugin does not embed Chrome. Browser work is intentionally delegated to `@Chrome`.
- CAPTCHA, Cloudflare, and human-verification checks are not bypassed.

## Verified In This Preview

- `codex_oracle@personal` installed and enabled on Windows.
- MCP smoke verification from the installed package.
- `@Chrome` live handoff to ChatGPT and `consult_finalize` completion.

## 0.1.0-preview

The prior developer-preview package is superseded by the current GPT-5.6 Sol with `Pro` target.
