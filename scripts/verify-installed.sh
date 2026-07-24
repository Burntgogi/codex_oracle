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
WORKSPACE_DIR="${TMPDIR:-/tmp}/codex_oracle-installed-workspace-$$"
WORKSPACE_FILE="workspace-fixture.md"
OUTSIDE_DIR="${TMPDIR:-/tmp}/codex_oracle-installed-outside-$$"
OUTSIDE_DESCENDANT="outside-descendant.md"
OUTSIDE_GLOB="$OUTSIDE_DIR/**/*.md"
mkdir -p "$HOME_DIR" "$WORKSPACE_DIR" "$OUTSIDE_DIR/nested"
WORKSPACE_DIR=$(CDPATH= cd -- "$WORKSPACE_DIR" && pwd)
printf '%s\n' 'workspace fixture' > "$WORKSPACE_DIR/$WORKSPACE_FILE"
printf '%s\n' 'outside' > "$OUTSIDE_DIR/nested/$OUTSIDE_DESCENDANT"
trap 'rm -rf "$HOME_DIR" "$WORKSPACE_DIR" "$OUTSIDE_DIR"' EXIT

WORKSPACE_ROOT_JSON=$(python3 - "$WORKSPACE_DIR" <<'PY'
import json, sys
print(json.dumps(sys.argv[1]))
PY
)
FILES_JSON=$(python3 - "$WORKSPACE_FILE" "$OUTSIDE_DIR" "$OUTSIDE_GLOB" <<'PY'
import json, sys
print(json.dumps(sys.argv[1:]))
PY
)
REQUESTS=$(cat <<EOF
{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"consult","arguments":{"preset":"chatgpt-pro-heavy","prompt":"installed plugin dry-run check","files":$FILES_JSON,"workspaceRoot":$WORKSPACE_ROOT_JSON,"dryRun":true}}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"consult_prepare","arguments":{"preset":"chatgpt-pro-heavy","prompt":"installed prepare check","files":["$WORKSPACE_FILE"],"workspaceRoot":$WORKSPACE_ROOT_JSON,"browserThinkingTime":"extended"}}}
EOF
)

RESPONSES=$(cd "$PLUGIN_ROOT" && printf '%s\n' "$REQUESTS" | ORACLE_HOME_DIR="$HOME_DIR" "$BINARY")
RESPONSES_JSON="$RESPONSES" HOME_DIR="$HOME_DIR" WORKSPACE_DIR="$WORKSPACE_DIR" WORKSPACE_FILE="$WORKSPACE_FILE" OUTSIDE_DIR="$OUTSIDE_DIR" OUTSIDE_GLOB="$OUTSIDE_GLOB" OUTSIDE_DESCENDANT="$OUTSIDE_DESCENDANT" python3 - <<'PY'
import json, os
responses = [json.loads(line) for line in os.environ["RESPONSES_JSON"].splitlines() if line.strip()]
tools_by_name = {tool["name"]: tool for tool in responses[0]["result"]["tools"]}
tools = set(tools_by_name)
required = {"consult", "consult_prepare", "consult_finalize", "sessions", "session_delete", "login_setup", "smoke_check", "reattach"}
missing = required - tools
if missing:
    raise SystemExit(f"installed tools/list missing {sorted(missing)}")
for tool_name in ("consult", "consult_prepare"):
    workspace_root = tools_by_name[tool_name]["inputSchema"]["properties"].get("workspaceRoot", {})
    if workspace_root.get("type") != "string":
        raise SystemExit(f"installed {tool_name} input schema missing string workspaceRoot")
if responses[1]["result"]["structuredContent"]["status"] != "dry-run":
    raise SystemExit("consult dry-run did not return dry-run")
resolved = responses[1]["result"]["structuredContent"]["resolved"]
if resolved["files"] != [os.environ["WORKSPACE_FILE"]]:
    raise SystemExit("consult did not resolve fixture from workspaceRoot")
outside_patterns = {os.environ["OUTSIDE_DIR"], os.environ["OUTSIDE_GLOB"]}
ignored = responses[1]["result"]["structuredContent"].get("ignored", [])
if len(ignored) != 2:
    raise SystemExit("consult did not reject both outside directory and glob")
for item in ignored:
    if item.get("pattern") not in outside_patterns:
        raise SystemExit(f"consult returned an unexpected outside rejection pattern: {item.get('pattern')!r}")
    if item.get("reason") != "outside root":
        raise SystemExit(f"consult did not mark outside input as outside root: {item.get('reason')!r}")
    fields = [*resolved["files"], item.get("pattern", ""), item.get("path", ""), item.get("reason", "")]
    if any(os.environ["OUTSIDE_DESCENDANT"] in str(field) for field in fields):
        raise SystemExit("consult leaked an outside descendant path")
if resolved["model"] != "gpt-5.6-sol-pro":
    raise SystemExit("consult dry-run did not resolve to gpt-5.6-sol-pro")
if resolved["browser"]["desiredModel"] != "GPT-5.6 Sol":
    raise SystemExit("consult dry-run did not return GPT-5.6 Sol desired model")
if resolved["browser"]["thinkingLabel"] != "Pro":
    raise SystemExit("consult dry-run did not return Pro thinking label")
prepared = responses[2]["result"]["structuredContent"]
if prepared["resolved"]["files"] != [os.environ["WORKSPACE_FILE"]]:
    raise SystemExit("consult_prepare did not resolve fixture from workspaceRoot")
handoff = prepared["handoff"]
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
meta_path = os.path.join(os.environ["HOME_DIR"], "sessions", prepared["sessionId"], "meta.json")
with open(meta_path, encoding="utf-8") as meta_file:
    metadata = json.load(meta_file)
if metadata.get("cwd") != os.environ["WORKSPACE_DIR"]:
    raise SystemExit("consult_prepare session cwd did not match workspaceRoot")
PY

echo "Installed plugin verification passed: $PLUGIN_ROOT"
