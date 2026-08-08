package watch

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// source is one transcript being followed, and the graph node its lines belong
// to. Offsets advance only past complete lines, so a half-written trailing line
// is simply re-read on the next tick.
type source struct {
	path   string
	nodeID string
	off    int64
}

// Watcher tails a session's transcripts and the teammate transcripts beneath
// it, publishing graph deltas as lines arrive. Poll-based: the directory gains
// files mid-run and fsnotify would be a dependency for no benefit at this size.
type Watcher struct {
	sess     Session
	graph    *Graph
	hub      *Hub
	interval time.Duration
	logf     func(string, ...any)

	sources     map[string]*source
	pendingMeta map[string]bool
	skipped     int
	lastRun     Run
}

func NewWatcher(sess Session, g *Graph, h *Hub, interval time.Duration, logf func(string, ...any)) *Watcher {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Watcher{
		sess:        sess,
		graph:       g,
		hub:         h,
		interval:    interval,
		logf:        logf,
		sources:     map[string]*source{},
		pendingMeta: map[string]bool{},
	}
}

// Run blocks until ctx is cancelled. It reads each transcript from the start so
// a finished run still renders; the brief said tail-from-end, but an empty
// graph is exactly the failure the shell stopgap had.
func (w *Watcher) Run(ctx context.Context) error {
	w.start()
	tick := time.NewTicker(w.interval)
	defer tick.Stop()
	for {
		w.pass()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func (w *Watcher) start() {
	w.hub.Publish(w.graph.EnsureLead(w.sess.LeadLabel))
	w.sources[w.sess.LeadPath] = &source{path: w.sess.LeadPath, nodeID: LeadNodeID}
}

// pass is one discovery-and-read cycle: pick up new teammates, then drain every
// transcript. Crew are registered before their lines are read so a completion
// in the lead log can resolve to a node in the same cycle.
func (w *Watcher) pass() {
	w.scan()
	w.poll()
	// Once per cycle rather than per event: the rollup changes on almost every
	// line, and a quiet run then publishes nothing at all.
	if r := w.graph.RunSummary(); r != w.lastRun {
		w.lastRun = r
		w.hub.Publish([]any{r})
	}
}

// scan picks up teammate transcripts that appear after the run starts.
func (w *Watcher) scan() {
	entries, err := os.ReadDir(w.sess.SubagentDir)
	if err != nil {
		return // no crew yet; the directory is created with the first teammate
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		path := filepath.Join(w.sess.SubagentDir, name)
		_, known := w.sources[path]
		if known && !w.pendingMeta[path] {
			continue
		}

		meta := readAgentMeta(path)
		if meta.Name == "" && meta.AgentType == "" {
			// The sidecar is written separately from the transcript; keep
			// retrying so the node does not keep a raw agentId as its label.
			w.pendingMeta[path] = true
		} else {
			delete(w.pendingMeta, path)
		}

		id := agentIDFromPath(path)
		w.hub.Publish(w.graph.Register(id, meta))
		if !known {
			w.sources[path] = &source{path: path, nodeID: id}
			w.logf("crew %s (%s)", meta.label(id), name)
		}
	}
}

func (w *Watcher) poll() {
	for _, s := range w.sources {
		w.read(s)
	}
}

func (w *Watcher) read(s *source) {
	f, err := os.Open(s.path)
	if err != nil {
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return
	}
	if fi.Size() < s.off {
		s.off = 0 // truncated or replaced
	}
	if fi.Size() == s.off {
		return
	}
	if _, err := f.Seek(s.off, io.SeekStart); err != nil {
		return
	}

	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return // trailing partial line; the writer is mid-append
		}
		s.off += int64(len(line))
		w.handle(s.nodeID, bytes.TrimSpace(line))
	}
}

func (w *Watcher) handle(nodeID string, line []byte) {
	if len(line) == 0 {
		return
	}
	e, ok := decodeEvent(line)
	if !ok {
		w.skipped++
		w.logf("skipped unparseable line %d in %s", w.skipped, nodeID)
		return
	}
	w.hub.Publish(w.graph.Apply(nodeID, e))
}

// Skipped reports how many lines failed to decode.
func (w *Watcher) Skipped() int { return w.skipped }
