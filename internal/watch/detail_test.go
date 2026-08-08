package watch

import "testing"

func TestToolDetail(t *testing.T) {
	cases := []struct {
		tool string
		in   toolInput
		want string
	}{
		{"Bash", toolInput{Description: "Run the suite", Command: "go test ./..."}, "Run the suite"},
		{"Bash", toolInput{Command: "go test ./..."}, "go test ./..."},
		{"Read", toolInput{FilePath: "/Users/x/work/app/internal/watch/graph.go"}, "watch/graph.go"},
		{"Edit", toolInput{FilePath: "index.html"}, "index.html"},
		{"Grep", toolInput{Pattern: "func New", Path: "/Users/x/work/app/internal"}, "func New in internal"},
		{"Glob", toolInput{Pattern: "**/*.sql"}, "**/*.sql"},
		{"WebSearch", toolInput{Query: "postgres rls"}, "postgres rls"},
		{"Agent", toolInput{Description: "Lane 1 backend", Name: "lane1"}, "Lane 1 backend"},
		{"TaskUpdate", toolInput{}, ""},
	}
	for _, c := range cases {
		if got := toolDetail(c.tool, c.in); got != c.want {
			t.Errorf("%s: want %q, got %q", c.tool, c.want, got)
		}
	}
}

func TestShortenCollapsesAndTruncates(t *testing.T) {
	if got := shorten("  a\n\tb   c "); got != "a b c" {
		t.Errorf("whitespace: got %q", got)
	}
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}
	got := shorten(string(long))
	if len([]rune(got)) != detailMax {
		t.Errorf("want %d runes, got %d", detailMax, len([]rune(got)))
	}
}

func TestWikiPageFor(t *testing.T) {
	const root, home = "/Users/demo/work/wiki", "/Users/demo"
	cases := []struct{ path, want string }{
		{"/Users/demo/work/wiki/concepts/tenancy.md", "concepts/tenancy"},
		{"~/work/wiki/decisions/x.md", "decisions/x"},
		{"/Users/demo/work/wiki/raw/notes", "raw/notes"},
		{"/Users/demo/work/wiki", ""},                  // the root itself is not a page
		{"/Users/demo/work/wikipedia/page.md", ""},     // prefix must stop at a separator
		{"/Users/demo/work/app/internal/watch.go", ""}, // outside entirely
		{"", ""},
	}
	for _, c := range cases {
		if got := wikiPageFor(c.path, root, home); got != c.want {
			t.Errorf("%s: want %q, got %q", c.path, c.want, got)
		}
	}
}

func TestWikiPagesSources(t *testing.T) {
	const root, home = "/Users/demo/work/wiki", "/Users/demo"

	read := wikiPages("Read", toolInput{FilePath: "/Users/demo/work/wiki/concepts/tenancy.md"}, root, home)
	if len(read) != 1 || read[0].Page != "concepts/tenancy" || read[0].Via != ViaRead {
		t.Errorf("file read: %+v", read)
	}

	shell := wikiPages("Read", toolInput{Command: "rg -n tenant ~/work/wiki/decisions/x.md | head"}, root, home)
	if len(shell) != 1 || shell[0].Page != "decisions/x" || shell[0].Via != ViaRead {
		t.Errorf("shell: %+v", shell)
	}

	brief := wikiPages("Read", toolInput{Prompt: "Settled: [[decisions/monolith-first]] and [[concepts/tenancy.md]]. Go."}, root, home)
	if len(brief) != 2 {
		t.Fatalf("brief: want 2 links, got %+v", brief)
	}
	for _, h := range brief {
		if h.Via != ViaBrief {
			t.Errorf("want via=%s, got %+v", ViaBrief, h)
		}
	}
	if brief[1].Page != "concepts/tenancy" {
		t.Errorf("wikilink should drop .md: %q", brief[1].Page)
	}

	// Real briefs cite pages as plain paths far more often than as wikilinks.
	prose := wikiPages("Read", toolInput{
		Prompt: "READ FIRST: ~/work/wiki/concepts/tenancy.md and /Users/demo/work/wiki/decisions/rls.md. Every rule binds.",
	}, root, home)
	if len(prose) != 2 {
		t.Fatalf("prompt paths: want 2, got %+v", prose)
	}
	for _, h := range prose {
		if h.Via != ViaBrief {
			t.Errorf("a path in a brief is via=%s, got %+v", ViaBrief, h)
		}
	}
	if prose[1].Page != "decisions/rls" {
		t.Errorf("sentence-final period should not survive: %q", prose[1].Page)
	}

	if got := wikiPages("Read", toolInput{FilePath: "/Users/demo/work/wiki/a.md"}, "", home); got != nil {
		t.Errorf("no wiki root configured should yield nothing: %+v", got)
	}
}

// The wiki is written to as well as read from — a run that captures a decision
// is the other half of the loop, and reads nothing to do it.
func TestWikiWriteIsDistinctFromRead(t *testing.T) {
	const root, home = "/Users/demo/work/wiki", "/Users/demo"
	in := toolInput{FilePath: "/Users/demo/work/wiki/concepts/sse-auth.md"}

	if got := wikiPages("Write", in, root, home); len(got) != 1 || got[0].Via != ViaWrote {
		t.Errorf("Write: want via=%s, got %+v", ViaWrote, got)
	}
	if got := wikiPages("Edit", in, root, home); len(got) != 1 || got[0].Via != ViaWrote {
		t.Errorf("Edit: want via=%s, got %+v", ViaWrote, got)
	}
	if got := wikiPages("Read", in, root, home); len(got) != 1 || got[0].Via != ViaRead {
		t.Errorf("Read: want via=%s, got %+v", ViaRead, got)
	}
}
