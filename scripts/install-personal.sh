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

if [ -e "$PLUGIN_COPY_TARGET" ]; then
  if [ "$FORCE" -ne 1 ]; then
    echo "Target already exists. Re-run with --force after approval: $PLUGIN_COPY_TARGET" >&2
    exit 1
  fi
  if [ "$PLUGIN_DESTINATION_ROOT" = "/" ] || [ "$PLUGIN_COPY_TARGET" = "$PLUGIN_DESTINATION_ROOT" ] || [ "$PLUGIN_COPY_TARGET" = "$HOME" ]; then
    echo "Refusing to remove unsafe plugin target: $PLUGIN_COPY_TARGET" >&2
    exit 1
  fi
  case "$PLUGIN_COPY_TARGET" in
    "$PLUGIN_DESTINATION_ROOT"/*) ;;
    *) echo "Refusing to remove a path outside the plugin destination root: $PLUGIN_COPY_TARGET" >&2; exit 1 ;;
  esac
  rm -rf "$PLUGIN_COPY_TARGET"
fi
mkdir -p "$PLUGIN_COPY_TARGET"
for item in "$PLUGIN_ROOT"/* "$PLUGIN_ROOT"/.[!.]* "$PLUGIN_ROOT"/..?*; do
  [ -e "$item" ] || continue
  base=$(basename "$item")
  case "$base" in
    .git|.codex_oracle|.tmp|tmp|sessions|Docs_local|docs_local) continue ;;
  esac
  cp -R "$item" "$PLUGIN_COPY_TARGET/"
done
cp "$PLUGIN_COPY_TARGET/$MCP_TEMPLATE" "$PLUGIN_COPY_TARGET/.mcp.json"
chmod +x "$PLUGIN_COPY_TARGET/bin/codex_oracle_darwin_arm64" "$PLUGIN_COPY_TARGET/bin/codex_oracle_darwin_amd64" 2>/dev/null || true

mkdir -p "$(dirname "$MARKETPLACE_PATH")"
RESOLVED_MARKETPLACE_NAME=$(python3 - "$MARKETPLACE_PATH" "$MARKETPLACE_NAME" "$PLUGIN_NAME" <<'PY'
import json, pathlib, sys
marketplace_path, marketplace_name, plugin_name = sys.argv[1:]
path = pathlib.Path(marketplace_path)
entry = {
  "name": plugin_name,
  "source": {"source": "local", "path": f"./plugins/{plugin_name}"},
  "policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"},
  "category": "Productivity",
}
if path.exists():
  data = json.loads(path.read_text(encoding="utf-8"))
  name = data.get("name") or marketplace_name
  interface = data.get("interface") or {"displayName": "Personal"}
  plugins = [p for p in data.get("plugins", []) if p.get("name") != plugin_name]
else:
  name = marketplace_name
  interface = {"displayName": "Personal"}
  plugins = []
plugins.append(entry)
path.write_text(json.dumps({"name": name, "interface": interface, "plugins": plugins}, indent=2) + "\n", encoding="utf-8")
print(name)
PY
)

echo "Installed codex_oracle plugin files to $PLUGIN_COPY_TARGET"
echo "Updated marketplace file $MARKETPLACE_PATH"
echo "Next: codex plugin add codex_oracle@$RESOLVED_MARKETPLACE_NAME"
