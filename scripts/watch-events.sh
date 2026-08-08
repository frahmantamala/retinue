#!/usr/bin/env bash
# Plain-text agent-activity tail. Run in a second terminal while a team is working.
#
# `retinue watch` is the graph view and the better default; this stays for the cases a canvas is
# worse than a log — piping into grep, a machine with no browser, or reading the actual tool calls
# in order.
#
#   scripts/watch-events.sh                    # newest session in the current repo
#   scripts/watch-events.sh ~/work/your-app    # newest session in another repo
#   scripts/watch-events.sh <session-id>       # a specific session in the current repo
#   scripts/watch-events.sh ~/work/your-app <session-id>
#   scripts/watch-events.sh -n 0               # live only, no backfill (default backfill: 10)
#
# Level 3 teammates do NOT appear as isSidechain lines in the lead transcript — they get their
# own files under <session-id>/subagents/. This tails the lead plus every teammate, and picks up
# teammates that spawn after it starts.
#
# Read-only. Never writes to ~/.claude.
set -uo pipefail

command -v jq >/dev/null || { echo "needs jq: brew install jq" >&2; exit 1; }

REPO="$PWD"
SESSION=""
BACKFILL=10

while [ $# -gt 0 ]; do
  case "$1" in
    -n) BACKFILL="$2"; shift 2 ;;
    -h|--help) sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *)  if [ -d "$1" ]; then REPO="$1"; else SESSION="$1"; fi; shift ;;
  esac
done

REPO=$(cd "$REPO" 2>/dev/null && pwd) || { echo "not a directory" >&2; exit 1; }
PROJ_DIR="$HOME/.claude/projects/$(printf '%s' "$REPO" | sed 's|/|-|g')"
[ -d "$PROJ_DIR" ] || {
  echo "no transcripts for $REPO" >&2
  echo "(expected $PROJ_DIR — run this from the repo the team is working in)" >&2
  exit 1
}

if [ -n "$SESSION" ]; then
  LEAD="$PROJ_DIR/$SESSION.jsonl"
else
  LEAD=$(ls -t "$PROJ_DIR"/*.jsonl 2>/dev/null | head -1)
fi
[ -f "${LEAD:-}" ] || { echo "no session log in $PROJ_DIR" >&2; exit 1; }
SUB_DIR="${LEAD%.jsonl}/subagents"

echo "repo    $REPO"
echo "session $(basename "${LEAD%.jsonl}")"
echo "lanes   $SUB_DIR"
echo

# Each line a stream emits: TIME \t COLOR \t LABEL \t ACTIVITY
JQ_PROG='
  ((.timestamp // "")
     | if . == "" then "--:--:--"
       else (try (sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601 | strflocaltime("%H:%M:%S")) catch .[11:19]) end) as $t
  | (if .type == "assistant" then
       ((.message.content // []) | map(
          if .type=="tool_use" then "-> " + .name + (if (.input.description // "") != "" then " (" + .input.description + ")" else "" end)
          elif .type=="thinking" then "· thinking"
          elif .type=="text" then "· " + ((.text // "") | gsub("\\s+"; " ") | .[0:80])
          else empty end) | join("  "))
     elif .type == "user" then
       ((.message.content // []) | if type=="array"
          then (if (map(select(.type=="tool_result")) | length) > 0 then "<- result" else "· input" end)
          else "· input" end)
     else empty end) as $act
  | select($act != "" and $act != null and ($act | ltrimstr("· ") | length) > 0)
  | "\($t)\t\($color)\t\($who)\t\($act)"'

PALETTE="179 110 114 176 215 109 173 146"

# A teammate is named in its sidecar meta; nested subagents (spawnDepth > 0) get a > prefix.
label_for() {
  local f="$1" meta="${1%.jsonl}.meta.json" name=""
  if [ -f "$meta" ]; then
    name=$(jq -r 'if (.spawnDepth // 0) > 0
                  then ">" + (.agentType // "sub")
                  else (.name // .agentType // empty) end' "$meta" 2>/dev/null)
  fi
  case "$name" in ""|null) name=$(basename "$f" .jsonl) ;; esac
  printf '%s' "$name"
}

# Colors go by discovery order, not by hash — a hash collides and two lanes look alike.
NEXT_COLOR=0
COLOR=""
next_color() {  # sets $COLOR; not a subshell, so the counter survives
  local i=0 c n
  set -- $PALETTE
  n=$(( NEXT_COLOR % $# ))
  NEXT_COLOR=$(( NEXT_COLOR + 1 ))
  for c in "$@"; do
    [ "$i" -eq "$n" ] && { COLOR="$c"; return; }
    i=$(( i + 1 ))
  done
}

PIDS=""
stream() {
  tail -n "$BACKFILL" -F "$1" 2>/dev/null \
    | jq -rc --unbuffered --arg who "$2" --arg color "$3" "$JQ_PROG" &
  PIDS="$PIDS $!"
}

{
  trap 'kill $PIDS 2>/dev/null' EXIT INT TERM
  stream "$LEAD" "lead" "244"

  seen=""
  while :; do
    for f in "$SUB_DIR"/agent-*.jsonl; do
      [ -e "$f" ] || continue
      case "$seen" in *"|$f|"*) continue ;; esac
      seen="$seen|$f|"
      label=$(label_for "$f")
      next_color
      stream "$f" "$label" "$COLOR"
    done
    sleep 2
  done
} | while IFS=$'\t' read -r t color who act; do
  printf '\033[38;5;%sm%s  %-14.14s %s\033[0m\n' "$color" "$t" "$who" "$act"
done
