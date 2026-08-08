---
description: Ingest a source (URL or file) into the wiki
---

Ingest `$ARGUMENTS` into this wiki following the `/ingest` procedure in `CLAUDE.md`:

1. Capture the source to `raw/YYYY-MM-DD-slug.md` (verbatim text or a faithful summary + the URL). Immutable.
2. Read it; identify the concepts / entities / decisions it touches.
3. Update existing pages where they exist (merge, flag contradictions), create pages where they don't. Touch the several related pages, not just one.
4. Add/refresh `[[links]]` between touched pages and back to the raw source.
5. Append a dated entry to `CHANGELOG.md` and add any new page to `index.md`.

Report which pages you created vs updated.
