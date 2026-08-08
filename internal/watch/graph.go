package watch

import "sync"

// Wire format. Pinned by _design/MONITOR-LANES.md — the page renders exactly
// these three shapes and nothing else.
type (
	Node struct {
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Role   string `json:"role"`
		Label  string `json:"label"`
		State  string `json:"state"`
		Tool   string `json:"tool,omitempty"`
		Tokens int    `json:"tokens"`
		TS     int64  `json:"ts"`
	}
	Edge struct {
		Kind string `json:"kind"`
		From string `json:"from"`
		To   string `json:"to"`
		Rel  string `json:"rel"`
		TS   int64  `json:"ts"`
	}
	Pulse struct {
		Kind string `json:"kind"`
		From string `json:"from"`
		To   string `json:"to"`
		TS   int64  `json:"ts"`
	}

	// Activity is one readable line of what an agent just did. Additive to the
	// pinned contract: the three shapes above are unchanged.
	Activity struct {
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Tool   string `json:"tool"`
		Detail string `json:"detail"`
		TS     int64  `json:"ts"`
	}

	// Knowledge records a wiki page reaching an agent — read directly, or cited
	// in the brief that agent was given.
	Knowledge struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
		Page string `json:"page"`
		Via  string `json:"via"`
		TS   int64  `json:"ts"`
	}
)

// activityLimit bounds what a client gets on connect. A long run produces
// thousands of lines and the panel only ever shows the recent tail.
const activityLimit = 200

const (
	RoleLead = "lead"
	RoleCrew = "crew"

	StateThinking = "thinking"
	StateTool     = "tool"
	StateWaiting  = "waiting"
	StateDone     = "done"
)

// spawnTools are the tool names that create a child agent.
var spawnTools = map[string]bool{"Agent": true, "Task": true}

type spawnRec struct {
	from   string
	toolID string
	name   string
	ts     int64
}

// Graph is the live model of one run. Every mutation returns the deltas to
// broadcast, so the caller never diffs state itself.
//
// Both halves of a spawn (the parent's tool_use, the child's transcript) and
// both halves of a report (the child's registration, the parent's tool_result)
// can arrive in either order — files are tailed independently and a teammate's
// first line often lands before the lead's. Every correlation below is
// therefore stored from both sides and resolved by whichever arrives second.
type Graph struct {
	mu sync.Mutex

	wikiRoot string
	home     string

	nodes     map[string]*Node
	nodeOrder []string
	edges     map[string]bool
	edgeOrder []Edge
	parent    map[string]string

	spawnByTool map[string]spawnRec
	spawnByName map[string]spawnRec
	childByTool map[string]string
	childByName map[string]string
	reportWait  map[string]int64

	activity  []Activity
	knowledge []Knowledge
	knowSeen  map[string]bool
}

func NewGraph(wikiRoot, home string) *Graph {
	return &Graph{
		wikiRoot:    wikiRoot,
		home:        home,
		nodes:       map[string]*Node{},
		edges:       map[string]bool{},
		parent:      map[string]string{},
		spawnByTool: map[string]spawnRec{},
		spawnByName: map[string]spawnRec{},
		childByTool: map[string]string{},
		childByName: map[string]string{},
		reportWait:  map[string]int64{},
		knowSeen:    map[string]bool{},
	}
}

// EnsureLead creates the root node. Safe to call repeatedly.
func (g *Graph) EnsureLead(label string) []any {
	g.mu.Lock()
	defer g.mu.Unlock()
	n, created := g.node(LeadNodeID, RoleLead, label)
	if !created {
		return nil
	}
	return []any{*n}
}

