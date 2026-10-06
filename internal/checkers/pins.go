package checkers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Dvorinka/docsync/internal/find"
	"github.com/Dvorinka/docsync/internal/fsx"
)

// dockerPin is a FROM image:tag pin found in a Dockerfile.
type dockerPin struct {
	tool     string
	version  string // normalized
	raw      string
	file     string
	line     int
	floating bool
}

var fromRe = regexp.MustCompile(`(?im)^\s*FROM\s+(?:--\S+\s+)*(\S+)`)

var imageTools = map[string]string{
	"node": "node", "nodejs": "node",
	"golang": "go", "go": "go",
	"python": "python",
	"rust":   "rust", "ruby": "ruby", "php": "php",
	"openjdk": "java", "eclipse-temurin": "java",
}

var floatingTags = map[string]bool{
	"latest": true, "lts": true, "edge": true, "main": true, "master": true,
	"current": true, "stable": true, "slim": true, "alpine": true, "bookworm": true,
	"bullseye": true, "trixie": true,
}

func dockerPins(root string) []dockerPin {
	var pins []dockerPin
	fsx.WalkFiles(root, func(rel string) {
		base := filepath.Base(rel)
		if base != "Dockerfile" && !strings.HasPrefix(base, "Dockerfile.") &&
			!strings.HasSuffix(strings.ToLower(base), ".dockerfile") {
			return
		}
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return
		}
		for i, line := range strings.Split(string(b), "\n") {
			m := fromRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			ref := m[1]
			if strings.HasPrefix(ref, "$") || strings.Contains(ref, "${") ||
				strings.EqualFold(ref, "scratch") {
				continue
			}
			name, tag := ref, ""
			if i := strings.LastIndex(ref, ":"); i > 0 {
				name, tag = ref[:i], ref[i+1:]
			}
			short := name
			if j := strings.LastIndex(short, "/"); j >= 0 {
				short = short[j+1:]
			}
			tool, known := imageTools[strings.ToLower(short)]
			if !known {
				continue
			}
			p := dockerPin{tool: tool, raw: ref, file: rel, line: i + 1}
			if tag == "" || floatingTags[strings.ToLower(tag)] {
				p.floating = true
			} else {
				p.version = normVersion(tool, tag)
			}
			pins = append(pins, p)
		}
	})
	return pins
}

// Pins cross-checks build pins that must agree: Dockerfile FROM tags,
// workflow *-version keys, go.mod, package.json engines. Floating tags
// (node:latest) are flagged separately as pin_unpinned.
func Pins(root string, depFiles []string, sev map[string]string) []find.Finding {
	var out []find.Finding

	// versioned pins grouped by tool: tool -> normVersion -> sources
	type src struct {
		file, raw string
		line      int
	}
	byTool := map[string]map[string][]src{}

	add := func(tool, norm, raw, file string, line int) {
		if norm == "" {
			return
		}
		if byTool[tool] == nil {
			byTool[tool] = map[string][]src{}
		}
		byTool[tool][norm] = append(byTool[tool][norm], src{file, raw, line})
	}

	for _, p := range dockerPins(root) {
		if p.floating {
			out = append(out, find.New("pin_unpinned", p.raw,
				fmt.Sprintf("%s:%d uses floating tag %q — pin a version", p.file, p.line, p.raw),
				p.file, p.line, sev))
			continue
		}
		add(p.tool, p.version, p.raw, p.file, p.line)
	}
	for _, p := range DeclaredPins(root, depFiles) {
		add(p.Tool, normVersion(p.Tool, p.Version), p.Version, p.File, p.Line)
	}

	var tools []string
	for t := range byTool {
		if len(byTool[t]) > 1 {
			tools = append(tools, t)
		}
	}
	sort.Strings(tools)
	for _, t := range tools {
		var parts []string
		firstFile, firstLine := "", 0
		var vers []string
		for v := range byTool[t] {
			vers = append(vers, v)
		}
		sort.Strings(vers)
		for _, v := range vers {
			for _, s := range byTool[t][v] {
				loc := s.file
				if s.line > 0 {
					loc = fmt.Sprintf("%s:%d", s.file, s.line)
				}
				parts = append(parts, fmt.Sprintf("%s (%s)", loc, s.raw))
				if firstFile == "" {
					firstFile, firstLine = s.file, s.line
				}
			}
		}
		out = append(out, find.New("pin_mismatch", t,
			fmt.Sprintf("conflicting %s pins: %s", t, strings.Join(parts, " vs ")),
			firstFile, firstLine, sev))
	}
	return out
}
