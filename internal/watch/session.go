package watch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
// Code does: every separator becomes a dash.
func ProjectDirFor(home, repo string) string {
	return filepath.Join(home, ".claude", "projects", strings.ReplaceAll(repo, string(filepath.Separator), "-"))
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

	var lead string
	if id != "" {
		lead = filepath.Join(dir, id+".jsonl")
		if _, err := os.Stat(lead); err != nil {
			return Session{}, fmt.Errorf("no session %s in %s", id, dir)
		}
	} else {
		lead, err = newestTranscript(dir)
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
	}
	if name := teamLeadName(home, s.ID); name != "" {
		s.LeadLabel = name
	}
	return s, nil
}

func newestTranscript(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	type candidate struct {
		path string
		mod  int64
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
		found = append(found, candidate{filepath.Join(dir, e.Name()), info.ModTime().UnixNano()})
	}
	if len(found) == 0 {
		return "", errors.New("no session transcripts found")
	}
	sort.Slice(found, func(i, j int) bool { return found[i].mod > found[j].mod })
	return found[0].path, nil
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