// Register announces a crew agent discovered on disk and wires it to whichever
// parent the evidence supports.
func (g *Graph) Register(agentID string, m agentMeta) []any {
	g.mu.Lock()
	defer g.mu.Unlock()

	var out []any
	n, changed := g.node(agentID, RoleCrew, m.label(agentID))
	// The sidecar can land after the transcript, so a node may be sitting on a
	// raw agentId until the real name shows up.
	if lbl := m.label(agentID); lbl != agentID && lbl != n.Label {
		n.Label, changed = lbl, true
	}
	if changed {
		out = append(out, *n)
	}

	if m.ToolUseID != "" {
		g.childByTool[m.ToolUseID] = agentID
	}
	if m.Name != "" {
		g.childByName[m.Name] = agentID
	}

	rec, spawned := g.spawnFor(m)
	if _, linked := g.parent[agentID]; !linked {
		from, ts := g.parentFor(agentID, m, rec, spawned)
		out = append(out, g.link(from, agentID, ts)...)
	}

	// A teammate's sidecar carries no toolUseId, so the parked report can only
	// be found through the spawn record that named it.
	toolID := m.ToolUseID
	if spawned {
		toolID = rec.toolID
	}
	if ts, ok := g.reportWait[toolID]; ok && toolID != "" {
		delete(g.reportWait, toolID)
		out = append(out, g.finish(agentID, ts)...)
	}
	return out
}

// spawnFor finds the tool_use that created this agent, by id when the sidecar
// records one and by name otherwise.
func (g *Graph) spawnFor(m agentMeta) (spawnRec, bool) {
	if m.ToolUseID != "" {
		if rec, ok := g.spawnByTool[m.ToolUseID]; ok {
			return rec, true
		}
	}
	if m.Name != "" {
		if rec, ok := g.spawnByName[m.Name]; ok {
			return rec, true
		}
	}
	return spawnRec{}, false
}

// parentFor prefers hard evidence (the spawning tool_use, or the sidecar's
// parentAgentId) and falls back to the lead, which is where a top-level
// teammate always belongs.
func (g *Graph) parentFor(agentID string, m agentMeta, rec spawnRec, spawned bool) (string, int64) {
	if spawned {
		return rec.from, rec.ts
	}
	if m.ParentAgentID != "" && m.ParentAgentID != agentID {
		g.node(m.ParentAgentID, RoleCrew, m.ParentAgentID)
		return m.ParentAgentID, 0
	}
	return LeadNodeID, 0
}

