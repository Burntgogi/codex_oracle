#!/usr/bin/env sh
set -eu

APPLY=0
FORCE=0
MARKETPLACE_NAME="personal"
PLUGIN_ROOT=""
MARKETPLACE_PATH=""
PLUGIN_DESTINATION_ROOT=""

OS_NAME=$(uname -s)
if [ "$OS_NAME" != "Darwin" ]; then
  echo "install-personal.sh is for macOS. Use scripts/install-personal.ps1 on Windows." >&2
  exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required for JSON-safe marketplace updates." >&2
  exit 1
fi

while [ "$#" -gt 0 ]; do
  case "$1" in
    --apply) APPLY=1 ;;
    --force) FORCE=1 ;;
    --plugin-root) PLUGIN_ROOT="$2"; shift ;;
    --marketplace-path) MARKETPLACE_PATH="$2"; shift ;;
    --plugin-destination-root) PLUGIN_DESTINATION_ROOT="$2"; shift ;;
    --marketplace-name) MARKETPLACE_NAME="$2"; shift ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
if [ -z "$PLUGIN_ROOT" ]; then
  PLUGIN_ROOT="$REPO_ROOT"
fi
PLUGIN_ROOT=$(CDPATH= cd -- "$PLUGIN_ROOT" && pwd)

if [ -z "$MARKETPLACE_PATH" ]; then
  MARKETPLACE_PATH="$HOME/.codex/local-marketplaces/$MARKETPLACE_NAME/.agents/plugins/marketplace.json"
fi
if [ -z "$PLUGIN_DESTINATION_ROOT" ]; then
  PLUGIN_DESTINATION_ROOT="$HOME/.codex/local-marketplaces/$MARKETPLACE_NAME/plugins"
fi
PLUGIN_DESTINATION_ROOT=$(python3 -c 'import os,sys; print(os.path.abspath(os.path.expanduser(sys.argv[1])))' "$PLUGIN_DESTINATION_ROOT")

PLUGIN_NAME="codex_oracle"
PLUGIN_COPY_TARGET="$PLUGIN_DESTINATION_ROOT/$PLUGIN_NAME"

ARCH=$(uname -m)
case "$ARCH" in
  arm64|aarch64) MCP_TEMPLATE=".mcp.macos-arm64.json" ;;
  x86_64|amd64) MCP_TEMPLATE=".mcp.macos-amd64.json" ;;
  *) echo "Unsupported macOS architecture: $ARCH" >&2; exit 1 ;;
esac

if [ ! -f "$PLUGIN_ROOT/.codex-plugin/plugin.json" ]; then
  echo "Plugin manifest not found: $PLUGIN_ROOT/.codex-plugin/plugin.json" >&2
  exit 1
fi
MANIFEST_NAME=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8")).get("name", ""))' "$PLUGIN_ROOT/.codex-plugin/plugin.json")
if [ "$MANIFEST_NAME" != "$PLUGIN_NAME" ]; then
  echo "Unexpected plugin name '$MANIFEST_NAME'; expected $PLUGIN_NAME" >&2
  exit 1
fi
if [ ! -f "$PLUGIN_ROOT/$MCP_TEMPLATE" ]; then
  echo "MCP template not found: $PLUGIN_ROOT/$MCP_TEMPLATE" >&2
  exit 1
fi

if [ "$APPLY" -ne 1 ]; then
  python3 - "$PLUGIN_ROOT" "$PLUGIN_COPY_TARGET" "$MARKETPLACE_PATH" "$MARKETPLACE_NAME" "$MCP_TEMPLATE" <<'PY'
import json, sys
plugin_root, copy_target, marketplace_path, marketplace_name, mcp_template = sys.argv[1:]
print(json.dumps({
  "status": "dry-run",
  "willWrite": False,
  "approvalRequired": True,
  "pluginRoot": plugin_root,
  "pluginCopyTarget": copy_target,
  "marketplacePath": marketplace_path,
  "marketplaceName": marketplace_name,
  "mcpTemplate": mcp_template,
}, indent=2))
PY
  exit 0
fi

python3 "$SCRIPT_DIR/install-macos.py" "$PLUGIN_ROOT" "$PLUGIN_COPY_TARGET" "$MARKETPLACE_PATH" "$MARKETPLACE_NAME" "$MCP_TEMPLATE" "$FORCE"
