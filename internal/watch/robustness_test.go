package watch

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// A path or description in Indonesian, CJK or emoji must survive truncation.
// A byte cut lands mid-rune and json.Marshal replaces the broken tail with
// U+FFFD, so the browser shows a glyph where the filename should be.
func TestShortenCutsOnRunesNotBytes(t *testing.T) {
	cases := []struct{ name, unit string }{
		{"indonesian", "ê"},
		{"cjk", "測"},
		{"emoji", "🚀"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shorten(strings.Repeat(c.unit, 200))
			if !utf8.ValidString(got) {
				t.Fatalf("truncated to invalid UTF-8: %q", got)
			}
			if n := utf8.RuneCountInString(got); n != detailMax {
				t.Errorf("want %d runes, got %d (%q)", detailMax, n, got)
			}
			if !strings.HasSuffix(got, "…") {
				t.Errorf("want an ellipsis, got %q", got)
			}
		})
	}
}

// Exactly detailMax multi-byte runes exceeds detailMax bytes, but is short
// enough to keep whole.
func TestShortenMeasuresLengthInRunes(t *testing.T) {
	s := strings.Repeat("測", detailMax)
	if got := shorten(s); got != s {
		t.Errorf("want the string kept whole, got %q", got)
	}
}

func TestHostCheckRejectsNonLoopback(t *testing.T) {
	h := NewServer(fixtureSession(), NewGraph("", ""), NewHub()).Handler()

	// Loopback by name is what a DNS rebind gives the browser; the Host header
	// is the only thing that still names the attacker's origin.
	rejected := []string{
		"attacker.example.com",
		"attacker.example.com:7777",
		"retinue.internal",
		"192.168.1.10:7777",
		"",
	}
	// /events streams until its context ends; a cancelled one keeps a handler
	// that wrongly accepts the request from hanging the test instead of failing it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for _, host := range rejected {
		for _, path := range []string{"/", "/events", "/session"} {
			req := httptest.NewRequest("GET", path, nil).WithContext(ctx)
			req.Host = host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != 403 {
				t.Errorf("Host %q %s: want 403, got %d", host, path, rec.Code)
			}
		}
	}

	accepted := []string{"localhost", "localhost:7777", "127.0.0.1", "127.0.0.1:7777", "[::1]", "[::1]:7777"}
	for _, host := range accepted {
		for _, path := range []string{"/", "/session"} {
			req := httptest.NewRequest("GET", path, nil)
			req.Host = host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Errorf("Host %q %s: want 200, got %d", host, path, rec.Code)
			}
		}
	}
}

func TestSessionStaysUnreachableOffBox(t *testing.T) {
	srv := httptest.NewServer(NewServer(fixtureSession(), NewGraph("", ""), NewHub()).Handler())
	defer srv.Close()

	req := httptest.NewRequest("GET", srv.URL+"/session", nil)
	req.RequestURI = ""
	req.Host = "rebind.attacker.example"
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("want 403 for a rebound Host, got %d", resp.StatusCode)
	}
}

// writeTurns replaces path with n turns tagged by run, each a distinct priced
// assistant turn. Every line is the same width so a shorter file is reliably a
// smaller file, which is what the tailer's restart guard keys on.
func writeTurns(t *testing.T, path, run string, n int) {
	t.Helper()
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, `{"type":"assistant","uuid":"u%s%d","message":{"id":"m%s%d","model":"claude-opus-5","usage":{"input_tokens":100,"output_tokens":50},"content":[{"type":"tool_use","id":"t%s%d","name":"Bash","input":{"description":"step %s%d"}}]}}`+"\n", run, i, run, i, run, i, run, i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func leadNode(t *testing.T, g *Graph) Node {
	t.Helper()
	n, ok := nodesOf(g)[LeadNodeID]
	if !ok {
		t.Fatal("no lead node")
	}
	return n
}

// A shrunk transcript rewinds the offset and replays what it holds. That may
// repeat activity rows, which is accepted; what must not happen is usage being
// counted twice, or the events of a replacement file being skipped.
func TestTruncatedTranscriptDoesNotDoubleCountUsage(t *testing.T) {
	dir := t.TempDir()
	lead := filepath.Join(dir, "lead.jsonl")
	sess := Session{ID: "lead", LeadPath: lead, SubagentDir: filepath.Join(dir, "subagents"), LeadLabel: "lead"}

	g := NewGraph("", "")
	w := NewWatcher(sess, g, NewHub(), time.Millisecond, nil)
	w.start()

	writeTurns(t, lead, "A", 6)
	w.pass()
	base := leadNode(t, g)
	if base.Tokens == 0 || base.Cost == 0 {
		t.Fatalf("fixture must bill something, got tokens=%d cost=%v", base.Tokens, base.Cost)
	}

	// Same turns, shorter file: the whole prefix is re-read.
	writeTurns(t, lead, "A", 3)
	w.pass()
	if got := leadNode(t, g); got.Tokens != base.Tokens || got.Cost != base.Cost {
		t.Errorf("replay double-counted usage: want tokens=%d cost=%v, got tokens=%d cost=%v",
			base.Tokens, base.Cost, got.Tokens, got.Cost)
	}

	// A genuinely different, shorter file at the same path — the likely shape of
	// a shrink. Every one of its turns must render; none may be skipped as if it
	// were history already seen.
	writeTurns(t, lead, "B", 2)
	w.pass()

	seen := map[string]bool{}
	for _, a := range activityOf(g) {
		seen[a.Detail] = true
	}
	for _, want := range []string{"step B0", "step B1"} {
		if !seen[want] {
			t.Errorf("event from the replacement file was skipped: %q missing", want)
		}
	}

	perTurn := base.Tokens / 6
	if want := base.Tokens + 2*perTurn; leadNode(t, g).Tokens != want {
		t.Errorf("want tokens=%d (6 original + 2 new turns), got %d", want, leadNode(t, g).Tokens)
	}
}
