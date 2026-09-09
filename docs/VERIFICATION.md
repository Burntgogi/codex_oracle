# Verification and privacy scope

The 0.1.5 Windows runtime was tested through a signed-in Chrome session whose visible model label
was `6 Pro`. One short response and a 26,040-character pasted context were submitted in separate tests.
The longer request returned 120 JSON rows, all verified against the input values and their squares.
The MCP process was forcibly terminated while the web UI was still thinking. A new process recovered
the saved conversation URL; a new browser tab captured the same answer without another submission.
Finalize and metadata-only storage checks passed. The UI label does not prove a backend model ID.

0.1.6 was installed from the release ZIP into Codex. After restarting Codex, all observed Oracle
processes ran from the 0.1.6 cache. A real request through the visible `6 Pro` UI returned five
numbers' count, sum and sorted unique values correctly. URL checkpointing, the reattach plan and
finalization completed. The web UI reported 13 seconds. This is UI evidence, not backend model
identity verification. The later installer and documentation changes do not alter the web runtime.

Windows installer tests cover retained backups, plugin order and settings preservation, invalid
source rejection, and rollback when marketplace replacement fails after the directory swap.
The macOS Python installer transaction passes host tests with an injected replacement failure;
this does not establish native macOS support. Caught installation failures roll back. Abrupt power
loss or process termination can leave a sibling backup requiring manual recovery.

No real conversation URLs, user profiles, session transcripts, configuration backups, or login data
are included. Test fixtures contain intentionally fake cookies, example emails, and synthetic URLs.
Builds use -trimpath and -buildvcs=false. Run `python scripts/check-privacy.py` before publishing.
This pattern check is a guard, not a guarantee of absence of secrets.

Remaining limits: native macOS execution and network-disconnection recovery are unverified.
Historical Git binaries may contain older build paths; this release does not rewrite Git history.
The current banner uses the Astra label; the original artwork is preserved as a separate asset.
