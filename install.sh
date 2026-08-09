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

# Linking alone never notices a deletion: an entry dropped from the repo leaves a
# dangling link the runner still tries to load. Only our own dead links go —
# real files, real dirs and links pointing elsewhere are the user's, not ours.
prune_stale_links() {
  local dir="$1" entry target rel
  [ -d "$dir" ] || return 0
  for entry in "$dir"/*; do
    [ -L "$entry" ] || continue
    [ -e "$entry" ] && continue
    target="$(readlink "$entry")"
    case "$target" in
      "$REPO"/*) ;;
      *) continue ;;
    esac
    rel="${entry#/}"; rel="${rel//\//_}"
    cp -a "$entry" "$BAK/$rel"
    rm "$entry"
    echo "pruned $entry"
  done
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
  prune_stale_links "$DEST/$dir"
done

# The wiki: schema only. Pages are yours and are never touched.
if [ -d "$WIKI" ]; then
  mkdir -p "$WIKI/.claude"
  backup_and_link "$REPO/wiki/CLAUDE.md" "$WIKI/CLAUDE.md"
  backup_and_link "$REPO/wiki/commands" "$WIKI/.claude/commands"
  prune_stale_links "$WIKI/.claude"
else
  echo "note: no wiki at $WIKI — mkdir it and re-run, or set RETINUE_WIKI"
fi

echo
echo "backup $BAK"
echo
echo "settings.json is NOT linked — it holds machine-specific state."
echo "Merge settings.example.json into $DEST/settings.json by hand."
