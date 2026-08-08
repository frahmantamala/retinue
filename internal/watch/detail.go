package watch

import (
	"path/filepath"
	"regexp"
	"strings"
)

const detailMax = 84

// toolDetail turns a tool call into the one line a person watching over your
// shoulder would say out loud. Most tools already carry a written description;
// the rest get their most identifying argument.
func toolDetail(tool string, in toolInput) string {
	switch tool {
	case "Read", "Write", "NotebookEdit":
		return shorten(tailPath(in.FilePath, 2))
	case "Edit":
		return shorten(tailPath(in.FilePath, 2))
	case "Grep":
		return shorten(strings.TrimSpace(in.Pattern + inPath(in.Path)))
	case "Glob":
		return shorten(strings.TrimSpace(in.Pattern + inPath(in.Path)))
	case "WebFetch":
		return shorten(in.URL)
	case "WebSearch":
		return shorten(in.Query)
	case "Agent", "Task":
		return shorten(firstNonEmpty(in.Description, in.Name))
	case "Bash":
		return shorten(firstNonEmpty(in.Description, in.Command))
	}
	return shorten(firstNonEmpty(in.Description, tailPath(in.FilePath, 2), in.Pattern, in.Query))
}

func inPath(p string) string {
	if p == "" {
		return ""
	}
	return " in " + tailPath(p, 1)
}

func tailPath(p string, segments int) string {
	if p == "" {
		return ""
	}
	parts := strings.Split(filepath.Clean(p), string(filepath.Separator))
	if len(parts) <= segments {
		return p
	}
	return strings.Join(parts[len(parts)-segments:], "/")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func shorten(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= detailMax {
		return s
	}
	return s[:detailMax-1] + "…"
}

// Knowledge provenance.

const (
	ViaRead  = "read"  // the agent opened the page itself
	ViaBrief = "brief" // the lead cited the page in a brief it wrote
	ViaWrote = "wrote" // the run put something back into the wiki
)

// writeTools produce knowledge rather than consume it.
var writeTools = map[string]bool{"Write": true, "Edit": true, "NotebookEdit": true}

type knowledgeHit struct {
	Page string
	Via  string
}

var wikiLink = regexp.MustCompile(`\[\[([^\[\]|]{1,120})\]\]`)

// wikiPages reports which knowledge pages a tool call touched. Two ways count:
// a path under the wiki root (the agent read it), and a [[wikilink]] inside a
// prompt (the lead injected a settled decision into a brief). The second is the
// one that says whether the wiki actually reached the crew.
func wikiPages(tool string, in toolInput, root, home string) []knowledgeHit {
	if root == "" {
		return nil
	}
	root = strings.TrimSuffix(expandHome(root, home), string(filepath.Separator))

	seen := map[string]bool{}
	var out []knowledgeHit
	add := func(page, via string) {
		if page == "" || seen[page] {
			return
		}
		seen[page] = true
		out = append(out, knowledgeHit{Page: page, Via: via})
	}

	direct := ViaRead
	if writeTools[tool] {
		direct = ViaWrote
	}
	for _, p := range []string{in.FilePath, in.Path} {
		add(wikiPageFor(p, root, home), direct)
	}
	// A shell command mentions the wiki as one token among many (rg, cat, ls).
	for _, tok := range pathTokens(in.Command) {
		add(wikiPageFor(tok, root, home), ViaRead)
	}
	// A brief cites knowledge two ways: the wiki's own [[link]] convention, and
	// a plain path in a "READ FIRST" line. Both mean the lead put the page in
	// front of an agent, which is the claim worth making.
	for _, m := range wikiLink.FindAllStringSubmatch(in.Prompt, -1) {
		add(strings.TrimSuffix(strings.TrimSpace(m[1]), ".md"), ViaBrief)
	}
	for _, tok := range pathTokens(in.Prompt) {
		add(wikiPageFor(tok, root, home), ViaBrief)
	}
	return out
}

// pathTokens splits prose or a shell command into candidate paths, trimming the
// punctuation that trails a path in a sentence.
func pathTokens(s string) []string {
	if s == "" {
		return nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '"' || r == '\'' ||
			r == ';' || r == '|' || r == '(' || r == ')' || r == '`' || r == ','
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		// Trailing only — a leading dot is part of the path ("./wiki/x.md").
		out = append(out, strings.TrimRight(f, ".:"))
	}
	return out
}

// wikiPageFor returns the page name for a path inside the wiki, or "" when the
// path lies outside it.
func wikiPageFor(p, root, home string) string {
	if p == "" || root == "" {
		return ""
	}
	clean := filepath.Clean(expandHome(p, home))
	if clean != root && !strings.HasPrefix(clean, root+string(filepath.Separator)) {
		return ""
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(clean, root), string(filepath.Separator))
	if rel == "" {
		return ""
	}
	return strings.TrimSuffix(rel, ".md")
}

func expandHome(p, home string) string {
	if home == "" || !strings.HasPrefix(p, "~") {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}
