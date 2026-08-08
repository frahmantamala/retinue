---
name: new-app
description: Scaffold a new project to the house standard — repo CLAUDE.md written first, then Go modular monolith backend and/or feature-based Next.js frontend, README, and git init. Use when starting a new app, service, or side project from scratch, or when the user says "new app", "bootstrap a project", "start a new repo".
---

# New App

Set up a new project so it starts consistent instead of drifting. The repo
`CLAUDE.md` comes first — before any directory, before any code. Everything
after it follows from what that file declares.

## Interview first

Ask in one batch, then proceed. Do not scaffold from a one-liner.

- What is it, in one sentence? Who uses it?
- Backend, frontend, or both?
- Persistence — Postgres? Multi-tenant? If multi-tenant, `tenant_id` + RLS from
  day one, never retrofitted.
- Auth — needed at all, and shared with an existing system?
- Is this a throwaway experiment? If yes, skip to the minimal path below.

## Step 1 — CLAUDE.md, before anything else

Start from `~/.claude/templates/project-claude.md`. Model the depth on the most
thoroughly documented repo you already have.

It must state, concretely enough that an agent can't guess wrong:

- Tech stack and versions
- Architecture — module boundaries and what may import what
- Directory layout
- Naming conventions
- Invariants that must never be violated (tenant isolation, auth boundaries,
  migration rules)

A vague CLAUDE.md is worse than none — it produces confident wrong work.

## Step 2 — Structure

**Backend (Go modular monolith).** If you have a canonical layout RFC, read it
rather than reproducing one from memory. Modules own their own schema and
expose a service interface — no cross-module table access.

**Frontend (Next.js, feature-based).** Feature folders, not layer folders.
Shared design system separate from features. Colocate components, hooks, and
state with the feature that owns them.

Create the tree and the module boundaries. Do **not** write business logic,
handlers, or components — this skill produces a skeleton, not an application.

## Step 3 — Tooling

- **`yarn`, never `npm`.** Generate `yarn.lock`.
- `.gitignore` for the actual stack, including `node_modules/`, build output,
  `.env`.
- `.env.example` with every variable named and no real values.
- Makefile or `package.json` scripts for the standard four: run, test, lint,
  migrate.

## Step 4 — README

Write it now, while the intent is fresh — a README added later never gets
written. Cover: what it does and for whom, stack, how to run it locally, and
architecture in one Mermaid diagram. Keep it under 80 lines.

## Step 5 — Git

`git init`, then one commit with the scaffold. Do **not** create a GitHub remote
or push — that is the user's call, always.

## Minimal path (throwaway experiments)

CLAUDE.md, README, `.gitignore`, `git init`. Nothing else. Most experiments die;
the ones that live get the full treatment when they prove out.

## Rules

- Comments stay sparse and meaningful — why, never what. No banners, no
  narrative blocks, no commented-out code.
- No placeholder files that exist only to make a tree look complete.
- No dependency the interview didn't justify.
- Multi-tenant means RLS in the first migration, not the tenth.

## Finish

Report the tree, the stack chosen, and anything the interview left undecided.
