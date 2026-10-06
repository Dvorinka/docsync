package internal

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Dvorinka/docsync/internal/checkers"
	"github.com/Dvorinka/docsync/internal/find"
	"github.com/Dvorinka/docsync/internal/fsx"
	"github.com/Dvorinka/docsync/internal/parsers"
)

// Result is the full check run output.
type Result struct {
	Findings []find.Finding `json:"findings"`
	Summary  find.Summary   `json:"summary"`
	Warnings []string       `json:"warnings,omitempty"`
}

// RunCheck executes all drift checkers against a repo root.
func RunCheck(root string, cfg Config) (Result, error) {
	warnings := []string{}
	findings := []find.Finding{}

	// Parse docs.
	var docs []parsers.Doc
	var docFiles []string
	for _, pattern := range cfg.Docs {
		docFiles = append(docFiles, fsx.GlobRoot(root, pattern)...)
	}
	docFiles = dedupStrings(docFiles)
	for _, rel := range docFiles {
		d, err := parsers.ParseMarkdown(filepath.Join(root, rel), rel)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("cannot read %s — skipped", rel))
			continue
		}
		docs = append(docs, d)
	}
	if len(docs) == 0 {
		warnings = append(warnings, "no docs found — skipping doc checks")
	}

	// Env checker (skipped silently if no env file).
	if _, err := os.Stat(filepath.Join(root, cfg.EnvFile)); err == nil {
		findings = append(findings, checkers.Env(root, cfg.EnvFile, cfg.CodeDirs, cfg.Languages, cfg.Severity)...)
	} else {
		warnings = append(warnings, fmt.Sprintf("no %s found — skipping env checks", cfg.EnvFile))
	}

	findings = append(findings, checkers.Paths(root, docs, cfg.Severity)...)
	findings = append(findings, checkers.Commands(root, docs, cfg.Severity)...)
	findings = append(findings, checkers.Routes(root, docs, cfg.APIRoutes, cfg.Severity)...)
	findings = append(findings, checkers.Versions(root, docs, cfg.DependencyFiles, cfg.Severity)...)
	findings = append(findings, checkers.Pins(root, cfg.DependencyFiles, cfg.Severity)...)

	findings = filterIgnored(findings, cfg.Ignore)
	find.Sort(findings)
	return Result{Findings: findings, Summary: find.Summarize(findings), Warnings: warnings}, nil
}

func dedupStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// filterIgnored drops findings whose key matches an ignore pattern
// (exact, single-star glob, or doublestar).
func filterIgnored(fs []find.Finding, ignore []string) []find.Finding {
	if len(ignore) == 0 {
		return fs
	}
	var out []find.Finding
	for _, f := range fs {
		skip := false
		for _, pat := range ignore {
			if pat == f.Key || fsx.MatchGlob(pat, f.Key) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, f)
		}
	}
	return out
}

// ExitCode maps findings to the docsync exit-code contract:
// 0 clean, 1 warnings only, 2 critical findings.
func ExitCode(res Result) int {
	if res.Summary.Critical > 0 {
		return 2
	}
	if res.Summary.Warning > 0 {
		return 1
	}
	return 0
}

// MissingEnvKeys returns the env_missing_in_example keys from a result —
// the fixable subset.
func MissingEnvKeys(res Result) []string {
	var out []string
	for _, f := range res.Findings {
		if f.Category == "env_missing_in_example" {
			out = append(out, f.Key)
		}
	}
	return out
}
