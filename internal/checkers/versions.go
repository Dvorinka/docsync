package checkers

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Dvorinka/docsync/internal/find"
	"github.com/Dvorinka/docsync/internal/fsx"
	"github.com/Dvorinka/docsync/internal/parsers"
)

// DeclaredPins collects all tool-version pins from dependency files:
// package.json engines, go.mod, workflow *-version keys.
func DeclaredPins(root string, depFiles []string) []parsers.VersionPin {
	var pins []parsers.VersionPin
	seen := map[string]bool{}
	for _, pattern := range depFiles {
		for _, rel := range fsx.GlobRoot(root, pattern) {
			abs := filepath.Join(root, rel)
			base := filepath.Base(rel)
			var got []parsers.VersionPin
			switch {
			case base == "package.json":
				pkg, err := parsers.ParsePackageJSON(abs)
				if err != nil {
					continue
				}
				for tool, v := range pkg.Engines {
					t := normEngine(tool)
					if t == "" {
						continue
					}
					got = append(got, parsers.VersionPin{Tool: t, Version: v, File: rel})
				}
			case base == "go.mod":
				if v := parsers.GoModVersion(abs); v != "" {
					got = append(got, parsers.VersionPin{Tool: "go", Version: v, File: rel})
				}
			case strings.HasSuffix(rel, ".yml") || strings.HasSuffix(rel, ".yaml"):
				got = append(got, parsers.WorkflowVersions(abs, rel)...)
			}
			for _, p := range got {
				k := p.Tool + "|" + p.Version + "|" + p.File
				if !seen[k] {
					seen[k] = true
					pins = append(pins, p)
				}
			}
		}
	}
	return pins
}

func normEngine(name string) string {
	switch strings.ToLower(name) {
	case "node":
		return "node"
	case "go":
		return "go"
	case "python":
		return "python"
	}
	return ""
}

var numRe = regexp.MustCompile(`\d+(?:\.\d+)*`)

// normVersion reduces a version string to the comparison granularity:
// major for node, major.minor for go and python.
func normVersion(tool, v string) string {
	m := numRe.FindString(v)
	if m == "" {
		return ""
	}
	parts := strings.Split(m, ".")
	switch tool {
	case "node":
		return parts[0]
	default: // go, python — minor releases matter
		if len(parts) >= 2 {
			return parts[0] + "." + parts[1]
		}
		return parts[0]
	}
}

// Versions compares "Requires Node 24"-style doc assertions against
// declared pins in dependency files. One finding per doc assertion.
func Versions(root string, docs []parsers.Doc, depFiles []string, sev map[string]string) []find.Finding {
	pins := DeclaredPins(root, depFiles)
	declared := map[string][]parsers.VersionPin{}
	for _, p := range pins {
		declared[p.Tool] = append(declared[p.Tool], p)
	}
	var out []find.Finding
	seen := map[string]bool{}
	for _, d := range docs {
		for _, v := range d.Versions {
			srcs, known := declared[v.Tool]
			if !known {
				continue // repo doesn't pin this tool anywhere — nothing to compare
			}
			want := normVersion(v.Tool, v.Version)
			match := false
			var descs []string
			seenVer := map[string]bool{}
			for _, p := range srcs {
				nv := normVersion(p.Tool, p.Version)
				if nv == want {
					match = true
				}
				if !seenVer[p.Version+"@"+p.File] {
					seenVer[p.Version+"@"+p.File] = true
					descs = append(descs, fmt.Sprintf("%s (%s)", p.Version, p.File))
				}
			}
			if match {
				continue
			}
			k := v.Tool + "|" + want
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, find.New("version_mismatch", v.Tool,
				fmt.Sprintf("%s:%d says %q but %s declares %s", d.Path, v.Line, v.Raw,
					v.Tool, strings.Join(descs, ", ")),
				d.Path, v.Line, sev))
		}
	}
	return out
}
