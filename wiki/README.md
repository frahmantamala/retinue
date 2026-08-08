# The wiki — schema only

This directory holds the **schema** for a compiled knowledge base, not a knowledge base.
`CLAUDE.md` governs page types, frontmatter, named edges, and supersession; `commands/` holds
`/ingest`, `/query`, and `/lint`.

The pages themselves — `concepts/`, `entities/`, `decisions/`, `raw/` — are gitignored on purpose.
Mine contain a client's pricing and an employer's domain knowledge. Yours will contain something
equally unsuited to a public repo. The schema is the part worth sharing.

## Starting your own

```bash
mkdir -p "${RETINUE_WIKI:-$HOME/work/wiki}"/{concepts,entities,decisions,raw}
cd ~/work/retinue && ./install.sh      # links CLAUDE.md + commands into it
```

Then `/ingest <url or file>` and let it compile. The wiki earns its keep somewhere around the
twentieth page, when answering a question starts requiring the connection between two sources
rather than a lookup in one.

## Why compiled instead of retrieved

Retrieval re-reads raw documents on every question and gets no smarter over time. A compiled wiki
resolves contradictions once, records what supersedes what, and hands the next session a fact
instead of a corpus. It also stays greppable, diffable, and readable by a human — three properties
a vector index does not have.

At personal scale this is not a close call: `rg` over a few hundred markdown files beats an
embedding pipeline you have to run, and costs nothing to maintain.
