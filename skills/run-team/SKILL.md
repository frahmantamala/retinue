---
name: run-team
description: Scaffold and launch a Level 3 Claude agent team for a multi-file feature with dependencies. Use when the user wants to parallelize backend/frontend/test/docs/review work under a coordinating lead agent.
---

# Run Team

Set up and drive a Level 3 agent team (lead coordinates teammates over a shared task list). Use ONLY when tasks are dependent and span multiple files — otherwise point the user to Agent View (`claude agents`) or a single subagent. See `~/.claude/TEAM-PLAYBOOK.md`.

## Preconditions

1. Confirm `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` is set (`echo $CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS`). If empty, tell the user to `source ~/.zshrc` or open a new shell — do not proceed without it.
2. Confirm teammates are routed to an Opus model (`echo $CLAUDE_CODE_SUBAGENT_MODEL` — expect `claude-opus-*`). If it names an older Opus than the current one, say so once and carry on.

## Steps

You are the **lead** of the retinue. Run the full contract in `~/.claude/TEAM-PLAYBOOK.md`
("The retinue contract"). Drive to completion; do not check in between steps.

1. **Load context — repo and wiki.**
   - Read the repo's `CLAUDE.md` (root + `backend/`/`frontend/` subdirs). If the repo has NO `CLAUDE.md`, warn the user — teammates will infer the architecture and may break conventions. Offer to generate one first. Inject the architecture summary (modular monolith layering, feature-folder boundaries, invariants) into every teammate's brief so they don't diverge.
   - Read `$RETINUE_WIKI/index.md` (default `~/work/wiki`) and pull any decisions touching this repo or this problem. These are settled; teammates must not re-litigate them.
2. **Clarify the feature — once.** If the scope is genuinely ambiguous (different readings produce materially different builds), ask now, in one round. Otherwise state your assumptions in the brief and proceed. Do not stall on questions you can answer from the repo.
3. **Decompose into lanes** — one teammate per independent stream, plus a Review lane. Map each to concrete files/modules. Typical split:
   - Backend — routes/handlers/migrations
   - Frontend — components/forms/state
   - Testing — integration tests for all endpoints
   - Review — security/bugs/style across everything the others produced
4. **State the contract and the dependencies.** Write the API shape / shared types up front so lanes don't diverge. State which lanes block on which (e.g. tests depend on backend routes existing) BEFORE dependent work starts; a blocked teammate stays queued, not guessing.
5. **Brief every teammate** with: its lane and files, the contract, the architecture summary, the relevant settled decisions, and the **escalation column** from the playbook table. Add scope discipline (modify only your lane; Review reports, never fixes) and comment discipline (sparse, meaningful, explain *why*, no narrative blocks).
6. **Spawn and supervise.** Poll progress. Re-brief or reassign agents that stall, wander out of lane, or diverge from the contract. Write no source code yourself — assign remaining gaps to an agent.
7. **Verify yourself.** Run the build, the tests, the endpoint. A teammate's success report is a claim, not evidence.
8. **Gate.** Merge a stream only on green build AND Review approval. On failure, assign a specific fix and loop back to step 6 — **max two cycles per finding**, then escalate.
9. **Capture.** Write decisions made during the run to `$RETINUE_WIKI/decisions/YYYY-MM-DD-slug.md` with `supersedes::[[...]]` edges where you overrode an earlier call, and add the page to `index.md`. Skip only if the run made no decision worth inheriting.
10. **Report once**, at the end.

## Escalation

Stop the run and ask the user only for: destructive migrations, `DROP`/`TRUNCATE`/`DELETE` without
`WHERE`, deleting files you did not create, scope growth beyond the brief, a third fix cycle on one
finding, or the budget cap. Everything else you decide. Full table in the playbook.

## Output

At the end: what was built, what you decided and why, what Review caught, what you captured to the
wiki, and anything you left out. Mid-run output is escalations only.
