package watch

import (
	"encoding/json"
	"testing"
	"time"
)

// A lane name is not unique across a run: a lead that respawns "Review" has two
// live agents under one name. The first result must finish the first agent, not
// whichever one registered under that name most recently.
func TestRespawnedNameKeepsAgentsApart(t *testing.T) {
	g := NewGraph("", "")
	g.EnsureLead("lead")

	spawn := func(toolID string) string {
		return `{"type":"assistant","uuid":"` + toolID + `","timestamp":"2026-08-08T16:40:00.000Z","message":{"content":[{"type":"tool_use","id":"` +
			toolID + `","name":"Agent","input":{"name":"Review"}}]}}`
	}
	g.Apply(LeadNodeID, mustDecode(t, spawn("toolu_1")))
	g.Apply(LeadNodeID, mustDecode(t, spawn("toolu_2")))

	g.Register("areview-1111", agentMeta{Name: "Review", AgentType: "general-purpose"})
	g.Register("areview-2222", agentMeta{Name: "Review", AgentType: "general-purpose"})

	g.Apply(LeadNodeID, mustDecode(t, `{"type":"user","uuid":"r","timestamp":"2026-08-08T16:45:00.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1"}]}}`))

	nodes := nodesOf(g)
	if got := nodes["areview-1111"].State; got != StateDone {
		t.Errorf("toolu_1's result belongs to the first Review: state %q", got)
	}
	if got := nodes["areview-2222"].State; got == StateDone {
		t.Error("the second Review was finished by the first one's result")
	}
}

// A teammate transcript can be discovered before its sidecar is written. The
// lead is a guess at that point, and when the real parent arrives the node ends
// up with two spawn edges.
func TestAgentWithoutEvidenceStaysUnlinked(t *testing.T) {
	g := NewGraph("", "")
	g.EnsureLead("lead")

	g.Register("anested-4444", agentMeta{}) // sidecar has not landed yet
	if edges := edgesOf(g); len(edges) != 0 {
		t.Fatalf("nothing supports a parent yet, got %v", keys(edges))
	}

	g.Register("anested-4444", agentMeta{AgentType: "Explore", ParentAgentID: "aowner-5555", SpawnDepth: 1})

	edges := edgesOf(g)
	if _, ok := edges["aowner-5555|anested-4444|spawn"]; !ok {
		t.Errorf("missing the real spawn edge; have %v", keys(edges))
	}
	if len(edges) != 1 {
		t.Errorf("want exactly one parent edge, got %v", keys(edges))
	}
}

// A genuine top-level teammate still belongs to the lead: its sidecar says
// depth 0, which is evidence rather than a guess.
func TestTopLevelTeammateStillFallsBackToLead(t *testing.T) {
	g := NewGraph("", "")
	g.EnsureLead("lead")

	g.Register("aalpha-1111", agentMeta{Name: "alpha", AgentType: "general-purpose", SpawnDepth: 0})

	if _, ok := edgesOf(g)[LeadNodeID+"|aalpha-1111|spawn"]; !ok {
		t.Errorf("depth-0 teammate should hang off the lead; have %v", keys(edgesOf(g)))
	}
}

// The parent node is minted while resolving the edge, so a live client is sent
// an edge naming a node it has never seen and drops it until a reload.
func TestMintedParentNodeIsBroadcast(t *testing.T) {
	g := NewGraph("", "")
	g.EnsureLead("lead")

	out := g.Register("anested-4444", agentMeta{AgentType: "Explore", ParentAgentID: "aowner-5555", SpawnDepth: 1})

	var sawNode, sawEdge bool
	for _, d := range out {
		switch v := d.(type) {
		case Node:
			if v.ID == "aowner-5555" {
				sawNode = true
			}
		case Edge:
			if v.From == "aowner-5555" && v.To == "anested-4444" {
				sawEdge = true
				if !sawNode {
					t.Error("edge broadcast before the node it names")
				}
			}
		}
	}
	if !sawEdge {
		t.Fatal("no spawn edge in the deltas")
	}
	if !sawNode {
		t.Error("parent node was created but never broadcast")
	}
}

// The window is anchored to the last event, so a quiet run's cutoff stops
// advancing and the rate it had when it went quiet is reported forever.
func TestBurnRateDecaysWhileIdle(t *testing.T) {
	g := NewGraph("", "")
	clock := time.Date(2026, 8, 8, 16, 35, 0, 0, time.UTC)
	g.now = func() time.Time { return clock }
	g.EnsureLead("lead")

	turn := func(stamp string) string {
		return `{"type":"assistant","uuid":"x","timestamp":"` + stamp +
			`","message":{"model":"claude-opus-5","usage":{"input_tokens":1000,"output_tokens":500}}}`
	}
	g.Apply(LeadNodeID, mustDecode(t, turn("2026-08-08T16:30:00.000Z")))
	g.Apply(LeadNodeID, mustDecode(t, turn("2026-08-08T16:35:00.000Z")))

	live := g.RunSummary().BurnPerHour
	if live <= 0 {
		t.Fatalf("two turns five minutes apart is a rate, got %v", live)
	}

	clock = clock.Add(5 * time.Minute) // still in-window, but twice the span
	decayed := g.RunSummary().BurnPerHour
	if decayed <= 0 || decayed >= live {
		t.Errorf("idle run should decay: %v then %v", live, decayed)
	}

	clock = clock.Add(time.Hour)
	if idle := g.RunSummary().BurnPerHour; idle != 0 {
		t.Errorf("every sample has aged out, want 0, got %v", idle)
	}
	if len(g.spend) != 0 {
		t.Errorf("aged-out samples should be trimmed, %d kept", len(g.spend))
	}
}

// Seq is the activity feed's identity. Lane 3 dedupes on it, so a line must
// carry the same Seq in its live delta and in every later snapshot replay.
func TestActivitySeqIsStableAcrossSnapshot(t *testing.T) {
	g := NewGraph("", "")
	g.EnsureLead("lead")

	var live []Activity
	for _, d := range g.Apply(LeadNodeID, mustDecode(t, `{"type":"assistant","uuid":"a","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"description":"one"}},{"type":"tool_use","id":"t2","name":"Bash","input":{"description":"two"}}]}}`)) {
		if a, ok := d.(Activity); ok {
			live = append(live, a)
		}
	}
	if len(live) != 2 {
		t.Fatalf("want 2 activity deltas, got %d", len(live))
	}
	if live[0].Seq != 1 || live[1].Seq != 2 {
		t.Errorf("seq starts at 1 and increases: got %d then %d", live[0].Seq, live[1].Seq)
	}

	replay := activityOf(g)
	if len(replay) != len(live) {
		t.Fatalf("snapshot replayed %d lines, %d were emitted", len(replay), len(live))
	}
	for i, a := range replay {
		if a.Seq != live[i].Seq {
			t.Errorf("line %d: snapshot seq %d, delta seq %d", i, a.Seq, live[i].Seq)
		}
	}

	// The page reads the wire field, not the Go one.
	b, err := json.Marshal(live[0])
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	if got, ok := wire["seq"]; !ok || got != float64(1) {
		t.Errorf(`want "seq":1 on the wire, got %v`, wire)
	}
}
