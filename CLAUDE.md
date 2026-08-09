# Development Guide

## CRITICAL SAFETY RULES

**Require explicit approval (the harness also enforces some of these):**
- `git push` - only when I ask for it, never as a follow-on to a commit (prompts, not denied)
- Force push (`-f`, `--force`, `--force-with-lease`) - denied in settings
- `DROP DATABASE` / `DROP TABLE` / `TRUNCATE` - ASK FIRST (a PreToolUse hook forces a prompt; never run without confirmation)
- `DELETE FROM` without WHERE - ASK FIRST
- Destructive migrations - PROPOSE FIRST
- `sudo`, `npm/yarn publish` - denied in settings

**Deleting files/dirs — BACK UP FIRST, then delete:**
- `rm` is allowed (no longer blocked), but before removing any file or directory,
  copy it to a timestamped backup first (e.g. `cp -a <target> /tmp/claude-bak/<name>.$(date +%s)`),
  then delete. Never `rm` something you haven't backed up or didn't create.
- If what you're about to delete contradicts how it was described, stop and surface it.

**Proposal format:**
```
I suggest [action] because [reason].
Impact: [what changes]
Risk: [assessment]
Approve? (I will not execute until confirmed)
```

---

## SCOPE DISCIPLINE

**Each agent MUST only change files within its scope. Do NOT touch files outside your boundary.**
Every agent below carries this table's row in its own persona — this is the shared copy.

