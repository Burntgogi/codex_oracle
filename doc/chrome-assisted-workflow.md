# Chrome-Assisted codex_oracle Workflow

This is the preferred live browser path. It replaces private-profile direct-CDP automation for
normal use.

## Target Shape

```text
Codex user request
  -> oracle-consult skill
  -> codex_oracle consult_prepare
  -> external @Chrome plugin operates the user's signed-in ChatGPT tab
  -> ChatGPT creates a fresh conversation
  -> @Chrome captures the answer
  -> codex_oracle consult_finalize records metadata/result
```

## Ownership

`codex_oracle` owns:

- prompt and file bundling
- secret/path/file guardrails
- preset expansion
- handoff digest and nonce
- privacy manifest
- local session metadata
- finalization validation

`@Chrome` owns:

- access to the user's ordinary Chrome profile
- ChatGPT tab creation/claiming
- visible login/CAPTCHA/model-selection interaction
- answer capture from the web page

Codex owns orchestration between the two. A user request that invokes `codex_oracle` with a
prompt/files authorizes Codex to submit that prepared handoff to `chatgpt.com` through `@Chrome`
without a second send-confirmation prompt. The Go binary does not import, spawn, or call the Chrome
plugin directly.

## Handoff Contract

`consult_prepare` returns `structuredContent.handoff` with:

```json
{
  "chatgptUrl": "https://chatgpt.com/",
  "model": "gpt-6-astra-pro",
  "modelLabel": "GPT-6 Astra",
  "thinking": "extended",
  "thinkingLabel": "Pro",
  "handoffDigest": "...",
  "handoffNonce": "...",
  "prompt": "...rendered prompt bundle...",
  "privacy": {
    "targetHost": "chatgpt.com",
    "allowBrowserHistory": false,
    "allowUnrelatedTabs": false,
    "requiresUserApprovalForExternalSubmission": false,
    "externalSubmissionAuthorizedByInvocation": true
  },
  "submission": {
    "mode": "inline",
    "promptText": "...text to place in the composer...",
    "contextSource": "handoff.prompt",
    "externalSubmissionAuthorizedByInvocation": true,
    "requiresSeparateSendConfirmation": false,
    "stopOnHumanVerification": true
  },
  "orchestration": {
    "mode": "external-chrome-plugin",
    "requiredPlugin": "chrome@openai-bundled",
    "requiresSignedInUserSession": true,
    "ownsBrowserAutomation": false,
    "automationOwner": "codex-agent-via-chrome-plugin",
    "allowAutomaticSubmission": true,
    "requiresSeparateSendConfirmation": false,
    "stopConditions": [
      "login-required",
      "captcha-or-human-verification",
      "target-host-not-chatgpt.com",
      "unexpected-sensitive-file"
    ]
  }
}
```

By default this handoff is returned to Codex but not written to a local artifact file. Set
`ORACLE_SESSION_CONTENT_MODE=full` only when local handoff/transcript artifacts are intentionally
needed.

## Live Steps

1. Run `consult` with `dryRun:true` when the file set is uncertain.
2. Run `consult_prepare`.
3. Use `@Chrome` to open or claim `https://chatgpt.com/`.
4. Create a new conversation.
5. Select GPT-6 Astra and confirm `Pro` when the preset asks for heavy Pro review.
6. Submit according to `handoff.submission.mode`:
   - `inline`: place `handoff.submission.promptText` in the composer and send.
   - `pasted-text-attachment`: paste/attach `handoff.prompt` as the Oracle Consult Bundle, save or
     close any pasted-text editor modal, place `handoff.submission.promptText` in the composer, and
     send.
   - `file-upload`: not used by the preferred Chrome-assisted release path. ZIP/file upload remains
     legacy direct-CDP diagnostic material until a visible upload artifact contract is implemented.
7. Wait for a durable `/c/<id>` URL; a `/c/WEB:...` address immediately after sending is temporary.
   Stay in the same tab without resubmitting. Call `consult_checkpoint` with `checkpoint.arguments` from prepare plus the durable conversation URL,
   then wait until the answer is complete. Checkpoint failure must be surfaced and retried; do not
   submit the prompt again. This stores the URL only, not a prompt or partial answer.
8. Capture the answer, conversation URL, and bounded evidence.
9. Call `consult_finalize` with `sessionId`, `handoffDigest`, `handoffNonce`, `status`, `answer`,
   and `runtime.conversationUrl`.
10. Verify the advice locally before applying it.

## Failure Handling

- Login: the user completes sign-in directly in Chrome; credentials are not pasted into Codex chat.
- CAPTCHA/human verification: do not bypass; wait for the user or finalize as failed.
- Model ambiguity: ask the user to select GPT-6 Astra and `Pro`, or fail closed. During UI rollout,
  `Pro 확장` or `프로 확장` may be accepted as equivalent visible mode evidence.
- Pasted-text attachment modal: save/close it before pressing send; otherwise ChatGPT may keep the
  composer in edit state.
- Long responses: keep the prepared session and finalize once the visible answer is captured.

## Recover an Interrupted Review

1. Find the original session using `sessions`, then call `reattach` with `{"id":"<session-id>"}`.
2. For `executionMode:"chrome-assisted"`, follow `plan.steps` using @Chrome. This is read-only
   even with `dryRun:false`; the server does not launch Chrome or change the session.
3. Use `runtime.conversationUrl` if present. If it was never saved (common before the first
   checkpoint), recover this task's previously claimed tab or ask the user for the conversation URL.
   Do not inspect unrelated tabs/history or resubmit the prompt. Metadata-only storage cannot
   reconstruct a lost prompt or locate a lost browser conversation by itself.
4. Verify the original conversation and capture its completed answer. Pass the returned `finalize`
   identity fields to `consult_finalize`, adding `status:"completed"`, `answer`, and
   `runtime.conversationUrl`. It must match the checkpoint unless an intentional replacement uses
   `force:true`. URLs must identify `/c/<id>` (including `/g/<project>/c/<id>`) on HTTPS chatgpt.com,
   without credentials, port, query, or fragment; remove URL tracking parameters before saving.
5. If saving fails, fix the reported filesystem error and retry the same finalize request.
   Output/transcript writes finish before completion metadata is committed. A session that was
   running remains running after these failures, so no `force:true` is needed.
6. `requiresForce:true` means the session was already completed or failed. Read its answer in
   Chrome without refinalizing; use `force:true` only for an intentional replacement.

Default metadata-only mode still stores no prompt, answer, or transcript body. Full mode writes
files through sibling temporary files before replacement. These are not multi-file transactions:
a failure can update some content files while leaving the previous status. The Store lock protects
heartbeat/result updates in one server instance, not concurrent processes sharing the same session.

Run `scripts/smoke-recovery.ps1` to exercise prepare → recovery plan → finalize → inspect → delete
through the built executable with synthetic content and an isolated temporary session home. This
checks the local MCP contract; it does not submit to ChatGPT or test its live UI.

## Legacy Direct-CDP Diagnostics

The direct-CDP runner and scripts under `legacy\scripts` are diagnostic material. They are not the
release path for 10,000-user distribution because a separate private Chrome profile can trigger
login and Cloudflare verification loops. Non-dry-run `consult` direct-CDP execution is disabled by
default in the preview release and requires `ORACLE_ENABLE_LEGACY_CDP=1` for local diagnostics.
