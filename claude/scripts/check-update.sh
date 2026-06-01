#!/usr/bin/env bash
# Humify update notifier (Claude adapter).
# Runs at SessionStart. Fail-silent and rate-limited to once per 24h, so it
# never blocks or noises up a session. Prints a user-visible systemMessage only
# when the upstream repo has a newer version than the installed plugin.
set -uo pipefail

root="${CLAUDE_PLUGIN_ROOT:-}"
[ -z "$root" ] && exit 0
manifest="$root/.claude-plugin/plugin.json"
upstream="https://raw.githubusercontent.com/schylerchase/humify/main/claude/.claude-plugin/plugin.json"

# Rate-limit: skip if we checked within the last 24h.
stamp=""
data="${CLAUDE_PLUGIN_DATA:-}"
if [ -n "$data" ]; then
  mkdir -p "$data" 2>/dev/null || true
  stamp="$data/last-update-check"
  if [ -f "$stamp" ]; then
    last="$(cat "$stamp" 2>/dev/null || echo 0)"
    now="$(date +%s)"
    [ $(( now - last )) -lt 86400 ] && exit 0
  fi
fi

# Extract a "version" value from a plugin.json on stdin, no jq dependency.
read_version() { grep -m1 '"version"' 2>/dev/null | sed -E 's/.*"version"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/'; }

installed="$(read_version < "$manifest" 2>/dev/null || true)"
[ -z "$installed" ] && exit 0

remote_json="$(curl -fsSL --max-time 3 "$upstream" 2>/dev/null || true)"
# Record the check time regardless of network outcome.
[ -n "$stamp" ] && { date +%s > "$stamp" 2>/dev/null || true; }
[ -z "$remote_json" ] && exit 0

remote="$(printf '%s' "$remote_json" | read_version)"
[ -z "$remote" ] && exit 0
[ "$remote" = "$installed" ] && exit 0

# Notify only when the upstream version sorts strictly newer.
newer="$(printf '%s\n%s\n' "$installed" "$remote" | sort -V | tail -n1)"
if [ "$newer" = "$remote" ]; then
  msg="Humify update available (installed $installed, latest $remote). Update with: claude plugin marketplace update humify && claude plugin update humify@humify, then restart Claude."
  printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","systemMessage":"%s"}}\n' "$msg"
fi
exit 0
