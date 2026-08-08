---
name: review-diff
description: Review the current branch's diff for bugs, security issues, and style by delegating to the code-reviewer agent. Use when the user wants a focused pre-PR review of local changes.
---

# Review Diff

Run a focused review of the current changes and report findings. Read-only — never edits code.

## Steps

1. **Establish scope.** Prefer, in order: `git diff --staged`, then `git diff` (unstaged), then `git diff <base>...HEAD` if the user names a base branch. If not a git repo, ask which files to review.
2. **Delegate** to the `code-reviewer` agent (`~/.claude/agents/code-reviewer.md`) with the diff scope. It runs on Sonnet and returns findings grouped by severity.
3. **Relay findings** grouped as 🔴 Blocking / 🟡 Should-fix / 🟢 Nit, each with `file:line` and a concrete fix. End with a verdict: APPROVE / APPROVE WITH NITS / REQUEST CHANGES.
4. **Do not fix** anything unless the user explicitly asks afterward — this skill reports only.

## Notes

- For a deeper multi-agent cloud review of a whole branch or PR, suggest `/code-review ultra` instead (user-triggered, billed).
- Keep it honest: if the diff is clean, say so — don't manufacture findings.
