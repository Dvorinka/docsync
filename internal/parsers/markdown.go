package parsers

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// Span is a text fragment with its 1-based line number.
type Span struct {
	Text string
	Line int
}

// RouteRef is a documented API route like "GET /api/health".
type RouteRef struct {
	Method string
	Path   string
	Line   int
}

// VersionRef is a documented version assertion like "Requires Node 24".
type VersionRef struct {
	Tool    string // normalized: go, node, python
	Version string
	Raw     string
	Line    int
}

// Doc holds every codebase-facing reference extracted from one markdown file.
type Doc struct {
	Path     string // repo-relative path of the markdown file
	Commands []Span
	Paths    []Span
	Routes   []RouteRef
	Versions []VersionRef
}

var (
	fenceRe    = regexp.MustCompile("^\\s*(```|~~~)\\s*([A-Za-z0-9_-]*)")
	codeSpanRe = regexp.MustCompile("`([^`\n]+)`")
	linkRe     = regexp.MustCompile(`!?\[[^\]]*\]\(([^)\s]+)[^)]*\)`)
	imgSrcRe   = regexp.MustCompile(`<img[^>]+src="([^"]+)"`)
	routeRe    = regexp.MustCompile(`(?i)\b(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s+(/[A-Za-z0-9_\-/{}.:%]+)`)
	versionRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\brequires?\s+(go|node(?:\.?js)?|python)\s+v?(\d+(?:\.\d+){0,2})\+?`),
		regexp.MustCompile(`\b(Go|Node(?:\.js)?|Python)\s*>=\s*v?(\d+(?:\.\d+){0,2})\+?`),
		regexp.MustCompile(`\b(Go|Node(?:\.js)?|Python)\s+v?(\d+\.\d+\.\d+)\+?\b`),
	}
	cmdStartRe    = regexp.MustCompile(`^(?:sudo\s+)?(?:env\s+\w+=\S+\s+)*(?:npm|npx|yarn|pnpm|bun|deno|go|make|cargo|just|docker-compose|docker|git|pip3?|python3?|node|pytest|\./|sh\s|bash\s)`)
	versionOnlyRe = regexp.MustCompile(`^v?\d+(\.\d+)*\+?$`)
	fileExtRe     = regexp.MustCompile(`\.([A-Za-z0-9]+)$`)
	allCapsSegRe  = regexp.MustCompile(`^[A-Z0-9_]+$`)
)

var fileExts = map[string]bool{
	"go": true, "ts": true, "tsx": true, "js": true, "jsx": true, "mjs": true,
	"cjs": true, "mts": true, "cts": true, "json": true, "yml": true, "yaml": true,
	"toml": true, "md": true, "markdown": true, "sh": true, "bash": true,
	"zsh": true, "env": true, "example": true, "mod": true, "sum": true,
	"lock": true, "css": true, "html": true, "htm": true, "sql": true,
	"py": true, "rs": true, "java": true, "kt": true, "kts": true,
	"swift": true, "rb": true, "c": true, "h": true, "cpp": true, "hpp": true,
	"cs": true, "xml": true, "txt": true, "cfg": true, "ini": true, "conf": true,
	"gradle": true, "properties": true, "plist": true, "pbxproj": true,
	"png": true, "jpg": true, "jpeg": true, "svg": true, "gif": true,
	"webp": true, "ico": true, "pem": true, "key": true, "crt": true,
	"log": true, "dockerfile": true, "mk": true, "sqlx": true, "proto": true,
}

var knownFiles = map[string]bool{
	"Makefile": true, "makefile": true, "Dockerfile": true, "dockerfile": true,
	"justfile": true, "Justfile": true, ".env": true, ".env.example": true,
	"go.mod": true, "go.sum": true, "Cargo.toml": true, "package.json": true,
	"docker-compose.yml": true, "docker-compose.yaml": true, "compose.yml": true,
	"compose.yaml": true, "Taskfile.yml": true, "LICENSE": true, "Procfile": true,
}

// LooksLikePath reports whether s plausibly names a repo file.
func LooksLikePath(s string) bool {
	if len(s) < 3 || len(s) > 300 {
		return false
	}
	if strings.ContainsAny(s, " `'\"") || strings.ContainsAny(s, "$*{}[]()<>|&;!+") {
		return false
	}
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "~") || strings.HasPrefix(s, "/") {
		// leading / is a URL/API path or unverifiable absolute host path
		return false
	}
	if strings.Contains(s, "://") || strings.Contains(s, "@") || strings.Contains(s, "#") {
		return false
	}
	if versionOnlyRe.MatchString(s) {
		return false
	}
	if strings.Contains(s, "/") {
		segs := strings.Split(s, "/")
		first := segs[0]
		// host.tld/path is a URL fragment, not a repo path — but keep
		// hidden dirs like .github/.
		if strings.Contains(first, ".") && !strings.HasPrefix(first, ".") {
			return false
		}
		// ALLCAPS/ALLCAPS combos (LISTEN/NOTIFY, GET/POST) aren't paths.
		allCaps := true
		for _, seg := range segs {
			if !allCapsSegRe.MatchString(seg) {
				allCaps = false
				break
			}
		}
		if allCaps {
			return false
		}
		return true
	}
	base := s
	if i := strings.LastIndex(s, "/"); i >= 0 {
		base = s[i+1:]
	}
	if knownFiles[base] {
		return true
	}
	if m := fileExtRe.FindStringSubmatch(base); m != nil {
		return fileExts[strings.ToLower(m[1])]
	}
	return false
}

