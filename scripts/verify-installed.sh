#!/usr/bin/env sh
set -eu

OS_NAME=$(uname -s)
if [ "$OS_NAME" != "Darwin" ]; then
  echo "verify-installed.sh is for macOS. Use scripts/verify-installed.ps1 on Windows." >&2
  exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required for JSON verification." >&2
  exit 1
fi

PLUGIN_ROOT="${1:-.}"
PLUGIN_ROOT=$(CDPATH= cd -- "$PLUGIN_ROOT" && pwd)

if [ ! -f "$PLUGIN_ROOT/.mcp.json" ]; then
  echo "Installed .mcp.json not found: $PLUGIN_ROOT/.mcp.json" >&2
  exit 1
fi
if [ ! -f "$PLUGIN_ROOT/README.md" ]; then
  echo "README.md not found under plugin root: $PLUGIN_ROOT/README.md" >&2
  exit 1
fi

COMMAND=$(python3 - "$PLUGIN_ROOT/.mcp.json" <<'PY'
import json, sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
server = data.get("mcpServers", {}).get("codex_oracle")
if not server:
    raise SystemExit(".mcp.json does not define mcpServers.codex_oracle")
print(server.get("command", ""))
PY
)

case "$COMMAND" in
  ./bin/codex_oracle_darwin_arm64|./bin/codex_oracle_darwin_amd64) ;;
  *) echo ".mcp.json command is $COMMAND; expected a darwin binary" >&2; exit 1 ;;
esac

BINARY="$PLUGIN_ROOT/${COMMAND#./}"
if [ ! -x "$BINARY" ]; then
  chmod +x "$BINARY" 2>/dev/null || true
fi
if [ ! -x "$BINARY" ]; then
  echo "MCP binary is not executable: $BINARY" >&2
  exit 1
fi

HOME_DIR="${TMPDIR:-/tmp}/codex_oracle-installed-verify-$$"
mkdir -p "$HOME_DIR"
trap 'rm -rf "$HOME_DIR"' EXIT

REQUESTS='{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"consult","arguments":{"preset":"chatgpt-pro-heavy","prompt":"installed plugin dry-run check","files":["README.md"],"dryRun":true}}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"consult_prepare","arguments":{"preset":"chatgpt-pro-heavy","prompt":"installed prepare check","files":["README.md"],"browserThinkingTime":"extended"}}}'

RESPONSES=$(cd "$PLUGIN_ROOT" && printf '%s\n' "$REQUESTS" | ORACLE_HOME_DIR="$HOME_DIR" "$BINARY")
RESPONSES_JSON="$RESPONSES" python3 - <<'PY'
import json, os
responses = [json.loads(line) for line in os.environ["RESPONSES_JSON"].splitlines() if line.strip()]
tools = {tool["name"] for tool in responses[0]["result"]["tools"]}
required = {"consult", "consult_prepare", "consult_finalize", "sessions", "session_delete", "login_setup", "smoke_check", "reattach"}
missing = required - tools
if missing:
    raise SystemExit(f"installed tools/list missing {sorted(missing)}")
if responses[1]["result"]["structuredContent"]["status"] != "dry-run":
    raise SystemExit("consult dry-run did not return dry-run")
resolved = responses[1]["result"]["structuredContent"]["resolved"]
if resolved["model"] != "gpt-5.6-sol-pro":
    raise SystemExit("consult dry-run did not resolve to gpt-5.6-sol-pro")
if resolved["browser"]["desiredModel"] != "GPT-5.6 Sol":
    raise SystemExit("consult dry-run did not return GPT-5.6 Sol desired model")
if resolved["browser"]["thinkingLabel"] != "Pro":
    raise SystemExit("consult dry-run did not return Pro thinking label")
handoff = responses[2]["result"]["structuredContent"]["handoff"]
if handoff["model"] != "gpt-5.6-sol-pro":
    raise SystemExit("consult_prepare did not return gpt-5.6-sol-pro")
if handoff["modelLabel"] != "GPT-5.6 Sol":
    raise SystemExit("consult_prepare did not return GPT-5.6 Sol")
if handoff["thinkingLabel"] != "Pro":
    raise SystemExit("consult_prepare did not return Pro")
if handoff["orchestration"]["requiredPlugin"] != "chrome@openai-bundled":
    raise SystemExit("consult_prepare did not declare chrome@openai-bundled")
if handoff["orchestration"].get("allowAutomaticSubmission") is not True:
    raise SystemExit("consult_prepare did not allow automatic Chrome submission")
if handoff["orchestration"].get("requiresSeparateSendConfirmation") is not False:
    raise SystemExit("consult_prepare still requires separate send confirmation")
if handoff["privacy"].get("requiresUserApprovalForExternalSubmission") is not False:
    raise SystemExit("consult_prepare still requires separate external submission approval")
if handoff["privacy"].get("externalSubmissionAuthorizedByInvocation") is not True:
    raise SystemExit("consult_prepare did not mark invocation-based authorization")
submission = handoff.get("submission", {})
if not submission.get("mode"):
    raise SystemExit("consult_prepare did not return submission mode")
if not submission.get("promptText"):
    raise SystemExit("consult_prepare did not return submission promptText")
PY

echo "Installed plugin verification passed: $PLUGIN_ROOT"
