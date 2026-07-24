# Changelog

## 0.1.2-preview

- Added product-centered Korean and English GitHub landing pages.
- Added paired Korean and English release notes.
- Bumped the plugin and MCP runtime contract to `0.1.2`.
- Targeted GPT-5.6 Sol with Pro while retaining `gpt-5.5-pro` as an input-only compatibility alias.
- Added fail-closed checks for the selected model and Pro mode.
- Added `workspaceRoot` to `consult` and `consult_prepare`.
- Rejected outside-root directories and glob bases before traversal.
- Bound traversal and bounded file snapshots to a rooted handle and failed closed on path/link drift.
- Rebuilt Windows amd64, macOS amd64, and macOS arm64 artifacts and checksums.
- Verified the installed Windows plugin through the external `@Chrome` live path.

## Previous 0.1 previews

### 0.1.1-preview

- Updated the default ChatGPT Web target to GPT-5.6 Sol with Pro.
- Kept `gpt-5.5-pro` as an input-only compatibility alias.
- Added initial fail-closed model and Pro-mode checks.

### 0.1.0-preview

- Added the initial `codex_oracle` plugin package and Windows/macOS binaries.
- Added dry-run-first personal install scripts and OS-specific MCP templates.
- Added the `@Chrome`-assisted prepare/finalize workflow.
