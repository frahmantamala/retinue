---
name: capture
description: Capture knowledge from the current session into the LLM wiki at ~/work/wiki — decisions and their rationale, domain facts, constraints that will bind future work. Use at the end of a session that decided something, after an agent-team run, or when the user says "capture this", "save this to the wiki", or "remember why we did this".
---

# Capture

Drain what this session learned into `~/work/wiki`. Works from any repo — the wiki is global.

The wiki decays from neglect, not from bad structure. Most knowledge is lost because nobody
switched repos to run `/ingest`. This skill removes that step.

## What belongs here

Capture a thing if a future session would pay to re-derive it:

- **A decision and its rationale** — what was chosen, what was rejected, and *why*. The why is the
  part that costs money to rebuild.
- **A domain fact learned the hard way** — an API's real behaviour, a legal or regulatory constraint,
  a number that came from a real conversation.
- **A constraint that will bind future work** — something that closes off options later.

## What does NOT belong here

Route it correctly instead of dumping it:

| Knowledge | Goes to |
|---|---|
| How I want Claude to work; my preferences | auto-memory at `~/.claude/projects/*/memory/` |
| A specific app's architecture or conventions | that repo's own `CLAUDE.md` |
| Anything derivable from git history or the code | nowhere — skip it |
| Task state, TODOs, "next steps" | nowhere — that's not knowledge |

If nothing in the session clears that bar, **say so and write nothing.** An empty capture is a
correct outcome; padding the wiki is worse than leaving it alone.

## Steps

1. **Read the schema.** `~/work/wiki/CLAUDE.md` governs page types, frontmatter, named edges, and
   supersession. Follow it exactly — this skill does not restate it.
2. **Read `~/work/wiki/index.md`** to see what already exists. You are usually updating a page, not
   creating one.
3. **Identify the candidates** from this session. Name them to the user in one line each before writing.
4. **For each: merge or create.**
   - An existing page covers it → merge into that page, refresh `updated:`.
   - It replaces an earlier claim → follow the supersession rule. The old page stays.
   - Nothing covers it → create the page in `concepts/`, `entities/`, or `decisions/`.
   - If the source was a real artifact (a conversation, an article, a client message), save it to
     `raw/YYYY-MM-DD-slug.md` first and link back to it. `raw/` is immutable.
5. **Add typed edges.** A `## Relations` block with `supersedes::` / `derived_from::` /
   `constrains::` / `contradicts::`. Bare `[[links]]` alone are not enough.
6. **Reach across clusters.** Before finishing, check what this touches *elsewhere* in the wiki —
   another project, another domain. The graph's weakness is that its clusters barely connect;
   a cross-cluster edge is worth more than a third link inside a cluster you were already in.
7. **Update `index.md` and `CHANGELOG.md`** — one terse line each, dated.

## Conventions

- Convert relative dates to absolute (`today` → the real date).
- Sparse, meaningful prose. One idea per page; split a page that grew two topics.
- Indonesian copy stays direct and practical — no pribahasa, no flowery framing.
- Do not commit unless the user asks.

## Output

One line per page created or updated, and one line naming anything you deliberately did not
capture and why.
