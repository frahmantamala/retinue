# `retinue watch` — parallel lane briefs (team contract)

Build a live monitor that renders agent activity as a graph: neurons firing, synapses carrying
messages. Four lanes, one agent each, all on **Opus**. Read `AGENTS.md` and `TEAM-PLAYBOOK.md`
("The retinue contract") first — every rule there applies, including the escalation table.

Written 2026-08-08, before the run, so a cold session can execute it without prior context.

## Why this task

It is self-bootstrapping. The crew's own work produces `isSidechain: true` events in the session
JSONL — which is exactly the data the monitor parses. The run generates its own test fixture.

## Verified data sources

Confirmed present on this machine; do not assume beyond this list without checking.

| Path | Contents |
|---|---|
| `~/.claude/projects/<slug>/<session-id>.jsonl` | append-only event log, one JSON object per line |
| `~/.claude/teams/session-<id>/config.json` | `leadAgentId`, `leadSessionId`, `members[]` (each with `agentId`, `name`, `agentType`, `cwd`) |

Fields confirmed on JSONL events: `uuid`, `parentUuid`, `timestamp`, `type`
(`assistant`/`user`/`system`), `isSidechain`, `sessionId`, `message.content[]` (with `tool_use` /
`tool_result` blocks), `sourceToolUseID`, `sourceToolAssistantUUID`, `durationMs`, `message.usage`.

**Known gap:** this machine has **zero** `isSidechain: true` events on record — that path has never
run here. Lane A must parse defensively and treat the sidechain shape as unverified until this run
produces real ones. Log and skip unrecognised events; never crash on them.

## The contract between lanes (pinned — A and B must agree)

Lane A emits Server-Sent Events; Lane B renders them. Neither may change this shape unilaterally —
if it needs to change, the lead decides and re-briefs both.

```json
{ "kind": "node",  "id": "agent-uuid", "role": "lead|crew", "label": "backend",
  "state": "thinking|tool|waiting|done", "tool": "Bash", "tokens": 1234, "ts": 1786200000000 }
{ "kind": "edge",  "from": "lead-id", "to": "agent-uuid", "rel": "spawn|report", "ts": 1786200000000 }
{ "kind": "pulse", "from": "agent-uuid", "to": "lead-id", "ts": 1786200000000 }
```

`spawn` edges come from a `Task` tool_use; `report` from its matching `tool_result`, correlated on
`sourceToolUseID`. A `pulse` is a single message crossing an existing edge — the firing signal.

## Lanes

### Lane A — watcher core (Go)
Tail the JSONL, build the graph model, serve SSE plus the static page.
- `retinue watch [--session <id>] [--port 7777]`; default to the most recently modified session.
- Tail from the current end of file, poll-based; handle the file growing and rotating.
- Correlate spawn/report via `sourceToolUseID`. Derive node state from the newest event.
- **Files:** `go.mod`, `cmd/watch/**`, `internal/watch/**` (except `web/`)

### Lane B — graph UI
One self-contained HTML file, no CDN, no build step. Force-directed canvas.
- Match the existing visual language: dark ground, marigold accent for the lead, mono labels,
  white crew nodes, dashed idle. Node radius by cumulative tokens; pulse a dot along the edge on `pulse`.
- Degrade honestly: show "waiting for events" rather than a fake graph when the stream is empty.
- **Files:** `internal/watch/web/index.html` only

### Lane C — tests
- Parser tests over a fixture JSONL committed to `internal/watch/testdata/`.
- Cover: malformed line, unknown event type, spawn without a matching report, out-of-order events.
- **Files:** `internal/watch/*_test.go`, `internal/watch/testdata/**`

### Lane D — review
Security, correctness, and adherence to the contract above. Report `file:line`. **Never fix.**

## Definition of done

Per lane: `go build ./...` and `go vet ./...` pass, own tests pass, only that lane's files touched.
For the run: `retinue watch` serves on localhost and renders a graph from the committed fixture.

## Dependencies

Lane B blocks on the SSE shape above (already pinned — it may start immediately).
Lane C blocks on Lane A's parser signatures. Lane D blocks on all.

## Out of scope

Persistence, auth, remote access, historical replay, multi-session views. Localhost only, read-only,
never writes to `~/.claude`.