// LooksLikeCommand reports whether s starts with a known tool invocation.
func LooksLikeCommand(s string) bool {
	return cmdStartRe.MatchString(strings.TrimSpace(s))
}

func isURL(s string) bool {
	return strings.Contains(s, "://") || strings.HasPrefix(s, "#") ||
		strings.HasPrefix(s, "mailto:") || strings.HasPrefix(s, "tel:")
}

func normTool(t string) string {
	t = strings.ToLower(strings.ReplaceAll(t, ".", ""))
	switch t {
	case "nodejs", "node":
		return "node"
	case "python", "python3":
		return "python"
	default:
		return t
	}
}

// ParseMarkdown extracts documentation references from a markdown file.
// Line-oriented scanning; no full markdown AST.
func ParseMarkdown(absPath, relPath string) (Doc, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return Doc{}, err
	}
	defer f.Close()

	doc := Doc{Path: relPath}
	inBlock := false
	blockLang := ""
	fenceTok := "```"

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		line := sc.Text()
		lineNo++
		trimmed := strings.TrimSpace(line)

		if m := fenceRe.FindStringSubmatch(line); m != nil {
			if !inBlock {
				inBlock = true
				fenceTok = m[1]
				blockLang = strings.ToLower(m[2])
			} else if strings.HasPrefix(trimmed, fenceTok) {
				inBlock = false
			}
			continue
		}

		// Route and version assertions are extracted everywhere.
		for _, rm := range routeRe.FindAllStringSubmatch(line, -1) {
			p := strings.TrimRight(rm[2], "/")
			if p == "" {
				p = "/"
			}
			doc.Routes = append(doc.Routes, RouteRef{Method: strings.ToUpper(rm[1]), Path: p, Line: lineNo})
		}
		for _, re := range versionRes {
			for _, vm := range re.FindAllStringSubmatch(line, -1) {
				doc.Versions = append(doc.Versions, VersionRef{
					Tool: normTool(vm[1]), Version: vm[2], Raw: vm[0], Line: lineNo,
				})
			}
		}

		if inBlock {
			switch blockLang {
			case "bash", "sh", "shell", "zsh", "console", "", "text":
				cmd := strings.TrimPrefix(trimmed, "$ ")
				cmd = strings.TrimSpace(cmd)
				if cmd != "" && !strings.HasPrefix(cmd, "#") && LooksLikeCommand(cmd) {
					doc.Commands = append(doc.Commands, Span{Text: cmd, Line: lineNo})
					continue
				}
			}
			// Bare path lines inside code blocks (e.g. a tree listing).
			if LooksLikePath(trimmed) && strings.Contains(trimmed, "/") {
				doc.Paths = append(doc.Paths, Span{Text: trimmed, Line: lineNo})
			}
			continue
		}

		// Prose: inline code spans, markdown links, <img src>.
		for _, m := range codeSpanRe.FindAllStringSubmatch(line, -1) {
			s := m[1]
			if LooksLikePath(s) {
				doc.Paths = append(doc.Paths, Span{Text: s, Line: lineNo})
			} else if LooksLikeCommand(s) {
				doc.Commands = append(doc.Commands, Span{Text: s, Line: lineNo})
			}
		}
		for _, m := range linkRe.FindAllStringSubmatch(line, -1) {
			p := strings.SplitN(m[1], "#", 2)[0]
			if p != "" && !isURL(m[1]) {
				doc.Paths = append(doc.Paths, Span{Text: p, Line: lineNo})
			}
		}
		for _, m := range imgSrcRe.FindAllStringSubmatch(line, -1) {
			if !isURL(m[1]) {
				doc.Paths = append(doc.Paths, Span{Text: m[1], Line: lineNo})
			}
		}
	}
	return doc, sc.Err()
}
