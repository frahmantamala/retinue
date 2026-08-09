package watch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LeadNodeID is the graph id of the session that owns the run. Crew nodes use
// the agentId the log gives them; the lead has none, so it gets a fixed one.
const LeadNodeID = "lead"

// Session locates the files one run writes. Nothing here is written to — the
// monitor is strictly a reader of ~/.claude.
type Session struct {
	Repo        string
	ProjectDir  string
	ID          string
	LeadPath    string
	SubagentDir string
	LeadLabel   string

	// Selection explains an automatic pick, and is empty when the caller named
	// the session. Attaching to the wrong transcript is otherwise indefensible
	// from the outside: the id alone does not say a near-tie existed.
	Selection string
}

// agentMeta is the sidecar written next to each subagent transcript. It is the
// only place a teammate's human name is recorded, so labels come from here
// rather than from string surgery on the agentId.
type agentMeta struct {
	Name          string `json:"name"`
	AgentType     string `json:"agentType"`
	Description   string `json:"description"`
	ParentAgentID string `json:"parentAgentId"`
	ToolUseID     string `json:"toolUseId"`
	SpawnDepth    int    `json:"spawnDepth"`
}

func (m agentMeta) label(fallback string) string {
	switch {
	case m.Name != "":
		return m.Name
	case m.AgentType != "":
		return m.AgentType
	default:
		return fallback
	}
}

// ProjectDirFor maps a repo path to its transcript directory the way Claude
// Code does: separators AND dots both become dashes. Missing the dot rule
// breaks every path with a hidden directory in it — notably worktrees under
// ~/.claude/worktrees, which the team playbook recommends creating.
var projectDirEscape = strings.NewReplacer(string(filepath.Separator), "-", ".", "-")

func ProjectDirFor(home, repo string) string {
	return filepath.Join(home, ".claude", "projects", projectDirEscape.Replace(repo))
}

// Discover resolves the session to watch. An empty id selects the most recently
// modified transcript in the repo's project directory.
func Discover(home, repo, id string) (Session, error) {
	abs, err := filepath.Abs(repo)
	if err != nil {
		return Session{}, err
	}
	dir := ProjectDirFor(home, abs)
	if _, err := os.Stat(dir); err != nil {
		return Session{}, fmt.Errorf("no transcripts for %s (looked in %s)", abs, dir)
	}

	var lead, selection string
	if id != "" {
		lead = filepath.Join(dir, id+".jsonl")
		if _, err := os.Stat(lead); err != nil {
			return Session{}, fmt.Errorf("no session %s in %s", id, dir)
		}
	} else {
		lead, selection, err = newestTranscript(dir)
		if err != nil {
			return Session{}, err
		}
	}

	s := Session{
		Repo:        abs,
		ProjectDir:  dir,
		ID:          strings.TrimSuffix(filepath.Base(lead), ".jsonl"),
		LeadPath:    lead,
		SubagentDir: filepath.Join(strings.TrimSuffix(lead, ".jsonl"), "subagents"),
		LeadLabel:   "lead",
		Selection:   selection,
	}
	if name := teamLeadName(home, s.ID); name != "" {
		s.LeadLabel = name
	}
	return s, nil
}

// tieWindow is how close two modification times have to be before they stop
// carrying information. Mtimes are nanosecond-precise, so a dormant session
// appending one line can out-rank the session the user is watching by a margin
// that means nothing; inside the window, content decides instead.
const tieWindow = time.Second

// tailBytes caps the read used to date a transcript. Transcripts reach
// megabytes, so only the end is read — and only for the handful of files that
// tied, which is usually none.
const tailBytes = 64 << 10

type candidate struct {
	path string
	name string
	mod  time.Time
	size int64
	last int64 // last event's Unix milli, 0 when the tail yields no timestamp
}