// Apply folds one event into the node that produced it.
func (g *Graph) Apply(nodeID string, e event) []any {
	if e.Type != "assistant" && e.Type != "user" {
		return nil // system, attachment, summary — not activity
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	role, label := RoleCrew, nodeID
	if nodeID == LeadNodeID {
		role, label = RoleLead, LeadNodeID
	}
	n, _ := g.node(nodeID, role, label)

	ts := e.millis()
	msg := e.decodeMessage()
	n.Tokens += msg.Usage.billed()
	if ts > n.TS {
		n.TS = ts
	}

	var extra []any
	state, tool := n.State, n.Tool
	switch e.Type {
	case "assistant":
		state, tool = StateThinking, ""
		for _, b := range msg.Content {
			switch b.Type {
			case "tool_use":
				state, tool = StateTool, b.Name
				in := b.input()
				extra = append(extra, g.record(nodeID, b.Name, toolDetail(b.Name, in), ts))
				extra = append(extra, g.recordKnowledge(nodeID, b.Name, in, ts)...)
				if spawnTools[b.Name] {
					extra = append(extra, g.recordSpawn(nodeID, b, ts)...)
				}
			case "text":
				if d := shorten(b.Text); d != "" {
					extra = append(extra, g.record(nodeID, "says", d, ts))
				}
			}
		}
	case "user":
		for _, b := range msg.Content {
			if b.Type != "tool_result" || b.ToolUseID == "" {
				continue
			}
			state, tool = StateWaiting, ""
			extra = append(extra, g.recordReport(b.ToolUseID, ts)...)
		}
	}

	// A finished agent stays finished; a late event must not resurrect it.
	if n.State != StateDone {
		n.State, n.Tool = state, tool
	}

	// Node first: an activity line names an agent, and the page resolves that
	// name from the node it has already seen.
	out := []any{*n}
	out = append(out, extra...)
	if p, ok := g.parent[nodeID]; ok {
		out = append(out, Pulse{Kind: "pulse", From: nodeID, To: p, TS: ts})
	}
	return out
}

func (g *Graph) record(id, tool, detail string, ts int64) any {
	a := Activity{Kind: "activity", ID: id, Tool: tool, Detail: detail, TS: ts}
	g.activity = append(g.activity, a)
	if len(g.activity) > activityLimit {
		g.activity = g.activity[len(g.activity)-activityLimit:]
	}
	return a
}

// recordKnowledge fires once per agent per page. A lane that greps the wiki
// forty times is one fact about that lane, not forty.
func (g *Graph) recordKnowledge(id, tool string, in toolInput, ts int64) []any {
	var out []any
	for _, hit := range wikiPages(tool, in, g.wikiRoot, g.home) {
		key := id + "|" + hit.Page
		if g.knowSeen[key] {
			continue
		}
		g.knowSeen[key] = true
		k := Knowledge{Kind: "knowledge", ID: id, Page: hit.Page, Via: hit.Via, TS: ts}
		g.knowledge = append(g.knowledge, k)
		out = append(out, k)
	}
	return out
}

func (g *Graph) recordSpawn(from string, b contentBlock, ts int64) []any {
	in := b.input()
	rec := spawnRec{from: from, toolID: b.ID, name: in.Name, ts: ts}
	if b.ID != "" {
		g.spawnByTool[b.ID] = rec
	}
	if in.Name != "" {
		g.spawnByName[in.Name] = rec
	}

	child, ok := g.childByTool[b.ID]
	if !ok && in.Name != "" {
		child, ok = g.childByName[in.Name]
	}
	if !ok {
		return nil // the child's transcript has not appeared yet
	}
	return g.link(from, child, ts)
}

func (g *Graph) recordReport(toolUseID string, ts int64) []any {
	child, ok := g.childFor(toolUseID)
	if !ok {
		// Only a spawn's result matters, but we cannot tell yet which tool this
		// was — park it and let Register collect it.
		if _, spawned := g.spawnByTool[toolUseID]; spawned {
			g.reportWait[toolUseID] = ts
		}
		return nil
	}
	return g.finish(child, ts)
}

func (g *Graph) childFor(toolUseID string) (string, bool) {
	if id, ok := g.childByTool[toolUseID]; ok {
		return id, true
	}
	if rec, ok := g.spawnByTool[toolUseID]; ok && rec.name != "" {
		if id, ok := g.childByName[rec.name]; ok {
			return id, true
		}
	}
	return "", false
}

// finish marks a child done and draws the report edge back to its parent.
func (g *Graph) finish(child string, ts int64) []any {
	n, ok := g.nodes[child]
	if !ok {
		return nil
	}
	var out []any
	if n.State != StateDone {
		n.State, n.Tool = StateDone, ""
		if ts > n.TS {
			n.TS = ts
		}
		out = append(out, *n)
	}
	if p, ok := g.parent[child]; ok {
		out = append(out, g.edge(child, p, "report", ts)...)
		out = append(out, Pulse{Kind: "pulse", From: child, To: p, TS: ts})
	}
	return out
}

func (g *Graph) link(from, to string, ts int64) []any {
	if from == "" || from == to {
		return nil
	}
	g.parent[to] = from
	out := g.edge(from, to, "spawn", ts)
	if len(out) > 0 {
		out = append(out, Pulse{Kind: "pulse", From: from, To: to, TS: ts})
	}
	return out
}

func (g *Graph) edge(from, to, rel string, ts int64) []any {
	key := from + "|" + to + "|" + rel
	if g.edges[key] {
		return nil
	}
	g.edges[key] = true
	e := Edge{Kind: "edge", From: from, To: to, Rel: rel, TS: ts}
	g.edgeOrder = append(g.edgeOrder, e)
	return []any{e}
}

func (g *Graph) node(id, role, label string) (*Node, bool) {
	if n, ok := g.nodes[id]; ok {
		return n, false
	}
	n := &Node{Kind: "node", ID: id, Role: role, Label: label, State: StateWaiting}
	g.nodes[id] = n
	g.nodeOrder = append(g.nodeOrder, id)
	return n, true
}

// Snapshot is the full graph, nodes before edges, for a client that just
// connected mid-run.
func (g *Graph) Snapshot() []any {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]any, 0, len(g.nodeOrder)+len(g.edgeOrder)+len(g.knowledge)+len(g.activity))
	for _, id := range g.nodeOrder {
		out = append(out, *g.nodes[id])
	}
	for _, e := range g.edgeOrder {
		out = append(out, e)
	}
	for _, k := range g.knowledge {
		out = append(out, k)
	}
	for _, a := range g.activity {
		out = append(out, a)
	}
	return out
}
