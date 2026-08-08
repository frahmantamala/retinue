---
description: Health-check the wiki for contradictions, orphans, and stale links
---

Run the `/lint` procedure from `CLAUDE.md`. Report:
- Contradictions between pages
- Orphan pages (no inbound `[[links]]`)
- Dead links — `[[slugs]]` referenced but never written
- `index.md` entries pointing to files that don't exist, and existing pages missing from `index.md`
- Stale `updated:` dates worth revisiting

Report findings as a list. Fix only trivial mechanical issues (broken slug, missing index line); leave judgment calls for me.
