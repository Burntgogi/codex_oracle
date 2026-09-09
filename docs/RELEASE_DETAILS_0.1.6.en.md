# 0.1.6 details and validation

[Release notes](../RELEASE_NOTES.en.md) · [한국어](RELEASE_DETAILS_0.1.6.md)

## Changes

GPT-6 Astra + Pro is the default web target. Saved conversation URLs allow result recovery without resending the request.

The installer copies files into a staging folder and checks their hashes before replacement. Handled replacement or settings-write errors restore the previous installation. Successful updates retain installation and settings backups while preserving existing settings and plugin order.

On Windows, `scripts/diagnose-install.ps1` compares registration, installed files and running Oracle processes. A mismatch prompts a restart; unavailable process inspection is reported as unverified.

Machine-specific paths were removed, and the banner and installation guide were updated. Codex registration checks now parse JSON instead of display text.

## Validation and limits

- The installed 0.1.6 runtime completed a real `6 Pro` request, answer capture, URL checkpoint and session finalization.
- Windows installation, rollback, settings and order-preservation checks passed.
- Forced interruption during generation was tested on 0.1.5 with the same web runtime code.
- The macOS installer logic passed host tests only. Native macOS execution, network-disconnection recovery and power-loss recovery remain unverified.
- Abrupt process termination or power loss does not guarantee automatic rollback. Retained backups support manual recovery.

## Updating

Verify the ZIP checksum and follow the [installation guide](AGENT_INSTALL.md). Keep backups until verification is complete. If an older MCP process remains active, reload Codex connections or restart the app.

[Technical verification record](VERIFICATION.md)
