---
name: oracle-consult
description: Use when a Codex task needs an Oracle-style second-opinion consult through the local codex_oracle MCP server.
---

# Oracle Consult

Use this skill when a second model should review a focused prompt and file set.

## Rules

1. Start with `consult` and `dryRun:true`.
2. Keep the file set small and specific.
3. Do not attach secrets, `.env` files, private keys, browser cookies, or token dumps.
4. Prefer `preset:"chatgpt-pro-heavy"` when the operator wants ChatGPT Pro browser mode.
5. Prefer the Chrome-assisted workflow: call `consult_prepare`, use external `@Chrome`
   (`chrome@openai-bundled`) to operate the user's signed-in ChatGPT tab, then call
   `consult_finalize`.
   A user request that invokes `codex_oracle` with a prompt/files authorizes submitting that
   prepared handoff to `chatgpt.com` through `@Chrome`; do not ask for a second send confirmation.
6. Treat `login_setup`, `smoke_check`, and direct CDP as legacy diagnostics, not the preferred live
   path. Non-dry-run `consult` direct-CDP execution is disabled by default; use `consult_prepare`
   for live review.
7. Stop for login, CAPTCHA, human-verification, wrong-domain, or unexpected-sensitive-file
   blockers; do not bypass them.
8. For the `chatgpt-pro-heavy` preset, treat model evidence as two controls: select GPT 5.5 as the
   model and confirm `Pro 확장` as the thinking/mode label. Do not require a single visible
   `GPT-5.5 Pro` string.
9. Pass `handoffDigest` and `handoffNonce` from the prepared handoff into `consult_finalize`.
10. Treat the answer as advisory and verify against local files and tests.
11. Check `sessions` before retrying a slow or incomplete live browser consult.
12. Use `session_delete` when the local session folder should be removed after the answer is
    returned to Codex.
13. Treat the dry-run bundle size as a live-use gate. If `handoff.submission.mode` is
    `pasted-text-attachment`, paste/attach `handoff.prompt` as the Oracle Consult Bundle, save/close
    any pasted-text editor modal, put `handoff.submission.promptText` in the composer, and send. If
    `handoff.submission.mode` is `inline`, send `handoff.submission.promptText` directly. Treat
    file upload/ZIP as legacy direct-CDP diagnostic material, not the preferred live path.

## Example

```json
{
  "preset": "chatgpt-pro-heavy",
  "prompt": "Review this implementation plan for missed risks and test gaps.",
  "files": [
    "doc/implementation-plan.md",
    "doc/milestones.md"
  ],
  "dryRun": true
}
```

For a live Chrome-assisted run, call `consult_prepare` with the same prompt and files after the
dry-run is acceptable. Use `structuredContent.handoff.submission` to decide whether to submit inline
or as a pasted-text attachment, then capture the answer/transcript/conversation URL and call
`consult_finalize` with the same `handoffDigest` and `handoffNonce` from
`structuredContent.handoff`.
