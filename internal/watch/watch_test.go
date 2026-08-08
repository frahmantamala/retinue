package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixtureSession() Session {
	root := filepath.Join("testdata", "session")
	return Session{
		Repo:        "/tmp/demo",
		ID:          "lead",
		LeadPath:    filepath.Join(root, "lead.jsonl"),
		SubagentDir: filepath.Join(root, "lead", "subagents"),
		LeadLabel:   "lead",
	}
}

func runFixture(t *testing.T, passes int) (*Graph, *Watcher) {
	t.Helper()
	g := NewGraph()
	w := NewWatcher(fixtureSession(), g, NewHub(), time.Millisecond, nil)
	w.start()
	for range passes {
		w.pass()
	}
	return g, w
}

func nodesOf(g *Graph) map[string]Node {
	out := map[string]Node{}
	for _, m := range g.Snapshot() {
		if n, ok := m.(Node); ok {
			out[n.ID] = n
		}
	}
	return out
}

func edgesOf(g *Graph) map[string]Edge {
	out := map[string]Edge{}
	for _, m := range g.Snapshot() {
		if e, ok := m.(Edge); ok {
			out[e.From+"|"+e.To+"|"+e.Rel] = e
		}
	}
	return out
}

func TestFixtureBuildsGraph(t *testing.T) {
	g, _ := runFixture(t, 1)
	nodes, edges := nodesOf(g), edgesOf(g)

	for _, id := range []string{LeadNodeID, "aalpha-1111", "abeta-2222", "agamma-3333"} {
		if _, ok := nodes[id]; !ok {
			t.Fatalf("missing node %q; got %v", id, nodes)
		}
	}
	if len(nodes) != 4 {
		t.Fatalf("want 4 nodes, got %d: %v", len(nodes), nodes)
	}

	if got := nodes["aalpha-1111"].Label; got != "alpha" {
		t.Errorf("label from sidecar: want alpha, got %q", got)
	}
	if got := nodes["agamma-3333"].Label; got != "Explore" {
		t.Errorf("nested label falls back to agentType: got %q", got)
	}

	want := []string{
		LeadNodeID + "|aalpha-1111|spawn",
		LeadNodeID + "|abeta-2222|spawn",
		"abeta-2222|agamma-3333|spawn",
		"aalpha-1111|" + LeadNodeID + "|report",
	}
	for _, k := range want {
		if _, ok := edges[k]; !ok {
			t.Errorf("missing edge %q; have %v", k, keys(edges))
		}
	}
}

// A teammate whose result never came back must not be drawn as finished — the
// difference between "still working" and "done" is the whole point of watching.
func TestSpawnWithoutReportStaysUnfinished(t *testing.T) {
	g, _ := runFixture(t, 1)
	nodes, edges := nodesOf(g), edgesOf(g)

	if got := nodes["abeta-2222"].State; got != StateTool {
		t.Errorf("beta state: want %q, got %q", StateTool, got)
	}
	if got := nodes["abeta-2222"].Tool; got != "Edit" {
		t.Errorf("beta tool: want Edit, got %q", got)
	}
	if _, ok := edges["abeta-2222|"+LeadNodeID+"|report"]; ok {
		t.Error("beta has no tool_result in the fixture but got a report edge")
	}
	if got := nodes["aalpha-1111"].State; got != StateDone {
		t.Errorf("alpha state: want %q, got %q", StateDone, got)
	}
}

// The malformed line sits in the middle of the fixture: everything after it
// must still be parsed, or one bad write blinds the monitor for the rest of a run.
func TestMalformedLineSkippedAndParsingContinues(t *testing.T) {
	g, w := runFixture(t, 1)

	if w.Skipped() != 1 {
		t.Fatalf("want 1 skipped line, got %d", w.Skipped())
	}
	nodes := nodesOf(g)
	if got := nodes[LeadNodeID].State; got != StateWaiting {
		t.Errorf("lead state after its last tool_result: want %q, got %q", StateWaiting, got)
	}
	// toolu_A's result is the last line of the file, after the bad one.
	if got := nodes["aalpha-1111"].State; got != StateDone {
		t.Error("a line after the malformed one was not processed")
	}
}

// attachment and system lines carry no activity; counting them would inflate
// state churn and, worse, create phantom nodes.
func TestUnknownEventTypesIgnored(t *testing.T) {
	g := NewGraph()
	g.EnsureLead("lead")
	for _, line := range []string{
		`{"type":"attachment","uuid":"x","timestamp":"2026-08-08T16:40:04.000Z"}`,
		`{"type":"system","uuid":"y","subtype":"hook_result"}`,
		`{"type":"summary","uuid":"z"}`,
	} {
		e, ok := decodeEvent([]byte(line))
		if !ok {
			t.Fatalf("fixture line should decode: %s", line)
		}
		if out := g.Apply(LeadNodeID, e); out != nil {
			t.Errorf("unknown type %s produced deltas: %v", line, out)
		}
	}
	if n := nodesOf(g)[LeadNodeID]; n.State != StateWaiting || n.Tokens != 0 {
		t.Errorf("lead mutated by unknown events: %+v", n)
	}
}

