# Wiki — Schema & Operating Rules

This repo is an **LLM-maintained knowledge base**: a compiled wiki, not a document dump.
Claude is the maintainer; the markdown pages are the codebase; Obsidian is the viewer.
Pattern: compile knowledge into interlinked pages once, then maintain it — don't re-read raw
sources on every question. See [[concepts/compilation-vs-retrieval]].

## Scope

**In scope:** engineering / learning notes, and business / product knowledge (strategy,
pricing, market research, decisions and their rationale).

**Out of scope — do NOT duplicate here:**
- How Claude should work with my code → that's the auto-memory at
  `~/.claude/projects/<project-slug>/memory/`.
- Code structure / architecture of a specific app → that app's own `CLAUDE.md`.
- Anything derivable from a repo's git history.

If a note is really about a single project's internals, it belongs in that project, not here.

## Three layers

1. **`raw/`** — source captures (articles, PDFs, talks, my own dumped notes). **Immutable.**
   Never edit a file in `raw/` after ingesting it; it is ground truth. One source = one file.
2. **The wiki** — `concepts/`, `entities/`, `decisions/`, plus `index.md` and `CHANGELOG.md`.
   These are the pages Claude writes and continuously re-links.
3. **This schema** — `CLAUDE.md`. Governs page shape and the three operations below.

## Page types & folders

| Folder | Holds | Filename |
|--------|-------|----------|
| `concepts/` | Ideas, patterns, techniques, mental models | `kebab-case.md` |
| `entities/` | People, companies, tools, products | `kebab-case.md` |
| `decisions/` | A choice I made + why (business/product/eng) | `YYYY-MM-DD-slug.md` |
| `raw/` | Immutable source captures | `YYYY-MM-DD-slug.md` |

## Frontmatter (every wiki page)

```yaml
---
type: concept | entity | decision | source
tags: [short, kebab-case]
sources: ["[[raw/2026-07-18-some-source]]"]   # where this knowledge came from
updated: 2026-07-18
---
```

## Linking discipline

- Cross-link with `[[page-slug]]` liberally — a link to a page that doesn't exist yet is fine;
  it marks a page worth writing later. The graph is the value.
- Every wiki page should link to at least one other page and, where relevant, back to its `raw/` source.
- Prefer clear page titles over long prose. One idea per page; split when a page grows two topics.
- **Link across clusters, not just within them.** A bare `[[link]]` inside a topic you're already
  writing is cheap; the valuable edge is the one joining two areas that had nothing to do with each
  other. When ingesting, ask what this touches *elsewhere* in the wiki.

## Named edges

Inline `[[links]]` stay as they are — they carry prose. But a bare link only says "related somehow",
which is not enough to act on. Add a `## Relations` block at the end of a page with **typed** edges:

```markdown
## Relations

- supersedes::[[decisions/2026-07-18-monolith-first]]
  - The monolith call stands; treating the old system as a spec does not.
- derived_from::[[raw/2026-07-19-domain-notes]]
- contradicts::[[concepts/some-opposing-view]]
```

Rules:

- Syntax is `- predicate::[[target]]`, in the body, never in frontmatter. Obsidian renders it fine
  and `rg 'supersedes::'` finds it — no plugin, no build step.
- Predicates are multi-word with underscores. `derived_from::` not `source::` — single words collide.
- Vocabulary: `supersedes` · `contradicts` · `extends` · `implements` · `constrains` ·
  `derived_from` · `informed_by` · `evidence_for` · `conforms_to`. Use `relates_to::` only as a last resort.
- Add an indented line under an edge when *why* it matters isn't obvious from the two titles.

## Supersession — never silently overwrite

When new knowledge replaces old, the old page **stays**. Nothing vanishes; the graph records the change.

1. Add `supersedes::[[old-page]]` to the new page, with one line on what changed.
2. Add `superseded_by::[[new-page]]` near the top of the old page, and leave its content intact.
3. If two pages disagree and neither wins yet, say so on both with `contradicts::` — an open
   contradiction is knowledge, and pretending to a resolution you don't have is not.

A knowledge base that never records where it changed its mind decays into confident averages.

## The three operations

### `/ingest <source>` — add knowledge
1. Save the source verbatim (or a faithful capture + the URL) to `raw/YYYY-MM-DD-slug.md`.
2. Read it. Identify the concepts / entities / decisions it touches.
3. For each: **update the existing page** if one exists (merge, note contradictions), else create it.
   Aim to touch several related pages, not just one — that's what makes knowledge compound.
4. Add/refresh `[[links]]` between the touched pages, and a `## Relations` block with typed edges.
   If this replaces an earlier claim, follow the supersession rule above.
5. Append a dated line to `CHANGELOG.md` and add any new page to `index.md`.

### `/query <question>` — answer from the wiki
- Answer from the **wiki pages**, not by re-reading `raw/`. Cite the pages used (`file:line` or `[[slug]]`).
- If the pages can't answer it, say so and suggest what to ingest — don't silently fall back to guessing.

### `/lint` — health check (report, don't auto-fix beyond trivial)
- Contradictions between pages; orphan pages (no inbound links); stale `updated` dates;
  dead `[[links]]` that never got written; `index.md` entries that no longer exist.
- **Cluster check.** Build the link graph and find its connected components. Report any cluster
  joined to the rest by a single edge, and any pair of clusters that *should* touch but don't —
  that bridge is usually the most valuable page nobody has written.
- **Untyped pages.** Pages with inline `[[links]]` but no `## Relations` block.
- Report findings; fix only obvious mechanical issues (broken link slug, missing index entry).

## Conventions
- Markdown only. No build step, no infra. Open the folder as an Obsidian vault for the graph view.
- Sparse, meaningful prose. Convert relative dates to absolute (today is 2026-07-18 when seeding).
