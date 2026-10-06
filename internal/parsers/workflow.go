package parsers

import (
	"bufio"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// VersionPin is a declared tool version from a dependency/workflow/build file.
type VersionPin struct {
	Tool    string // node, go, python
	Version string // raw value, e.g. "22", "1.23", ">=22"
	File    string
	Line    int // 0 when unknown (YAML values)
}

// PackageJSON holds the fields docsync needs from a package.json.
type PackageJSON struct {
	Scripts map[string]string
	Engines map[string]string
}

// ParsePackageJSON extracts scripts and engines from a package.json.
func ParsePackageJSON(path string) (PackageJSON, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return PackageJSON{}, err
	}
	var raw struct {
		Scripts map[string]string `json:"scripts"`
		Engines map[string]string `json:"engines"`
		DevDeps map[string]string `json:"devDependencies"`
		Deps    map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return PackageJSON{}, err
	}
	p := PackageJSON{Scripts: raw.Scripts, Engines: raw.Engines}
	if p.Scripts == nil {
		p.Scripts = map[string]string{}
	}
	return p, nil
}

// PackageJSONDeps returns dependency names (deps + devDeps) from package.json.
func PackageJSONDeps(path string) map[string]bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw struct {
		DevDeps map[string]string `json:"devDependencies"`
		Deps    map[string]string `json:"dependencies"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return nil
	}
	out := map[string]bool{}
	for k := range raw.Deps {
		out[k] = true
	}
	for k := range raw.DevDeps {
		out[k] = true
	}
	return out
}

// WorkflowVersions extracts tool version pins from a GitHub Actions workflow
// file: *-version keys anywhere in the tree, including matrix arrays.
func WorkflowVersions(path, rel string) []VersionPin {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc any
	if yaml.Unmarshal(b, &doc) != nil {
		return nil
	}
	var pins []VersionPin
	var walk func(n any)
	walk = func(n any) {
		switch t := n.(type) {
		case map[string]any:
			for k, v := range t {
				tool, isVer := versionKeyTool(k)
				if isVer {
					switch vv := v.(type) {
					case string:
						pins = append(pins, VersionPin{Tool: tool, Version: vv, File: rel})
					case int:
						pins = append(pins, VersionPin{Tool: tool, Version: strconv.Itoa(vv), File: rel})
					case float64:
						pins = append(pins, VersionPin{Tool: tool, Version: strconv.FormatFloat(vv, 'f', -1, 64), File: rel})
					case []any:
						for _, item := range vv {
							switch s := item.(type) {
							case string:
								pins = append(pins, VersionPin{Tool: tool, Version: s, File: rel})
							case int:
								pins = append(pins, VersionPin{Tool: tool, Version: strconv.Itoa(s), File: rel})
							}
						}
					}
					continue
				}
				walk(v)
			}
		case []any:
			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(doc)
	return pins
}

func versionKeyTool(k string) (string, bool) {
	switch strings.ToLower(k) {
	case "node-version", "node_version":
		return "node", true
	case "go-version", "go_version":
		return "go", true
	case "python-version", "python_version":
		return "python", true
	}
	return "", false
}

var goModRe = regexp.MustCompile(`(?m)^go\s+(\d+\.\d+(?:\.\d+)?)\s*$`)

// GoModVersion extracts the `go` directive from a go.mod file.
func GoModVersion(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if m := goModRe.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}

// MakefileTargets returns rule target names from a Makefile.
func MakefileTargets(path string) map[string]bool {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]bool{}
	re := regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*:(?:\s|$)`)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, ".") {
			continue
		}
		if m := re.FindStringSubmatch(line); m != nil {
			out[m[1]] = true
		}
	}
	return out
}

// JustfileRecipes returns recipe names from a justfile.
func JustfileRecipes(path string) map[string]bool {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]bool{}
	re := regexp.MustCompile(`^@?([A-Za-z0-9_-]+)(?:\s+[^:=]*)?\s*:(?:\s|$)`)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") ||
			strings.HasPrefix(line, "#") {
			continue
		}
		if m := re.FindStringSubmatch(line); m != nil {
			out[m[1]] = true
		}
	}
	return out
}
