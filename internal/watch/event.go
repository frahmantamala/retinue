package watch

import (
	"encoding/json"
	"time"
)

// event is one line of a session JSONL.
//
// Every field is optional on purpose. The log is written by a Claude Code build
// that ships faster than this reader, so an unknown shape must decode to a
// usable zero value instead of failing the line.
type event struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	SessionID   string          `json:"sessionId"`
	AgentID     string          `json:"agentId"`
	IsSidechain bool            `json:"isSidechain"`
	Timestamp   string          `json:"timestamp"`
	Message     json.RawMessage `json:"message"`
}

type message struct {
	Content contentBlocks `json:"content"`
	Usage   usage         `json:"usage"`
}

type usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// billed counts tokens the turn actually paid for. cache_read is excluded: the
// same prefix is re-read every turn, so summing it grows without bound and the
// node radius stops meaning anything.
func (u usage) billed() int {
	return u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens
}

type contentBlock struct {
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	ToolUseID string          `json:"tool_use_id"`
	Text      string          `json:"text"`
	Input     json.RawMessage `json:"input"`
}

// toolInput is the union of the input fields worth surfacing. Every tool has
// its own schema; these are the ones that answer "what is it doing right now".
type toolInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	FilePath    string `json:"file_path"`
	Path        string `json:"path"`
	Pattern     string `json:"pattern"`
	Command     string `json:"command"`
	Query       string `json:"query"`
	URL         string `json:"url"`
	Prompt      string `json:"prompt"`
}

func (b contentBlock) input() toolInput {
	var in toolInput
	if len(b.Input) > 0 {
		_ = json.Unmarshal(b.Input, &in)
	}
	return in
}

// contentBlocks tolerates both shapes the log uses: an array of blocks, or a
// bare string for plain user text.
type contentBlocks []contentBlock

func (c *contentBlocks) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '[' {
		var blocks []contentBlock
		if err := json.Unmarshal(b, &blocks); err != nil {
			return err
		}
		*c = blocks
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*c = contentBlocks{{Type: "text"}}
		return nil
	}
	return nil
}

func (e event) decodeMessage() message {
	var m message
	if len(e.Message) > 0 {
		_ = json.Unmarshal(e.Message, &m)
	}
	return m
}

// millis returns the event time as a Unix milli stamp, or 0 when the timestamp
// is missing or unparseable.
func (e event) millis() int64 {
	if e.Timestamp == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, e.Timestamp)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

// decodeEvent reports ok=false for a line that is not usable JSON. Callers skip
// those; a half-written trailing line is normal while a session is live.
func decodeEvent(line []byte) (event, bool) {
	var e event
	if err := json.Unmarshal(line, &e); err != nil {
		return event{}, false
	}
	return e, true
}
