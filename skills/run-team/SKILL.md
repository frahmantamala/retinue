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
3. **Baseline the tree.** Before decomposing — and so before any spawn — run the build and the suite and record every failure. The gate judges the *delta* against that baseline, not the absolute, so pre-existing or flaky failures are not charged to the crew.
4. **Decompose into lanes** — one teammate per independent stream. If the ask arrives as a PRD, run `pm` *first and alone*, then cut the lanes from its stories; it is not a parallel lane. Name the registered agent that runs each lane, and the paths it owns; the paths are what keep the lanes disjoint.
   - `backend` — server/service code, migrations, API contracts, backend tests
   - `frontend` — components, styles, stores, composables/hooks, frontend tests
   - `test-writer` — test files over another lane's code (integration and cross-lane; each implementer already tests its own)
   - `docs-writer` — `.md` brought back in sync with the code
   - `pr-writer` — PR title and body from the branch diff, once the tree is green

   Review is not a standing lane either: spawn a fresh `code-reviewer` per stream at gate time, read-only over that stream's diff, plus one at the end over the combined diff. One long-lived reviewer accumulates every lane's context and ends the run as its most expensive agent.

   `general-purpose` is the fallback, not the default: use it only for a lane no persona above covers (an infra script, a one-off data fix). It carries no scope boundary, so that brief must supply one.
5. **State the contract and the dependencies.** Write the API shape / shared types up front so lanes don't diverge. State which lanes block on which (e.g. tests depend on backend routes existing) BEFORE dependent work starts; a blocked teammate stays queued, not guessing.
6. **Brief every teammate with only what its persona cannot know**: the lane's task, its files, the pinned contract, the architecture summary from step 1, the settled decisions from the wiki, and the **escalation column** from the playbook table. Scope discipline and comment discipline are already in the persona — restating them is waste. The brief is that agent's cached prompt prefix, re-read on every turn it takes, so a duplicated paragraph is paid for hundreds of times.
7. **Spawn and supervise.**
   - **Supervision signal.** Between reports you see no teammate output. What you can read is `TaskList` — status and owner, no timestamps. A task `in_progress` whose owner has produced no report across two consecutive polls gets a `SendMessage` status ping; on a thin answer or none, re-brief or reassign it — same for one that wandered out of lane. (`retinue watch` is the human's view, not yours.)
   - **Budget.** You cannot read run cost; the human watching `retinue watch` calls it. When told you are at 70% of `--max-budget-usd` with lanes still open, stop spawning — no new lanes, no replacement agents — and drive the open lanes to a gate.
   - **No source from you.** Send a remaining gap to the lane that owns those paths, spawning a fresh agent of that persona if the first has finished.
8. **Verify each lane as it reports.** Run the build, the tests, the endpoint yourself — a teammate's success report is a claim, not evidence. Don't hold the fast lane until the crew is done; gate it now, and run one integration verify over the whole tree after the last lane closes.
9. **Gate, then commit.** A lane passes on *no new failures against the baseline* AND approval from a `code-reviewer` spawned fresh for it. On pass, `git add -- <the lane's paths>` (its new files are untracked, so a bare `git commit -- <path>` errors) and commit exactly those paths — **never `-a`, never a bare `git commit`**, or you sweep another lane's half-finished work into the commit that is supposed to be revertible on its own. On failure, assign a specific fix and loop back to step 7 — **max two cycles per finding**, keyed on normalised `file:line` + category from the reviewer's report so a reworded bug is not a fresh one — then escalate. The integration review at the end gates the same way, against the lane that owns the paths.
10. **Capture.** Write decisions made during the run to `$RETINUE_WIKI/decisions/YYYY-MM-DD-slug.md` with `supersedes::[[...]]` edges where you overrode an earlier call, and add the page to `index.md`. Skip only if the run made no decision worth inheriting.
11. **Report once**, at the end.

## Escalation

Stop the run and ask the user only for: destructive migrations, `DROP`/`TRUNCATE`/`DELETE` without
`WHERE`, deleting files you did not create, scope growth beyond the brief, a third fix cycle on one
finding, or the budget cap. Everything else you decide. Full table in the playbook. Escalating does
not skip step 10 — capture what the run learned first, then report.

## Output

At the end: what was built, what you decided and why, what Review caught, what you captured to the
wiki, and anything you left out. Mid-run output is escalations only.