| Agent | CAN modify | CANNOT modify |
|------|-----------|---------------|
| **`backend`** | Server code, DB migrations, API specs, backend tests, backend config | Components, styles, stores, composables/hooks, frontend tests |
| **`frontend`** | Components, styles, stores, composables/hooks, client types, frontend tests | Server code, DB migrations, API handlers, backend tests |
| **`pm`** | Stories, acceptance criteria, specs, plans (`.md` only) | Any source code |
| **`test-writer`** | Test files only | Source code (report bugs, don't fix them) |
| **`docs-writer`** | Documentation (`.md` only) | Any source code |
| **`code-reviewer`**, **`pr-writer`** | Nothing — read-only | Everything; they report `file:line` |

**Rules:**
- Work crossing a boundary is two lanes, not one. Finish the owning side first, or assign the rest to the agent that owns it
- Read files from other domains for context, but never modify them
- If you notice an issue outside your scope, flag it — don't fix it
- Shared files (e.g. API types, config) belong to whichever agent owns them. When unclear, ask before editing

---

## COMMENT DISCIPLINE

When writing code, comments must be **sparse and meaningful**:
- Comment the **why**, not the **what** — never restate what the code already says.
- No narrative blocks, no decorative banners, no commented-out code.
- A non-obvious decision, a gotcha, or a constraint is worth a short line. Obvious code is not.
- Prefer clear names over comments. If a comment just translates the next line to English, delete it.

---

## DURABLE CONTEXT — THE WIKI

The wiki is the compiled knowledge base: domain and business knowledge, and the decisions behind it.
Unlike this file and the auto-memory, it does NOT load itself — so reach for it.

**It lives at `$RETINUE_WIKI`, defaulting to `~/work/wiki`.** Everything below, every skill, and
`install.sh` read that one variable; resolve it before touching a path. Referred to as `$WIKI` here.

**Read it when** the task touches a domain the wiki covers, when you are about to make an
architectural or pricing call, or when starting an agent-team run. Entry point is `$WIKI/index.md` —
cheap to read. Take page frontmatter and summaries first, page bodies second, `raw/` last.

**Write to it when** a session produces knowledge that outlives it: a decision and its rationale, a
domain fact learned the hard way, a constraint that will bind future work. Use `/capture`. Do not
wait to be asked — an uncaptured decision gets re-litigated next month at full price.

**Do not put in the wiki:** how I want Claude to work (that's the auto-memory), a specific app's
architecture (that's its own `CLAUDE.md`), or anything derivable from git history.

---

## MODEL TUNING — OPUS 5

Opus 5 self-verifies, delegates, and elaborates more than prior models. These
rules counteract that. They apply everywhere — session and spawned agent alike.

### Output length

- Keep responses focused and brief. Caveats and disclaimers stay short; the
  answer gets the space.
- Written deliverables (RFCs, `.md` docs, reports) match the length the task
  needs. No filler sections, no redundant summaries, no boilerplate.
- Lowering `effort` does NOT shorten output — only instructions do.

### Verification

- Do NOT add a "verify your work" step to prompts or harnesses. Verification is
  already in the main loop; asking for it causes redundant work, not accuracy.
- Do NOT spawn a subagent to review or double-check. Verify inline.
- Exception: the Review teammate in an agent team is a real lane, not scaffolding.

### Subagent delegation

- Delegate only when the payoff clearly exceeds the overhead — each subagent
  re-establishes context, re-explores, and reports back.
- DO delegate: wide multi-file investigation, genuinely independent lanes.
- Do NOT delegate: work finishable in a handful of tool calls, simple searches,
  verification.
- Prefer one subagent over several. Never exceed 20 parallel agents unless
  explicitly asked.
- Brief a subagent precisely the first time. Commit to its result — don't redo
  or re-derive its work.

### Task scope

Deliver what was asked, at the scope intended. Make routine judgment calls;
check in only when readings differ materially. If the ask looks wrong, say so
in one sentence and continue as asked. Finish the whole task — report done only
when it is. If something is blocked, do the rest and state plainly what's missing.

### Effort

| Work | Start at |
|------|----------|
| Agentic coding, refactors, long-horizon | `xhigh` |
| Reviews, audits, architecture | `high` |
| Scoped edits, lookups, mechanical work | `low` / `medium` |

Sweep downward from the start point — `low` and `medium` are strong on Opus 5
and are the main cost lever. At `xhigh`/`max` set `max_tokens` >= 64K.

---

## Project-Level Configuration

Each project should have its own `CLAUDE.md` in the project root with:
- Tech stack and framework details
- Architecture and file structure
- Naming conventions and patterns

A template is available at `~/.claude/templates/project-claude.md`.

---

## The Agents

An agent is spawned with its own context, tools, and model, registered by the
`name`/`description`/`model`/`tools` frontmatter at the top of its file. There are seven, and
between them they cover every lane a run needs:

| Agent | Does | Owns |
|-------|------|------|
| **`pm`** | PRD breakdown, user stories (INVEST), acceptance criteria (Given-When-Then), dependencies, risks | `.md` only |
| **`backend`** | API implementation, DB schema and migrations, architecture, performance, security | Backend paths |
| **`frontend`** | Components, state management, SSR/CSR decisions, accessibility | Frontend paths |
| **`test-writer`** | Test plans and automated tests against the ship criteria; reports bugs | Test files |
| **`code-reviewer`** | Correctness, security, accessibility, visual constants, reuse, style | Nothing — reports |
| **`docs-writer`** | Documentation kept in step with the code | `.md` only |
| **`pr-writer`** | PR title and body from the branch diff | Nothing — drafts |

Fall back to `general-purpose` only for a lane none of them covers.

### Pattern files

Stack-specific code patterns live in `agents/patterns/` and are loaded on demand — `go-patterns.md`
for the data layer, concurrency, and idempotency; `vue-patterns.md` for Vue/Nuxt structure, state,
and SSR/CSR. An agent loads the one matching the repo's stack. Add more as needed
(`react-patterns.md`, `node-patterns.md`).

---

## File Structure

```
~/.claude/
├── CLAUDE.md                        # This file (global guide)
├── agents/                          # every file here registers one spawnable agent
│   ├── pm.md
│   ├── backend.md
│   ├── frontend.md
│   ├── code-reviewer.md
│   ├── test-writer.md
│   ├── docs-writer.md
│   ├── pr-writer.md
│   └── patterns/
│       ├── go-patterns.md           # DB guidelines, Go concurrency & idempotency
│       └── vue-patterns.md          # Vue/Nuxt patterns, layering, SSR/CSR criteria
├── templates/
│   └── project-claude.md            # Per-project CLAUDE.md template

{project}/
└── CLAUDE.md                        # Stack, architecture, conventions
```

---

## Agent Team Coordination (Level 3)

When running an agent team (`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`), the **lead agent** follows these rules. Full reference: `~/.claude/TEAM-PLAYBOOK.md`.

**Choosing the shape first.** Don't spawn a team for independent tasks (use Agent View) or for a single fix (just do it). Teams are for *dependent, multi-file* work only.

**Lead role — oversee, do not code.**
- The lead does NOT write or edit source files — not features, not integration glue, not `main.go`
  wiring, not bug fixes. Every code change is assigned to an agent.
- The lead's job: set the contract, decompose lanes, review diffs, make decisions, and VERIFY
  (run builds/tests/servers, curl endpoints) — keystrokes on source belong to agents.
- If a gap remains after agents finish, spawn/assign an agent to close it rather than editing directly.

**Decomposition.**
- Break the feature into one teammate per independent stream, plus a dedicated Review teammate.
- Spawn the registered agent for the lane — `pm`, `backend`, `frontend`, `test-writer`,
  `docs-writer`, `code-reviewer`, `pr-writer`. They already carry their own scope boundary, so don't
  restate it in the brief. `general-purpose` is the fallback for a lane none of them covers.
- Map every teammate to concrete files/modules. No vague lanes.
- State the contract between streams (API shape, types) up front so teammates don't diverge.

**Dependency discipline.**
- Flag dependencies BEFORE dependent work starts (e.g. tests block on backend routes existing).
- Coordinate through the shared task list; a teammate waiting on a dependency stays queued, not guessing.

**Scope discipline (inherits the scope table above).**
- Each teammate modifies only files in its lane (backend / frontend / tests / docs).
- The Review teammate REPORTS issues with `file:line` — it never fixes them.
- The lead merges a stream only after Review approves it.

**Cost & safety.**
- Teammates run on Opus (`CLAUDE_CODE_SUBAGENT_MODEL="claude-opus-5"`); the lead stays on the session model. Cap every unattended run — Opus lanes are not cheap.
- Autonomous/headless runs must be capped: `--max-budget-usd N`.
- The `permissions` deny list in settings.json is authoritative — never work around `git push --force`, `sudo`, or `publish` denials.
