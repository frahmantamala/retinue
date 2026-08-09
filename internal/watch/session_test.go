package watch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The ids from the real misfire: a 50-line transcript and a 470-line one that
// shared a modification time, sorted arbitrarily, and attached to the wrong
// session. Note the small one sorts first by name.
const (
	staleID = "25e132fd-16c5-4ee4-b457-0201409bfe11"
	liveID  = "29c5ac14-5234-48cf-8938-857ddfed8f84"
)

func projectDir(t *testing.T, home, repo string) string {
	t.Helper()
	dir := ProjectDirFor(home, repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeTranscript lays down a transcript of n lines ending at last, and stamps
// the file's mtime independently so the two signals can disagree.
func writeTranscript(t *testing.T, dir, id string, n int, last time.Time, mod time.Time) string {
	t.Helper()
	var b strings.Builder
	for i := range n {
		ts := last.Add(-time.Duration(n-1-i) * time.Second).UTC().Format(time.RFC3339Nano)
		fmt.Fprintf(&b, `{"type":"assistant","uuid":"u%d","timestamp":%q,"message":{"id":"m%d"}}`+"\n", i, ts, i)
	}
	path := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
	return path
}

func discover(t *testing.T, home, repo, id string) Session {
	t.Helper()
	s, err := Discover(home, repo, id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The reported bug: identical mtimes must not be resolved by whatever the
// filesystem hands back first.
func TestDiscoverExactMtimeTiePicksLiveSession(t *testing.T) {
	home := t.TempDir()
	repo := "/Users/x/work/tie"
	dir := projectDir(t, home, repo)

	mod := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	writeTranscript(t, dir, staleID, 50, mod.Add(-90*time.Minute), mod)
	writeTranscript(t, dir, liveID, 470, mod.Add(-time.Minute), mod)

	// Repeated because the old failure mode was an unstable sort over equal
	// keys: one correct run proved nothing.
	for range 25 {
		s := discover(t, home, repo, "")
		if s.ID != liveID {
			t.Fatalf("want live session %s, got %s", liveID, s.ID)
		}
	}
	if s := discover(t, home, repo, ""); !strings.Contains(s.Selection, "modified within") {
		t.Errorf("Selection should report the near-tie, got %q", s.Selection)
	}
}

// Last event beats size, so the tiebreak is not just "biggest file wins": a
// long-abandoned transcript should lose to a smaller one still being written.
func TestDiscoverTiePrefersLatestEventOverSize(t *testing.T) {
	home := t.TempDir()
	repo := "/Users/x/work/recency"
	dir := projectDir(t, home, repo)

	mod := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	writeTranscript(t, dir, "aaaa", 400, mod.Add(-6*time.Hour), mod)
	writeTranscript(t, dir, "bbbb", 40, mod.Add(-30*time.Second), mod)

	if s := discover(t, home, repo, ""); s.ID != "bbbb" {
		t.Errorf("want the recently active transcript bbbb, got %s", s.ID)
	}
}

// Mtimes a fraction of a second apart are noise, not ranking: a background
// session touching a file must not steal the attach.
func TestDiscoverSubSecondSkewCountsAsTie(t *testing.T) {
	home := t.TempDir()
	repo := "/Users/x/work/skew"
	dir := projectDir(t, home, repo)

	base := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	writeTranscript(t, dir, staleID, 50, base.Add(-2*time.Hour), base.Add(300*time.Millisecond))
	writeTranscript(t, dir, liveID, 470, base, base)

	if s := discover(t, home, repo, ""); s.ID != liveID {
		t.Errorf("want %s despite its slightly older mtime, got %s", liveID, s.ID)
	}
}

// Without any parseable timestamps the tiebreak falls through to size, and the
// bigger file wins even though its name sorts last.
func TestDiscoverTieFallsBackToSize(t *testing.T) {
	home := t.TempDir()
	repo := "/Users/x/work/garbage"
	dir := projectDir(t, home, repo)

	mod := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	for name, body := range map[string]string{
		"aaaa": "not json\n",
		"zzzz": strings.Repeat("also not json\n", 40),
	} {
		p := filepath.Join(dir, name+".jsonl")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}

	s := discover(t, home, repo, "")
	if s.ID != "zzzz" {
		t.Errorf("want the larger transcript zzzz, got %s", s.ID)
	}
	if !strings.Contains(s.Selection, "no timestamp") {
		t.Errorf("Selection should admit it had no event to date, got %q", s.Selection)
	}
}

// Outside the tie window mtime still rules, even against a much larger file.
func TestDiscoverClearMtimeWinnerStillWins(t *testing.T) {
	home := t.TempDir()
	repo := "/Users/x/work/clear"
	dir := projectDir(t, home, repo)

	now := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	writeTranscript(t, dir, "big-and-old", 900, now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	writeTranscript(t, dir, "small-and-new", 3, now, now)

	s := discover(t, home, repo, "")
	if s.ID != "small-and-new" {
		t.Errorf("want small-and-new, got %s", s.ID)
	}
	if s.Selection != "newest of 2 transcripts" {
		t.Errorf("unexpected Selection %q", s.Selection)
	}
}

func TestDiscoverSingleTranscript(t *testing.T) {
	home := t.TempDir()
	repo := "/Users/x/work/solo"
	dir := projectDir(t, home, repo)

	now := time.Now()
	writeTranscript(t, dir, "only", 5, now, now)

	s := discover(t, home, repo, "")
	if s.ID != "only" {
		t.Fatalf("want only, got %s", s.ID)
	}
	if s.LeadPath != filepath.Join(dir, "only.jsonl") {
		t.Errorf("unexpected lead path %q", s.LeadPath)
	}
	if s.SubagentDir != filepath.Join(dir, "only", "subagents") {
		t.Errorf("unexpected subagent dir %q", s.SubagentDir)
	}
	if s.Selection == "" {
		t.Error("an automatic pick should always explain itself")
	}
}

// The directory exists but holds nothing attachable — including entries that
// only look like transcripts.
func TestDiscoverEmptyProjectDir(t *testing.T) {
	home := t.TempDir()
	repo := "/Users/x/work/empty"
	dir := projectDir(t, home, repo)

	if _, err := Discover(home, repo, ""); err == nil {
		t.Fatal("want an error for a project dir with no transcripts")
	}

	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "decoy.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(home, repo, ""); err == nil {
		t.Error("a directory named *.jsonl is not a transcript")
	}
}

// An explicit id skips ranking entirely: the loser of every tiebreak above is
// still selectable by name.
func TestDiscoverExplicitIDBypassesSelection(t *testing.T) {
	home := t.TempDir()
	repo := "/Users/x/work/explicit"
	dir := projectDir(t, home, repo)

	now := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	writeTranscript(t, dir, staleID, 50, now.Add(-3*time.Hour), now.Add(-3*time.Hour))
	writeTranscript(t, dir, liveID, 470, now, now)

	s := discover(t, home, repo, staleID)
	if s.ID != staleID {
		t.Fatalf("want the requested session %s, got %s", staleID, s.ID)
	}
	if s.Selection != "" {
		t.Errorf("Selection must stay empty when the caller named the session, got %q", s.Selection)
	}

	if _, err := Discover(home, repo, "0000"); err == nil {
		t.Error("want an error for an unknown session id")
	}
}

// A partial trailing line is normal on a live session and must not hide the
// events before it.
func TestLastEventMillisIgnoresPartialTail(t *testing.T) {
	dir := t.TempDir()
	last := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	path := writeTranscript(t, dir, "s", 4, last, last)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"type":"assistant","timesta`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if got := lastEventMillis(path); got != last.UnixMilli() {
		t.Errorf("want %d, got %d", last.UnixMilli(), got)
	}
}

// Only the tail is read, so the answer must not depend on the file fitting in
// the window.
func TestLastEventMillisReadsBeyondTailWindow(t *testing.T) {
	dir := t.TempDir()
	last := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	path := filepath.Join(dir, "big.jsonl")

	var b strings.Builder
	b.WriteString(strings.Repeat(`{"type":"user","message":"`+strings.Repeat("x", 4000)+`"}`+"\n", 40))
	fmt.Fprintf(&b, `{"type":"assistant","timestamp":%q,"message":{"id":"m"}}`+"\n", last.Format(time.RFC3339Nano))
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if fi.Size() <= tailBytes {
		t.Fatalf("fixture must exceed the %d byte tail window, got %d", tailBytes, fi.Size())
	}

	if got := lastEventMillis(path); got != last.UnixMilli() {
		t.Errorf("want %d, got %d", last.UnixMilli(), got)
	}
}

func TestLastEventMillisMissingFile(t *testing.T) {
	if got := lastEventMillis(filepath.Join(t.TempDir(), "absent.jsonl")); got != 0 {
		t.Errorf("want 0 for an unreadable transcript, got %d", got)
	}
}