// newestTranscript picks the transcript to attach to and explains the pick.
// Modification time ranks first, but only outside tieWindow: within it the
// files are treated as equally recent and resolved by their newest event, then
// by size, then by name. Every tiebreak is a property of the files themselves,
// so the result never depends on readdir or map order.
func newestTranscript(dir string) (string, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", err
	}
	var found []candidate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		found = append(found, candidate{
			path: filepath.Join(dir, e.Name()),
			name: e.Name(),
			mod:  info.ModTime(),
			size: info.Size(),
		})
	}
	if len(found) == 0 {
		return "", "", errors.New("no session transcripts found")
	}

	sort.Slice(found, func(i, j int) bool {
		if !found[i].mod.Equal(found[j].mod) {
			return found[i].mod.After(found[j].mod)
		}
		return found[i].name < found[j].name
	})

	n := 1
	for n < len(found) && found[0].mod.Sub(found[n].mod) < tieWindow {
		n++
	}
	tied := found[:n]
	if len(tied) > 1 {
		for i := range tied {
			tied[i].last = lastEventMillis(tied[i].path)
		}
		sort.Slice(tied, func(i, j int) bool {
			a, b := tied[i], tied[j]
			switch {
			case a.last != b.last:
				return a.last > b.last
			case a.size != b.size:
				return a.size > b.size
			default:
				return a.name < b.name
			}
		})
	}
	return tied[0].path, selectionReason(len(found), tied), nil
}

func selectionReason(total int, tied []candidate) string {
	if total == 1 {
		return "only transcript in the project directory"
	}
	if len(tied) == 1 {
		return fmt.Sprintf("newest of %d transcripts", total)
	}
	when := "no timestamp"
	if tied[0].last > 0 {
		when = time.UnixMilli(tied[0].last).Format(time.RFC3339)
	}
	return fmt.Sprintf("newest of %d transcripts; %d modified within %s of each other, "+
		"chose the most recently active (last event %s, %d bytes)",
		total, len(tied), tieWindow, when, tied[0].size)
}

// lastEventMillis dates a transcript by its final usable event. It reads only
// the tail, and returns 0 rather than an error for anything unreadable — a
// missing date demotes the file, it does not fail the whole selection.
func lastEventMillis(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0
	}
	off := info.Size() - tailBytes
	if off < 0 {
		off = 0
	}
	buf := make([]byte, info.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && !errors.Is(err, io.EOF) {
		return 0
	}

	lines := bytes.Split(buf, []byte("\n"))
	if off > 0 {
		lines = lines[1:] // the window opened mid-record
	}
	// Backwards: the final line is often a partial append on a live session.
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		if e, ok := decodeEvent(line); ok {
			if ms := e.millis(); ms > 0 {
				return ms
			}
		}
	}
	return 0
}

// teamLeadName reads the team config for a nicer lead label. The config keys
// its directory by the session id's first segment, and its member agentIds
// ("lane1@session-x") do not match transcript agentIds — only names join, so
// nothing else here depends on it.
func teamLeadName(home, sessionID string) string {
	short, _, ok := strings.Cut(sessionID, "-")
	if !ok || short == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(home, ".claude", "teams", "session-"+short, "config.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Members []struct {
			AgentID string `json:"agentId"`
			Name    string `json:"name"`
		} `json:"members"`
		LeadAgentID string `json:"leadAgentId"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return ""
	}
	for _, m := range cfg.Members {
		if m.AgentID == cfg.LeadAgentID && m.Name != "" {
			return m.Name
		}
	}
	return ""
}

// agentIDFromPath recovers the agentId embedded in a transcript filename;
// the events inside carry the same value.
func agentIDFromPath(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	return strings.TrimPrefix(base, "agent-")
}

func readAgentMeta(path string) agentMeta {
	var m agentMeta
	b, err := os.ReadFile(strings.TrimSuffix(path, ".jsonl") + ".meta.json")
	if err != nil {
		return m
	}
	_ = json.Unmarshal(b, &m)
	return m
}
