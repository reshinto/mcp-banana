#!/usr/bin/env bash
# SessionStart + branch creation hook: auto-switch plugins based on git branch prefix.
# Reads plugin-profiles.json and updates settings.local.json enabledPlugins.
#
# CRITICAL: Only manages plugins listed in plugin-profiles.json. Any plugins already
# present in settings.local.json that are NOT in plugin-profiles.json are preserved
# (merged, not overwritten). This ensures the forge plugin itself is never disabled.
#
# Uses Python for JSON manipulation (available on macOS/Linux without extra installs).
# Falls back to a simple sed-based approach if Python is not available.
# Always exits 0 — never blocks session start.

set -euo pipefail

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
PROFILES="$PROJECT_DIR/.claude/hooks/plugin-profiles.json"
SETTINGS="$PROJECT_DIR/.claude/settings.local.json"

# Guard: profiles must exist; settings.local.json is created if missing
if [ ! -f "$PROFILES" ]; then
  echo "WARN: plugin-profiles.json missing — plugin auto-switching disabled" >&2
  exit 0
fi

if [ ! -f "$SETTINGS" ]; then
  echo '{}' > "$SETTINGS"
fi

# Get current branch (handle detached HEAD)
BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")
if [ -z "$BRANCH" ] || [ "$BRANCH" = "HEAD" ]; then
  BRANCH="main"
fi

# Allow override from argument (used by branch creation hook)
if [ -n "${1:-}" ]; then
  BRANCH="$1"
fi

# Use Python (available on macOS and virtually all Linux) for JSON manipulation.
# Python is more universally available than jq or Node.js.
PYTHON=""
for PY in python3 python; do
  if command -v "$PY" &>/dev/null; then
    PYTHON="$PY"
    break
  fi
done

if [ -z "$PYTHON" ]; then
  echo "WARN: python3/python not found — plugin auto-switching disabled" >&2
  exit 0
fi

$PYTHON - "$PROFILES" "$SETTINGS" "$BRANCH" << 'PYEOF'
import json
import sys
import os
import tempfile

profiles_path = sys.argv[1]
settings_path = sys.argv[2]
branch = sys.argv[3]

try:
    with open(profiles_path) as f:
        profiles = json.load(f)
    with open(settings_path) as f:
        settings = json.load(f)

    # Collect all plugin names managed by this profiles file
    managed = set(profiles.get("core", []))
    for plugins in profiles.get("branch_modes", {}).values():
        managed.update(plugins)

    # Start with existing enabledPlugins, preserving unmanaged entries
    existing = settings.get("enabledPlugins", {})
    merged = {}

    # Copy unmanaged plugins (not in profiles) as-is
    for plugin_id, enabled in existing.items():
        if plugin_id not in managed:
            merged[plugin_id] = enabled

    # Set all managed plugins to false initially
    for plugin in managed:
        merged[plugin] = False

    # Enable core plugins
    for plugin in profiles.get("core", []):
        merged[plugin] = True

    # Enable branch-matched plugins
    for prefix, plugins in profiles.get("branch_modes", {}).items():
        if branch.startswith(prefix):
            for plugin in plugins:
                merged[plugin] = True

    # Check if anything changed (managed plugins only)
    old_managed = {p: existing.get(p, False) for p in managed}
    new_managed = {p: merged[p] for p in managed}
    changed = old_managed != new_managed

    settings["enabledPlugins"] = merged

    # Atomic write: temp file + rename
    dir_name = os.path.dirname(settings_path)
    fd, tmp_path = tempfile.mkstemp(dir=dir_name, suffix=".tmp")
    with os.fdopen(fd, "w") as f:
        json.dump(settings, f, indent=2)
        f.write("\n")
    os.replace(tmp_path, settings_path)

    enabled_count = sum(1 for v in merged.values() if v)
    print(f'Plugin mode: branch "{branch}" — enabled {enabled_count} plugins')
    if changed:
        print("Plugins changed — restart session (new chat) for changes to take effect")

except Exception as e:
    print(f"WARN: plugin auto-switch failed: {e}")
PYEOF

exit 0
