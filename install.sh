#!/usr/bin/env bash
# Symlink this repo into ~/.claude so edits here take effect immediately.
# Existing files are backed up before anything is replaced.
#
# agents/ skills/ templates/ are linked per-entry, not as whole directories,
# so anything you keep locally but don't publish survives an install.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEST="${CLAUDE_HOME:-$HOME/.claude}"
WIKI="${RETINUE_WIKI:-$HOME/work/wiki}"
BAK="/tmp/claude-bak/install.$(date +%s)"

echo "repo   $REPO"
echo "target $DEST"
echo "wiki   $WIKI"
echo

mkdir -p "$DEST" "$BAK"

backup_and_link() {
  local src="$1" dst="$2" rel
  if [ -e "$dst" ] || [ -L "$dst" ]; then
    rel="${dst#/}"; rel="${rel//\//_}"          # flatten so backups never collide
    cp -aL "$dst" "$BAK/$rel" 2>/dev/null || cp -a "$dst" "$BAK/$rel"
    rm -rf "$dst"
  fi
  mkdir -p "$(dirname "$dst")"
  ln -s "$src" "$dst"
  echo "linked $dst"
}

for f in CLAUDE.md TEAM-PLAYBOOK.md; do
  backup_and_link "$REPO/$f" "$DEST/$f"
done

# Per-entry so local-only agents/skills are left alone.
for dir in agents skills templates; do
  mkdir -p "$DEST/$dir"
  for entry in "$REPO/$dir"/*; do
    [ -e "$entry" ] || continue
    backup_and_link "$entry" "$DEST/$dir/$(basename "$entry")"
  done
done

# The wiki: schema only. Pages are yours and are never touched.
if [ -d "$WIKI" ]; then
  mkdir -p "$WIKI/.claude"
  backup_and_link "$REPO/wiki/CLAUDE.md" "$WIKI/CLAUDE.md"
  backup_and_link "$REPO/wiki/commands" "$WIKI/.claude/commands"
else
  echo "note: no wiki at $WIKI — mkdir it and re-run, or set RETINUE_WIKI"
fi

echo
echo "backup $BAK"
echo
echo "settings.json is NOT linked — it holds machine-specific state."
echo "Merge settings.example.json into $DEST/settings.json by hand."
