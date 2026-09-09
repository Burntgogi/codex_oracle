# codex_oracle Workflow

The preferred live workflow is Chrome-assisted. `codex_oracle` prepares and records the consult;
Codex uses the external `@Chrome` plugin to operate the user's ordinary signed-in ChatGPT session.

## Agent Workflow

1. Decide whether a second opinion is useful.
2. Choose the smallest file set that contains the relevant truth.
3. Call `consult` with `dryRun:true`.
4. Inspect resolved engine/model, ignored files, bundle preview, and attachment plan.
5. Remove secrets or oversized context before any live run.
6. Call `consult_prepare`.
7. Use `@Chrome` (`chrome@openai-bundled`) to open or claim `https://chatgpt.com/`.
8. Create a fresh ChatGPT conversation for this consult.
9. Verify GPT-6 Astra model selection and `Pro` mode/thinking evidence when requested.
10. Submit according to `handoff.submission.mode`: send `handoff.submission.promptText` directly
    for `inline`, or paste `handoff.prompt` as a pasted-text attachment and then send
    `handoff.submission.promptText` for `pasted-text-attachment`.
11. Immediately save the new conversation URL with `consult_checkpoint` and the prepared identity,
    then wait for and capture the final answer and bounded model evidence.
12. Call `consult_finalize` with the matching `handoffDigest` and `handoffNonce`.
13. Use `session_delete` if the local session should be removed after the result is returned to
    Codex.

## Chrome-Assisted Contract

Live mode is allowed only when:

- the active tab is on `https://chatgpt.com/`
- the user is signed in or completes login directly in Chrome
- the workflow creates a fresh conversation
- the requested model/mode controls are visible or the workflow fails closed
- CAPTCHA and human-verification loops are not bypassed
- finalization includes the prepared handoff digest and nonce

The Go MCP server does not call the Chrome plugin directly. Codex orchestrates both plugin
surfaces.

## Dry-Run Contract

Dry-run must not launch Chrome, create a ChatGPT conversation, upload files, or create a local
session. Dry-run must validate inputs, expand presets, resolve files, and render the mandatory
untrusted-context preamble before file contents.

## Storage Contract

Default mode keeps local session content minimal. Prompt text, bundle text, handoff JSON, answer
output, and transcripts are omitted unless `ORACLE_SESSION_CONTENT_MODE=full` is set.

Use `sessions` for inspection and `session_delete` for cleanup.

## Legacy Diagnostics

`reattach` returns a Chrome recovery plan for Chrome-assisted sessions. `login_setup`, `smoke_check`,
and the files under `legacy\scripts` are retained for
private-profile direct-CDP diagnostics only. They are not part of the preferred production path or
the default release gate.
