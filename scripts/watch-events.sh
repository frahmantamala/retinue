#!/usr/bin/env bash
# Stopgap agent-activity tail. Run in a second terminal while a team is working.
#
# This is NOT the monitor — `retinue watch` (see _design/MONITOR-LANES.md) replaces it with a
# real graph. This exists so there is something to watch during the run that builds that.
#
#   scripts/watch-events.sh                 # newest session in the current project
#   scripts/watch-events.sh <session-id>    # a specific session
#
# Read-only. Never writes to ~/.claude.
set -uo pipefail

command -v jq >/dev/null || { echo "needs jq: brew install jq" >&2; exit 1; }

PROJ_DIR="$HOME/.claude/projects/$(pwd | sed 's|/|-|g')"
[ -d "$PROJ_DIR" ] || PROJ_DIR="$HOME/.claude/projects/-Users-$(whoami)-work"
[ -d "$PROJ_DIR" ] || { echo "no project dir found under ~/.claude/projects" >&2; exit 1; }

if [ $# -ge 1 ]; then
  FILE="$PROJ_DIR/$1.jsonl"
else
  FILE=$(ls -t "$PROJ_DIR"/*.jsonl 2>/dev/null | head -1)
fi
[ -f "$FILE" ] || { echo "no session log at $FILE" >&2; exit 1; }

echo "watching $(basename "$FILE")"
echo "crew lines are highlighted — if none appear, no subagent has been spawned yet"
echo

tail -n 0 -F "$FILE" | jq -rc --unbuffered '
  ((.timestamp // "") | if . == "" then "--:--:--" else .[11:19] end) as $t
  | (if .isSidechain == true then "CREW" else "lead" end) as $who
  | (if .type == "assistant" then
       ((.message.content // []) | map(
          if .type=="tool_use" then "→ " + .name + (if .input.description then " (" + .input.description + ")" else "" end)
          elif .type=="thinking" then "· thinking"
          elif .type=="text" then "· says"
          else empty end) | join("  "))
     elif .type == "user" then
       ((.message.content // []) | if type=="array"
          then (if (map(select(.type=="tool_result")) | length) > 0 then "← result" else "· input" end)
          else "· input" end)
     else empty end) as $act
  | select($act != "" and $act != null)
  | "\($t)  \($who)  \($act)"' \
| while IFS= read -r line; do
    case "$line" in
      *"CREW"*) printf '\033[38;5;179m%s\033[0m\n' "$line" ;;
      *)        printf '\033[2m%s\033[0m\n' "$line" ;;
    esac
  done
