package checkers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Dvorinka/docsync/internal/find"
	"github.com/Dvorinka/docsync/internal/parsers"
)

// createTargetRe matches lines that produce the referenced file — the doc
// instructs the reader to create it, so its absence is not drift:
// `cp .env.example .env`, `touch x`, `mkdir -p a/b`, `> out.log`.
var createTargetRe = regexp.MustCompile(`\b(cp|mv|touch|mkdir|install|ln|tee)\b|>>?\s*\S*$`)

// Paths verifies that file paths referenced in docs resolve on disk.
// Code-span paths resolve against the repo root; markdown links/images also
// resolve relative to the document's directory. strict skips the namespace
// heuristic — set it when docs must reference only real top-level dirs.
func Paths(root string, docs []parsers.Doc, sev map[string]string, strict bool) []find.Finding {
	var out []find.Finding
	seen := map[string]bool{}
	lineCache := map[string][]string{}
	for _, doc := range docs {
		docDir := filepath.Dir(doc.Path)
		for _, sp := range doc.Paths {
			p := strings.TrimSpace(sp.Text)
			p = strings.TrimSuffix(p, "/")
			if p == "" {
				continue
			}
			// Extensionless multi-segment strings whose first segment is
			// not a real repo dir are namespaces, not paths (x/crypto/…,
			// tools/call, org/image) — unless strict mode is on.
			if !strict && filepath.Ext(filepath.Base(p)) == "" && strings.Contains(p, "/") {
				first := strings.SplitN(p, "/", 2)[0]
				if st, err := os.Stat(filepath.Join(root, first)); err != nil || !st.IsDir() {
					continue
				}
			}
			exists := fileExists(filepath.Join(root, p))
			if !exists && docDir != "." {
				// Links/images resolve relative to the doc first.
				exists = fileExists(filepath.Join(root, docDir, p))
			}
			if exists {
				continue
			}
			// Skip targets of create commands on the same line.
			if isCreateTarget(root, doc.Path, sp.Line, p, lineCache) {
				continue
			}
			dedup := p + "|" + doc.Path
			if seen[dedup] {
				continue
			}
			seen[dedup] = true
			out = append(out, find.New("path_not_found", p,
				fmt.Sprintf("referenced in %s:%d but file does not exist", doc.Path, sp.Line),
				doc.Path, sp.Line, sev))
		}
	}
	return out
}

// isCreateTarget reports whether the doc line that mentions p is actually a
// command that would create p — `cp x p`, `touch p`, `> p`.
func isCreateTarget(root, docPath string, line int, p string, cache map[string][]string) bool {
	lines, ok := cache[docPath]
	if !ok {
		b, err := os.ReadFile(filepath.Join(root, docPath))
		if err != nil {
			return false
		}
		lines = strings.Split(string(b), "\n")
		cache[docPath] = lines
	}
	if line < 1 || line > len(lines) {
		return false
	}
	l := lines[line-1]
	idx := strings.Index(l, p)
	if idx < 0 {
		return false
	}
	return createTargetRe.MatchString(l[:idx+len(p)])
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
