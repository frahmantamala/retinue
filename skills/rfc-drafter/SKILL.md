---
name: rfc-drafter
description: Draft a review-ready RFC (architecture decision document) in the house format — numbered sections, explicit "Decision asked", non-goals, alternatives, open questions for reviewers. Use when an architectural or cross-cutting change needs to be socialized with EM/TLs before any code is written, or when the user says "write an RFC", "draft a design doc", "propose the architecture for X".
---

# RFC Drafter

Produce a document, not code. This skill is PM/Designer scope: `.md` files only.
If the user wants implementation, they want a different mode — say so and stop.

## When NOT to use

- Bug fixes, single-module changes, anything one person can decide alone.
- The decision is already made and just needs writing down — that's a wiki
  `decision` entry, not an RFC.

## Preconditions

1. Read the repo's `CLAUDE.md` and `_design/CLAUDE.md` if present.
2. Read the most recent RFC in `_design/` — match its depth and tone, not just
   its headings.
3. Read `_design/index.md` if present; you will add an entry to it.
4. Check `~/work/wiki/` for relevant `decisions/` and `entities/` — an RFC that
   contradicts a recorded decision must say so explicitly.

## Interview first

Do not start writing until these are answered. Ask them in one batch:

- **What decision are reviewers being asked to approve?** One sentence. If this
  can't be stated crisply, the RFC isn't ready to write.
- What breaks or costs money if nothing changes?
- What is explicitly out of scope?
- Which options were genuinely considered, and why were they set aside?
- What must be decided by someone else before this can proceed?

## Numbering and location

- Write to `<project>/_design/RFC-NNNN-<kebab-slug>.md`.
- `NNNN` = highest existing RFC number in that `_design/` plus one, zero-padded
  to 4. Scan first; never guess.

## Front matter

Keep it wiki-ingestible — RFCs are `type: decision` and flow into the wiki.

```yaml
---
type: decision
title: "RFC-NNNN — <full title>"
status: Draft (for review)
author: <you>
date: <today, absolute>
sources: ["[[analysis/current-system]]"]
---
```

## Structure

```
# RFC-NNNN — <short title>

**Status:** Draft · for review with EM/TLs
**Decision asked:** <the one thing reviewers must approve>

## 1. Summary
## 2. Why change
## 3. Non-goals
## 4. Target architecture       <- sub-numbered 4.1, 4.2, ... as needed
## 5. Migration plan            <- phased; omit for greenfield
## 6. Risks & mitigations
## 7. Alternatives considered
## 8. Open decisions for reviewers
```

Sections 3, 6, 7, 8 are not optional. They are what makes the document
reviewable instead of a proposal nobody can argue with.

## Rules

- **Diagrams are Mermaid**, never ASCII art. Module maps, sequence flows,
  migration phases.
- **Alternatives need real tradeoffs.** "We considered microservices but chose a
  monolith" is not an alternative — state what it would have cost and what it
  would have bought.
- **Open decisions are addressed to people.** Each one names what's blocked and
  who decides, so a reviewer knows what their answer unblocks.
- **No code.** Snippets only where a directory layout or a type signature *is*
  the decision. Never a working implementation.
- **Length: 130–220 lines.** Longer means the scope is too broad — split it.
- Prose is direct. No filler sections, no restating the summary at the end.

## Finish

1. Add the entry to `_design/index.md`.
2. Report the file path and the one-sentence decision asked — nothing else.
