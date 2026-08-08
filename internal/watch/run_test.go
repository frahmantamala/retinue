package watch

import (
	"math"
	"testing"
)

func TestRunSummaryTracksTheSharedTaskList(t *testing.T) {
	g, _ := runFixture(t, 1)
	r := g.RunSummary()

	if r.TasksTotal != 2 {
		t.Errorf("want 2 tasks created, got %d", r.TasksTotal)
	}
	if r.TasksDone != 1 {
		t.Errorf("want 1 completed, got %d", r.TasksDone)
	}
	if r.TasksLive != 1 {
		t.Errorf("want 1 in progress, got %d", r.TasksLive)
	}
	if r.StartedTS == 0 || r.LatestTS <= r.StartedTS {
		t.Errorf("want a positive elapsed span, got %d..%d", r.StartedTS, r.LatestTS)
	}
}

// An update carrying only addBlockedBy has no status and must not disturb the
// task's real state.
func TestStatuslessUpdateIsIgnored(t *testing.T) {
	g, _ := runFixture(t, 1)
	if r := g.RunSummary(); r.TasksLive != 1 || r.TasksDone != 1 {
		t.Errorf("blocked-by update changed task state: %+v", r)
	}
}

// Transcripts are tailed independently, so a later status can be read first.
func TestTaskStatusNewestWinsByTimestamp(t *testing.T) {
	g := NewGraph("", "")
	g.EnsureLead("lead")
	g.Apply(LeadNodeID, mustDecode(t, `{"type":"assistant","uuid":"a","timestamp":"2026-08-08T16:00:00.000Z","message":{"content":[{"type":"tool_use","id":"t0","name":"TaskCreate","input":{"subject":"one"}}]}}`))

	// The completion is applied first, the earlier in_progress second.
	g.Apply(LeadNodeID, mustDecode(t, `{"type":"assistant","uuid":"b","timestamp":"2026-08-08T17:00:00.000Z","message":{"content":[{"type":"tool_use","id":"t1","name":"TaskUpdate","input":{"taskId":"1","status":"completed"}}]}}`))
	g.Apply(LeadNodeID, mustDecode(t, `{"type":"assistant","uuid":"c","timestamp":"2026-08-08T16:30:00.000Z","message":{"content":[{"type":"tool_use","id":"t2","name":"TaskUpdate","input":{"taskId":"1","status":"in_progress"}}]}}`))

	if r := g.RunSummary(); r.TasksDone != 1 || r.TasksLive != 0 {
		t.Errorf("stale update overwrote the newer status: %+v", r)
	}
}

// A lifetime average is meaningless once a run has idled — only the trailing
// window counts toward the rate.
func TestBurnRateUsesTrailingWindowOnly(t *testing.T) {
	g := NewGraph("", "")
	g.EnsureLead("lead")
	turn := func(stamp string) string {
		return `{"type":"assistant","uuid":"x","timestamp":"` + stamp +
			`","message":{"model":"claude-opus-5","usage":{"input_tokens":1000,"output_tokens":500}}}`
	}
	// $0.0175 per turn. The first is 35 minutes before the last — far outside
	// the 10-minute window — and must not count.
	g.Apply(LeadNodeID, mustDecode(t, turn("2026-08-08T16:00:00.000Z")))
	g.Apply(LeadNodeID, mustDecode(t, turn("2026-08-08T16:30:00.000Z")))
	g.Apply(LeadNodeID, mustDecode(t, turn("2026-08-08T16:35:00.000Z")))

	r := g.RunSummary()
	if math.Abs(r.Spent-0.0525) > 1e-9 {
		t.Errorf("spend should count every turn: want 0.0525, got %v", r.Spent)
	}
	// Two turns in-window, $0.035 across 5 minutes = $0.42/hr.
	if math.Abs(r.BurnPerHour-0.42) > 1e-9 {
		t.Errorf("want 0.42/hr from the trailing window, got %v", r.BurnPerHour)
	}
	if len(g.spend) != 2 {
		t.Errorf("out-of-window samples should be trimmed, %d kept", len(g.spend))
	}
}

// One priced turn is not a rate — reporting one would invent a number.
func TestBurnRateNeedsTwoSamples(t *testing.T) {
	g := NewGraph("", "")
	g.EnsureLead("lead")
	g.Apply(LeadNodeID, mustDecode(t, `{"type":"assistant","uuid":"a","timestamp":"2026-08-08T16:00:00.000Z","message":{"model":"claude-opus-5","usage":{"input_tokens":1000,"output_tokens":500}}}`))

	if r := g.RunSummary(); r.BurnPerHour != 0 {
		t.Errorf("a single sample is not a rate: got %v", r.BurnPerHour)
	}
}

func TestBudgetRidesOnTheSummary(t *testing.T) {
	g := NewGraph("", "")
	g.SetBudget(250)
	if got := g.RunSummary().Budget; got != 250 {
		t.Errorf("want budget 250, got %v", got)
	}
}

// A client connecting mid-run needs the rollup in its snapshot, not only in
// the deltas it happens to catch afterwards.
func TestSnapshotCarriesTheRunSummary(t *testing.T) {
	g, _ := runFixture(t, 1)
	snap := g.Snapshot()

	last, ok := snap[len(snap)-1].(Run)
	if !ok {
		t.Fatalf("want a Run at the end of the snapshot, got %T", snap[len(snap)-1])
	}
	if last.TasksTotal != 2 {
		t.Errorf("snapshot summary is stale: %+v", last)
	}
}
