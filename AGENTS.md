# AGENTS.md

Runner-neutral entry point. Claude Code reads `CLAUDE.md`; Codex, Cursor, and others read this file.
Both describe the same system — this one leaves out the Claude-specific plumbing.

## What this repo gives an agent

1. **A supervision contract** — `TEAM-PLAYBOOK.md`, "The retinue contract". How a lead agent
   decomposes work, supervises a crew, verifies before merging, bounds its retries, and decides what
   it may do alone versus what stops the run.
2. **Role definitions** — `agents/*.md`. Five modes (SWE, Frontend, PM, Designer, QA) plus review,
   test, docs, and PR roles. Each declares what it may and may not modify.
3. **A knowledge-base schema** — `wiki/CLAUDE.md`. How to compile durable knowledge into linked
   markdown with typed edges and explicit supersession.

All three are plain markdown. Nothing here needs a runtime, an index, or a vector store.

## Working rules

- **Scope boundaries are binding.** A frontend role does not touch migrations. A QA role reports
  bugs and never fixes source. If work crosses a boundary, finish one side, then switch roles.
- **Comments are sparse and meaningful.** Explain *why*, never restate the code. No narrative blocks.
- **The lead does not write source.** It briefs, supervises, verifies, and gates. Gaps get assigned.
- **Verify before believing.** An agent reporting success is a claim. Run the build and the tests.
- **Bounded retries.** At most two fix cycles on the same finding, then escalate to the human.
- **Never work around a denied operation.** If the runner blocks `git push`, `sudo`, or `rm`, that
  block is the point — surface it, don't route around it.

## Durable context

A compiled wiki (`$RETINUE_WIKI`, default `~/work/wiki`) holds decisions and domain knowledge that outlive a session.
Read it at the start of a run so settled decisions are not re-litigated; write back what the run
decided, with `supersedes::[[old-page]]` edges where it overrode an earlier call. `wiki/CLAUDE.md`
is the schema. Nothing here is Claude-specific — it is markdown and `rg`.

## What is Claude Code-specific

Portable: the contract, the roles, the patterns, the wiki schema, and the `/ingest`, `/query`,
`/lint`, and `/capture` procedures. Any agent that can read files and run commands can follow them.

Not portable, and needing an equivalent on another runner:

| Claude Code | What it does | Elsewhere |
|---|---|---|
| `skills/*/SKILL.md` | procedures invoked as `/name` | paste the body, or the runner's own command format |
| `agents/*.md` frontmatter | per-role model and tool selection | the runner's subagent config, if it has one |
| `settings.json` | permission allow/deny, pre-tool hooks | the runner's permission model |
| `CLAUDE_CODE_SUBAGENT_MODEL` | routes crew to a model | the runner's equivalent env var |
| Agent teams | crew that shares a task list | run the contract single-threaded if unsupported |

The contract degrades gracefully: a runner with no crew support can still follow the loop
sequentially — decompose, do one lane, verify, gate, capture. It is slower, not broken.