// Files are tailed independently, so a completion can be read before the
// teammate's own transcript is discovered.
func TestReportBeforeRegister(t *testing.T) {
	g := NewGraph()
	g.EnsureLead("lead")

	spawn := mustDecode(t, `{"type":"assistant","uuid":"a","timestamp":"2026-08-08T16:40:00.000Z","message":{"content":[{"type":"tool_use","id":"toolu_X","name":"Agent","input":{"name":"delta"}}]}}`)
	report := mustDecode(t, `{"type":"user","uuid":"b","timestamp":"2026-08-08T16:41:00.000Z","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_X"}]}}`)

	g.Apply(LeadNodeID, spawn)
	g.Apply(LeadNodeID, report) // child unknown at this point
	if _, ok := nodesOf(g)["adelta-9999"]; ok {
		t.Fatal("child node should not exist before registration")
	}

	g.Register("adelta-9999", agentMeta{Name: "delta", AgentType: "general-purpose"})

	nodes, edges := nodesOf(g), edgesOf(g)
	if got := nodes["adelta-9999"].State; got != StateDone {
		t.Errorf("parked report not applied on registration: state %q", got)
	}
	if _, ok := edges[LeadNodeID+"|adelta-9999|spawn"]; !ok {
		t.Errorf("missing spawn edge; have %v", keys(edges))
	}
	if _, ok := edges["adelta-9999|"+LeadNodeID+"|report"]; !ok {
		t.Errorf("missing report edge; have %v", keys(edges))
	}
}

// A late line from a finished agent must not flip it back to running.
func TestDoneIsSticky(t *testing.T) {
	g := NewGraph()
	g.EnsureLead("lead")
	g.Apply(LeadNodeID, mustDecode(t, `{"type":"assistant","uuid":"a","message":{"content":[{"type":"tool_use","id":"toolu_X","name":"Agent","input":{"name":"delta"}}]}}`))
	g.Register("adelta-9999", agentMeta{Name: "delta"})
	g.Apply(LeadNodeID, mustDecode(t, `{"type":"user","uuid":"b","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_X"}]}}`))

	g.Apply("adelta-9999", mustDecode(t, `{"type":"assistant","uuid":"c","message":{"content":[{"type":"tool_use","id":"t2","name":"Bash"}],"usage":{"output_tokens":10}}}`))

	n := nodesOf(g)["adelta-9999"]
	if n.State != StateDone {
		t.Errorf("late event resurrected a finished agent: %q", n.State)
	}
	if n.Tokens != 10 {
		t.Errorf("tokens should still accumulate after done: %d", n.Tokens)
	}
}

// cache_read repeats the same prefix every turn; including it makes the radius
// meaningless within a few minutes.
func TestTokensExcludeCacheReads(t *testing.T) {
	g, _ := runFixture(t, 1)
	if got := nodesOf(g)[LeadNodeID].Tokens; got != 1415 {
		t.Errorf("lead tokens: want 1415 (cache_read excluded), got %d", got)
	}
}

// A second cycle over unchanged files must be a no-op; offsets advance and
// edges dedupe, or a long run accumulates duplicates forever.
func TestRepeatedPassesAreIdempotent(t *testing.T) {
	once, _ := runFixture(t, 1)
	twice, w := runFixture(t, 3)

	if len(nodesOf(once)) != len(nodesOf(twice)) || len(edgesOf(once)) != len(edgesOf(twice)) {
		t.Fatalf("graph grew across passes: %d/%d then %d/%d",
			len(nodesOf(once)), len(edgesOf(once)), len(nodesOf(twice)), len(edgesOf(twice)))
	}
	if w.Skipped() != 1 {
		t.Errorf("malformed line re-counted across passes: %d", w.Skipped())
	}
}

func TestDiscoverPicksNewestSession(t *testing.T) {
	home := t.TempDir()
	repo := "/Users/x/work/demo"
	dir := ProjectDirFor(home, repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	older := filepath.Join(dir, "1111.jsonl")
	newer := filepath.Join(dir, "2222.jsonl")
	for _, p := range []string{older, newer} {
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(older, past, past); err != nil {
		t.Fatal(err)
	}

	s, err := Discover(home, repo, "")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "2222" {
		t.Errorf("want newest session 2222, got %q", s.ID)
	}
	if s.SubagentDir != filepath.Join(dir, "2222", "subagents") {
		t.Errorf("unexpected subagent dir %q", s.SubagentDir)
	}

	if _, err := Discover(home, repo, "nope"); err == nil {
		t.Error("want an error for an unknown session id")
	}
	if _, err := Discover(home, "/Users/x/work/absent", ""); err == nil {
		t.Error("want an error for a repo with no transcripts")
	}
}

func mustDecode(t *testing.T, line string) event {
	t.Helper()
	e, ok := decodeEvent([]byte(line))
	if !ok {
		t.Fatalf("could not decode: %s", line)
	}
	return e
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
