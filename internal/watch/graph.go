package watch

import (
	"sync"
	"time"
)

// Wire format. Pinned by _design/MONITOR-LANES.md — the page renders exactly
// these three shapes and nothing else.
type (
	Node struct {
		Kind   string  `json:"kind"`
		ID     string  `json:"id"`
		Role   string  `json:"role"`
		Label  string  `json:"label"`
		State  string  `json:"state"`
		Tool   string  `json:"tool,omitempty"`
		Tokens int     `json:"tokens"`
		Cost   float64 `json:"cost"`
		Model  string  `json:"model,omitempty"`
		// Unpriced counts tokens from a model this build has no rate for, so
		// the page can say the total is partial instead of quietly low.
		Unpriced int   `json:"unpriced"`
		TS       int64 `json:"ts"`
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
		Kind string `json:"kind"`
		// Seq is the line's identity, stable across the live delta and every
		// later snapshot replay of it. Timestamps repeat within a turn, so the
		// page has nothing else to dedupe a reconnect against.
		Seq    int64  `json:"seq"`
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

	// Run is the whole-run summary: how far along, how long it has taken, and
	// how fast money is going out. Recomputed per tail cycle, not per event.
	Run struct {
		Kind       string  `json:"kind"`
		StartedTS  int64   `json:"startedTs"`
		LatestTS   int64   `json:"latestTs"`
		TasksTotal int     `json:"tasksTotal"`
		TasksDone  int     `json:"tasksDone"`
		TasksLive  int     `json:"tasksLive"`
		Spent      float64 `json:"spent"`
		Budget     float64 `json:"budget"`
		// BurnPerHour is measured over a trailing window, not the whole run —
		// a lifetime average is meaningless once a run has idled.
		BurnPerHour float64 `json:"burnPerHour"`
	}
)

// burnWindow bounds the trailing span used for the burn rate. It is measured
// against wall clock, not against the last event read: anchoring it to the log
// freezes both ends the moment a run goes quiet, and the page then shows a live
// rate for a run that has done nothing for an hour.
const burnWindow = 10 * 60 * 1000 // ms

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
	budget   float64

	nodes     map[string]*Node
	nodeOrder []string
	edges     map[string]bool
	edgeOrder []Edge
	parent    map[string]string

	spawnByTool map[string]spawnRec
	childByTool map[string]string
	// spawnWait and childWait are the two halves of the name join, queued rather
	// than mapped: the same lane name is spawned again after a restart, and a
	// last-write-wins map would hand the second agent the first one's result.
	spawnWait  map[string][]spawnRec
	childWait  map[string][]string
	reportWait map[string]int64
	// claimed keeps a name claim to one attempt per agent. Register is called
	// again whenever a sidecar is re-read, and a second claim would take the
	// next agent's spawn record.
	claimed map[string]bool

	activity    []Activity
	activitySeq int64
	knowledge   []Knowledge
	knowSeen    map[string]bool

	tasksCreated int
	taskState    map[string]taskMark
	spend        []spendSample
	firstTS      int64
	latestTS     int64
	countedTurns map[string]bool

	// now is injectable so the burn window can be exercised deterministically.
	now func() time.Time
}

// usageCounted reports whether this turn's usage has already been folded in,
// and records it if not. An empty id means the line carries no turn identity —
// count it, since there is nothing to deduplicate against. Bounded by turns
// rather than lines, so a long run costs one small string per turn.
func (g *Graph) usageCounted(messageID string) bool {
	if messageID == "" {
		return false
	}
	if g.countedTurns[messageID] {
		return true
	}
	g.countedTurns[messageID] = true
	return false
}

// taskMark keeps the timestamp so the newest status wins regardless of the
// order the files happened to be read in.
type taskMark struct {
	status string
	ts     int64
}

type spendSample struct {
	ts   int64
	cost float64
}

