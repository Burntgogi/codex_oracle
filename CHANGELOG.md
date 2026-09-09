# Changelog

## 0.1.6

- Limit personal-install copying to shipped roots, excluding local backups and unknown private directories.
- Refresh the banner to Astra and record the installed 0.1.6 web round-trip verification.
- Stage and hash-check installation files; retain backups and roll back handled replacement failures.
- Preserve marketplace settings and plugin order.
- Add Windows installed/running version diagnostics and parse Codex registration as JSON.

- Target GPT-6 Astra with Pro for new Chrome handoffs; preserve legacy session identity.
- Save durable conversation URLs with consult_checkpoint and recover without resubmission.
- Preserve retryability when session result writes fail; keep content private in metadata mode.
- Explain temporary WEB: URL retries and fix named argument forwarding in verification scripts.
- Include buildable Go sources, remove machine-specific paths, and add a public-file privacy check.
- Preserve existing branding and installation scripts. macOS remains cross-compiled, not natively verified.

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
