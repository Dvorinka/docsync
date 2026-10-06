package checkers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Dvorinka/docsync/internal/find"
	"github.com/Dvorinka/docsync/internal/fsx"
	"github.com/Dvorinka/docsync/internal/parsers"
)

// Commands verifies that commands documented in code blocks still resolve:
// npm scripts, make/just targets, script paths, and toolchain presence.
// It checks references resolve — it never executes anything.
func Commands(root string, docs []parsers.Doc, sev map[string]string) []find.Finding {
	st := newCmdState(root)
	st.sev = sev
	var out []find.Finding
	seen := map[string]bool{}
	for _, doc := range docs {
		for _, sp := range doc.Commands {
			f := st.check(root, doc, sp)
			if f == nil {
				continue
			}
			dedup := f.Category + "|" + f.Key + "|" + f.File
			if seen[dedup] {
				continue
			}
			seen[dedup] = true
			out = append(out, *f)
		}
	}
	return out
}

type cmdState struct {
	pkgJSONs    []pkgInfo // every package.json in the repo
	makefiles   []string
	makeTargets map[string]bool
	justfiles   []string
	justRecipes map[string]bool
	goMod       bool
	cargoToml   bool
	loaded      bool
	root        string
	sev         map[string]string
}

type pkgInfo struct {
	rel  string
	pkg  parsers.PackageJSON
	deps map[string]bool
}

func newCmdState(root string) *cmdState { return &cmdState{root: root} }

func (s *cmdState) load() {
	if s.loaded {
		return
	}
	s.loaded = true
	s.makeTargets = map[string]bool{}
	s.justRecipes = map[string]bool{}
	fsx.WalkFiles(s.root, func(rel string) {
		base := filepath.Base(rel)
		switch {
		case base == "package.json":
			pkg, err := parsers.ParsePackageJSON(filepath.Join(s.root, rel))
			if err == nil {
				s.pkgJSONs = append(s.pkgJSONs, pkgInfo{rel: rel, pkg: pkg,
					deps: parsers.PackageJSONDeps(filepath.Join(s.root, rel))})
			}
		case base == "Makefile" || base == "makefile" || strings.HasSuffix(base, ".mk"):
			s.makefiles = append(s.makefiles, rel)
			for t := range parsers.MakefileTargets(filepath.Join(s.root, rel)) {
				s.makeTargets[t] = true
			}
		case base == "justfile" || base == "Justfile" || base == ".justfile":
			s.justfiles = append(s.justfiles, rel)
			for t := range parsers.JustfileRecipes(filepath.Join(s.root, rel)) {
				s.justRecipes[t] = true
			}
		case base == "go.mod":
			s.goMod = true
		case base == "Cargo.toml":
			s.cargoToml = true
		}
	})
}

var npmScriptRe = regexp.MustCompile(`^(?:npm|pnpm|yarn|bun)\s+(?:run|run-script)\s+([A-Za-z0-9_:-]+)`)
var shellScriptRe = regexp.MustCompile(`^(?:sh|bash|node|python3?)\s+(\S+\.[A-Za-z]+)`)

func (s *cmdState) check(root string, doc parsers.Doc, sp parsers.Span) *find.Finding {
	s.load()
	fields := strings.Fields(sp.Text)
	if len(fields) == 0 {
		return nil
	}
	// strip env assignments and sudo
	for len(fields) > 0 && (fields[0] == "sudo" || strings.Contains(fields[0], "=")) {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return nil
	}
	cmd := fields[0]
	ref := func(key, detail string) *find.Finding {
		f := find.New("command_not_found", key, detail, doc.Path, sp.Line, s.sev)
		return &f
	}

	switch cmd {
	case "npm", "pnpm", "yarn", "bun":
		m := npmScriptRe.FindStringSubmatch(sp.Text)
		if m == nil {
			return nil // npm install/test builtin etc — not verifiable
		}
		script := m[1]
		if len(s.pkgJSONs) == 0 {
			return nil // no package.json anywhere — can't verify
		}
		for _, p := range s.pkgJSONs {
			if _, ok := p.pkg.Scripts[script]; ok {
				return nil
			}
		}
		return ref(cmd+" run "+script,
			fmt.Sprintf("referenced in %s:%d but script %q not found in any package.json",
				doc.Path, sp.Line, script))
	case "npx":
		if len(fields) < 2 {
			return nil
		}
		tool := strings.TrimLeft(fields[1], "-")
		if tool == "" || len(s.pkgJSONs) == 0 {
			return nil // nothing to verify against — skip
		}
		for _, p := range s.pkgJSONs {
			if p.deps[tool] {
				return nil
			}
			bin := filepath.Join(s.root, filepath.Dir(p.rel), "node_modules", ".bin", tool)
			if _, err := os.Stat(bin); err == nil {
				return nil
			}
		}
		f := ref("npx "+tool,
			fmt.Sprintf("referenced in %s:%d but %q is not a dependency or installed binary",
				doc.Path, sp.Line, tool))
		f.Severity = "warning" // npx can fetch on demand — never critical
		return f
	case "go":
		if s.goMod {
			return nil
		}
		return ref(sp.Text, fmt.Sprintf("referenced in %s:%d but no go.mod exists in this repo", doc.Path, sp.Line))
	case "cargo":
		if s.cargoToml {
			return nil
		}
		return ref(sp.Text, fmt.Sprintf("referenced in %s:%d but no Cargo.toml exists in this repo", doc.Path, sp.Line))
	case "make":
		if len(s.makefiles) == 0 {
			return ref(sp.Text, fmt.Sprintf("referenced in %s:%d but no Makefile exists", doc.Path, sp.Line))
		}
		if len(fields) < 2 {
			return nil
		}
		target := fields[1]
		if s.makeTargets[target] {
			return nil
		}
		return ref("make "+target,
			fmt.Sprintf("referenced in %s:%d but target %q not found in %s",
				doc.Path, sp.Line, target, strings.Join(s.makefiles, ", ")))
	case "just":
		if len(s.justfiles) == 0 {
			return ref(sp.Text, fmt.Sprintf("referenced in %s:%d but no justfile exists", doc.Path, sp.Line))
		}
		if len(fields) < 2 {
			return nil
		}
		if s.justRecipes[fields[1]] {
			return nil
		}
		return ref("just "+fields[1],
			fmt.Sprintf("referenced in %s:%d but recipe %q not found in %s",
				doc.Path, sp.Line, fields[1], strings.Join(s.justfiles, ", ")))
	}

	// ./script.sh and "sh script.sh" style invocations
	var scriptPath string
	if strings.HasPrefix(cmd, "./") || strings.HasPrefix(cmd, "../") {
		scriptPath = cmd
	} else if m := shellScriptRe.FindStringSubmatch(sp.Text); m != nil &&
		(cmd == "sh" || cmd == "bash" || cmd == "node" || cmd == "python" || cmd == "python3") {
		scriptPath = m[1]
	}
	if scriptPath != "" && parsers.LooksLikePath(scriptPath) {
		clean := strings.TrimPrefix(scriptPath, "./")
		docDir := filepath.Dir(doc.Path)
		if fileExists(filepath.Join(root, clean)) ||
			fileExists(filepath.Join(root, docDir, clean)) {
			return nil
		}
		return ref(scriptPath,
			fmt.Sprintf("referenced in %s:%d but script does not exist", doc.Path, sp.Line))
	}
	return nil
}