func NewGraph(wikiRoot, home string) *Graph {
	return &Graph{
		wikiRoot:     wikiRoot,
		home:         home,
		nodes:        map[string]*Node{},
		edges:        map[string]bool{},
		parent:       map[string]string{},
		spawnByTool:  map[string]spawnRec{},
		childByTool:  map[string]string{},
		spawnWait:    map[string][]spawnRec{},
		childWait:    map[string][]string{},
		reportWait:   map[string]int64{},
		claimed:      map[string]bool{},
		knowSeen:     map[string]bool{},
		taskState:    map[string]taskMark{},
		countedTurns: map[string]bool{},
		now:          time.Now,
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

	rec, spawned := g.claimSpawn(agentID, m)
	if _, linked := g.parent[agentID]; !linked {
		from, ts, minted := g.parentFor(agentID, m, rec, spawned)
		out = append(out, minted...)
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

// claimSpawn binds this agent to the tool_use that created it, permanently and
// exclusively. A teammate sidecar carries no toolUseId, so name is the only
// join key available — and a name is not unique across a run, so each spawn
// record is claimed by exactly one agent, oldest first, and recorded by toolID
// so nothing afterwards resolves through the shared name.
func (g *Graph) claimSpawn(agentID string, m agentMeta) (spawnRec, bool) {
	if m.ToolUseID != "" {
		g.childByTool[m.ToolUseID] = agentID
		rec, ok := g.spawnByTool[m.ToolUseID]
		return rec, ok
	}
	if m.Name == "" || g.claimed[agentID] {
		return spawnRec{}, false
	}
	g.claimed[agentID] = true
	if q := g.spawnWait[m.Name]; len(q) > 0 {
		rec := q[0]
		g.spawnWait[m.Name] = q[1:]
		if rec.toolID != "" {
			g.childByTool[rec.toolID] = agentID
		}
		return rec, true
	}
	g.childWait[m.Name] = append(g.childWait[m.Name], agentID)
	return spawnRec{}, false
}

// parentFor prefers hard evidence: the spawning tool_use, then the sidecar's
// parentAgentId. With neither, only a sidecar that declares depth 0 justifies
// the lead. Guessing otherwise parents a nested agent to the lead before its
// real spawn is read, and the true edge then lands as a second one — the node
// renders with two parents.
func (g *Graph) parentFor(agentID string, m agentMeta, rec spawnRec, spawned bool) (string, int64, []any) {
	if spawned {
		return rec.from, rec.ts, nil
	}
	if m.ParentAgentID != "" && m.ParentAgentID != agentID {
		var minted []any
		// The parent's own transcript may not have been read yet, and a client
		// given only the edge has no node to hang it on.
		if p, created := g.node(m.ParentAgentID, RoleCrew, m.ParentAgentID); created {
			minted = append(minted, *p)
		}
		return m.ParentAgentID, 0, minted
	}
	if m.SpawnDepth == 0 && (m.Name != "" || m.AgentType != "") {
		return LeadNodeID, 0, nil
	}
	return "", 0, nil // no sidecar yet; a later Register or recordSpawn will link it
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
	if msg.Model != "" {
		n.Model = msg.Model
	}

	// Usage is folded once per assistant turn. A turn spans several lines and
	// each repeats the same usage object, so counting per line multiplies the
	// bill by the number of blocks — measured at 2.6x on a real run. A turn
	// with no id (older logs, user events) falls back to per-line, which is
	// correct for those shapes.
	if !g.usageCounted(msg.ID) {
		n.Tokens += msg.Usage.billed()
		if r, ok := rateFor(msg.Model, msg.Usage.Speed, ts); ok {
			if spent := r.cost(msg.Usage); spent > 0 {
				n.Cost += spent
				g.spend = append(g.spend, spendSample{ts: ts, cost: spent})
			}
		} else {
			n.Unpriced += msg.Usage.billed()
		}
	}
	if ts > n.TS {
		n.TS = ts
	}
	if ts > 0 {
		if g.firstTS == 0 || ts < g.firstTS {
			g.firstTS = ts
		}
		if ts > g.latestTS {
			g.latestTS = ts
		}
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
				g.recordTask(b.Name, in, ts)
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
	g.activitySeq++
	a := Activity{Kind: "activity", Seq: g.activitySeq, ID: id, Tool: tool, Detail: detail, TS: ts}
	g.activity = append(g.activity, a)
	if len(g.activity) > activityLimit {
		g.activity = g.activity[len(g.activity)-activityLimit:]
	}
	return a
}

// SetBudget sets the ceiling the run is projected against. Zero means no
// ceiling — the page then shows a burn rate and no exhaustion estimate.
func (g *Graph) SetBudget(usd float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.budget = usd
}

// recordTask follows the lead's shared task list. Creation is counted rather
// than identified: the tool call carries no id (the id comes back in the
// result), and a count is all the progress figure needs.
func (g *Graph) recordTask(tool string, in toolInput, ts int64) {
	switch tool {
	case "TaskCreate":
		g.tasksCreated++
	case "TaskUpdate":
		if in.TaskID == "" || in.Status == "" {
			return // an owner-only or blocked-by-only update carries no status
		}
		// Newest status wins on timestamp, not arrival: transcripts are tailed
		// independently, so arrival order is not chronological order.
		if prev, ok := g.taskState[in.TaskID]; ok && prev.ts > ts {
			return
		}
		g.taskState[in.TaskID] = taskMark{status: in.Status, ts: ts}
	}
}

// RunSummary is the whole-run rollup.
func (g *Graph) RunSummary() Run {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.runSummary()
}

func (g *Graph) runSummary() Run {
	r := Run{
		Kind:       "run",
		StartedTS:  g.firstTS,
		LatestTS:   g.latestTS,
		TasksTotal: g.tasksCreated,
		Budget:     g.budget,
	}
	for _, n := range g.nodes {
		r.Spent += n.Cost
	}
	for _, m := range g.taskState {
		switch m.status {
		case "completed":
			r.TasksDone++
		case "in_progress":
			r.TasksLive++
		}
	}
	if r.TasksDone > r.TasksTotal {
		r.TasksTotal = r.TasksDone // a task created before the log we can see
	}

	// Burn over the trailing window only, with both ends anchored to now: as a
	// run idles the numerator loses samples while the denominator keeps growing,
	// so the rate decays to zero instead of freezing. Two samples are still the
	// minimum — one cost divided by the sliver of time since it landed is a
	// spike, not a rate.
	nowMS := g.now().UnixMilli()
	cutoff := nowMS - burnWindow
	kept, windowCost, earliest := g.spend[:0], 0.0, int64(0)
	for _, s := range g.spend {
		if s.ts < cutoff {
			continue
		}
		kept = append(kept, s)
		windowCost += s.cost
		if earliest == 0 || s.ts < earliest {
			earliest = s.ts
		}
	}
	g.spend = kept
	if span := nowMS - earliest; len(kept) > 1 && span > 0 && windowCost > 0 {
		r.BurnPerHour = windowCost / (float64(span) / 3_600_000)
	}
	return r
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

	child, ok := "", false
	if b.ID != "" {
		child, ok = g.childByTool[b.ID]
	}
	if !ok && in.Name != "" {
		if q := g.childWait[in.Name]; len(q) > 0 {
			child, ok = q[0], true
			g.childWait[in.Name] = q[1:]
			if b.ID != "" {
				g.childByTool[b.ID] = child
			}
		}
	}
	if !ok {
		if in.Name != "" {
			g.spawnWait[in.Name] = append(g.spawnWait[in.Name], rec)
		}
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

// childFor resolves strictly by tool_use id. Falling back to the spawn's name
// would hand this result to whichever agent registered under that name last.
func (g *Graph) childFor(toolUseID string) (string, bool) {
	id, ok := g.childByTool[toolUseID]
	return id, ok
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
	return append(out, g.runSummary())
}
