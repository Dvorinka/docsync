// Package checkers holds the drift detectors. Each is a pure function over
// the repo root and returns findings — no shared state.
package checkers

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Dvorinka/docsync/internal/find"
	"github.com/Dvorinka/docsync/internal/fsx"
	"github.com/Dvorinka/docsync/internal/parsers"
)

// codeRef records where an env var is referenced in code.
type codeRef struct {
	File string
	Line int
}

func scanFileLines(abs string, fn func(lineNo int, line string)) {
	f, err := os.Open(abs)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	n := 0
	for sc.Scan() {
		n++
		fn(n, sc.Text())
	}
}

var envPatterns = map[string][]*regexp.Regexp{
	"go": {
		regexp.MustCompile(`os\.Getenv\(\s*"([A-Z_][A-Z0-9_]*)"`),
		regexp.MustCompile(`os\.LookupEnv\(\s*"([A-Z_][A-Z0-9_]*)"`),
		regexp.MustCompile("os\\.Getenv\\(\\s*`([A-Z_][A-Z0-9_]*)`"),
		regexp.MustCompile("`env:\"([A-Z_][A-Z0-9_]*)"), // envconfig-style struct tags
	},
	"typescript": {
		regexp.MustCompile(`process\.env\.([A-Z_][A-Z0-9_]*)`),
		regexp.MustCompile(`process\.env\[\s*["']([A-Z_][A-Z0-9_]*)["']\s*\]`),
		regexp.MustCompile(`import\.meta\.env\.([A-Z_][A-Z0-9_]*)`),
	},
	"bash": {
		regexp.MustCompile(`\$([A-Z_][A-Z0-9_]*)\b`),
		regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`),
	},
	// compose files and CI workflows carry env references too:
	// environment: KEY: x / - KEY=x, ${KEY:-def}, ${{ secrets.KEY }}
	"yaml": {
		regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)(?::[^}]*)?\}`),
		regexp.MustCompile(`^\s*-?\s*"?([A-Z_][A-Z0-9_]*)\s*[:=]`),
		regexp.MustCompile(`\$\{\{\s*(?:secrets|env|vars)\.([A-Z_][A-Z0-9_]*)`),
	},
}

// bashLocalAssignRe finds plain assignments `VAR=value`. A var that is only
// ever unconditionally assigned inside a script is a local, not an env
// input — `VAR=${VAR:-default}` still counts as a real env read.
var bashLocalAssignRe = regexp.MustCompile(`^\s*(export\s+)?([A-Z_][A-Z0-9_]*)\s*=`)

// bashFileLocals returns vars assigned unconditionally (RHS doesn't read
// the var itself) and not exported — locals that shouldn't count as refs.
func bashFileLocals(abs string) map[string]bool {
	locals := map[string]bool{}
	exported := map[string]bool{}
	scanFileLines(abs, func(_ int, line string) {
		m := bashLocalAssignRe.FindStringSubmatch(line)
		if m == nil {
			return
		}
		export := m[1] != ""
		key := m[2]
		rhs := line[strings.Index(line, "=")+1:]
		selfRef := strings.Contains(rhs, "$"+key) || strings.Contains(rhs, "${"+key)
		if export {
			exported[key] = true
			return
		}
		if !selfRef {
			locals[key] = true
		}
	})
	for k := range exported {
		delete(locals, k)
	}
	return locals
}

var bashExtOrName = map[string]bool{
	".sh": true, ".bash": true, ".mk": true,
	"Makefile": true, "makefile": true, "Dockerfile": true,
}

var shellBuiltins = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "SHELL": true, "PWD": true,
	"TMPDIR": true, "LANG": true, "TERM": true, "EDITOR": true, "PAGER": true,
	"HOSTNAME": true, "DISPLAY": true, "OLDPWD": true, "SHLVL": true,
	"UID": true, "EUID": true, "IFS": true, "OPTIND": true, "OPTARG": true,
	"PS1": true, "PS2": true, "PS3": true, "PS4": true, "RANDOM": true,
	"SECONDS": true, "LINENO": true, "REPLY": true, "LOGNAME": true,
	"CI": true, "CDPATH": true, "ENV": true, "BASH_VERSION": true,
	"ZSH_VERSION": true, "OSTYPE": true, "MACHTYPE": true, "PIPESTATUS": true,
}

func fileLang(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	base := filepath.Base(path)
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "typescript"
	}
	if bashExtOrName[ext] || bashExtOrName[base] || strings.HasPrefix(base, "Dockerfile") {
		return "bash"
	}
	if ext == ".yml" || ext == ".yaml" {
		// Only compose-family files feed the env contract; workflow yaml
		// env:/secrets refs are CI-scoped, not .env.example-scoped.
		if strings.Contains(strings.ToLower(base), "compose") ||
			strings.HasPrefix(base, ".env") {
			return "yaml"
		}
	}
	return ""
}

// scanEnvRefs walks codeDirs and collects env var references: key -> refs.
func scanEnvRefs(root string, codeDirs, languages []string) map[string][]codeRef {
	enabled := map[string]bool{}
	for _, l := range languages {
		enabled[strings.ToLower(l)] = true
	}
	refs := map[string][]codeRef{}
	for _, dir := range codeDirs {
		abs := filepath.Join(root, dir)
		fsx.WalkFiles(abs, func(rel string) {
			lang := fileLang(rel)
			if lang == "" || !enabled[lang] {
				return
			}
			fileRel := rel
			if dir != "." && dir != "" {
				fileRel = filepath.ToSlash(filepath.Join(dir, rel))
			}
			var locals map[string]bool
			if lang == "bash" {
				locals = bashFileLocals(filepath.Join(root, fileRel))
			}
			scanFileLines(filepath.Join(root, fileRel), func(n int, line string) {
				for _, re := range envPatterns[lang] {
					for _, m := range re.FindAllStringSubmatch(line, -1) {
						key := m[1]
						if lang == "bash" && shellBuiltins[key] {
							continue
						}
						if lang == "bash" && locals[key] {
							continue
						}
						refs[key] = append(refs[key], codeRef{File: fileRel, Line: n})
					}
				}
			})
		})
	}
	return refs
}

// Env cross-references the env example file against env var usage in code.
// Missing-in-example is critical; orphaned example keys are warnings.
func Env(root, envFile string, codeDirs, languages []string, sev map[string]string) []find.Finding {
	keys, err := parsers.EnvFileKeys(filepath.Join(root, envFile))
	if err != nil {
		return nil // caller warns when the env file is absent
	}
	declared := map[string]bool{}
	for _, k := range keys {
		declared[k] = true
	}
	refs := scanEnvRefs(root, codeDirs, languages)

	var out []find.Finding
	for key, rs := range refs {
		if declared[key] {
			continue
		}
		sort.Slice(rs, func(i, j int) bool {
			if rs[i].File != rs[j].File {
				return rs[i].File < rs[j].File
			}
			return rs[i].Line < rs[j].Line
		})
		first := rs[0]
		out = append(out, find.New("env_missing_in_example", key,
			fmt.Sprintf("referenced in %s:%d but missing from %s", first.File, first.Line, envFile),
			first.File, first.Line, sev))
	}
	for _, k := range keys {
		if _, used := refs[k]; !used {
			out = append(out, find.New("env_orphan_in_example", k,
				fmt.Sprintf("declared in %s but not referenced in any scanned code", envFile),
				envFile, 0, sev))
		}
	}
	return out
}
