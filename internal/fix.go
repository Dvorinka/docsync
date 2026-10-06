package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Dvorinka/docsync/internal/find"
)

// FixResult is the JSON contract for `docsync fix`.
type FixResult struct {
	Fixed   []FixAction    `json:"fixed"`
	Unfixed []find.Finding `json:"unfixed"`
	DryRun  bool           `json:"dry_run"`
}

// FixAction describes one applied (or planned) fix.
type FixAction struct {
	Action string `json:"action"`
	Key    string `json:"key"`
	File   string `json:"file"`
	Line   int    `json:"line"`
}

// Fix applies the safe auto-fixes: appends missing env keys to the env
// example file. Markdown is never modified. DryRun reports without writing.
func Fix(root string, cfg Config, dryRun bool) (FixResult, Result, error) {
	res, err := RunCheck(root, cfg)
	if err != nil {
		return FixResult{}, res, err
	}
	fr := FixResult{DryRun: dryRun, Fixed: []FixAction{}, Unfixed: []find.Finding{}}
	envPath := filepath.Join(root, cfg.EnvFile)

	missing := MissingEnvKeys(res)
	if len(missing) > 0 {
		existing, _ := os.ReadFile(envPath)
		lineNo := 1
		for _, l := range strings.Split(string(existing), "\n") {
			if strings.TrimSpace(l) != "" {
				lineNo++
			}
		}
		if dryRun {
			for _, k := range missing {
				fr.Fixed = append(fr.Fixed, FixAction{Action: "env_added", Key: k, File: cfg.EnvFile, Line: lineNo})
				lineNo += 2
			}
		} else {
			var sb strings.Builder
			sb.Write(existing)
			if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
				sb.WriteString("\n")
			}
			for _, k := range missing {
				sb.WriteString(fmt.Sprintf("%s= # TODO: document this var\n", k))
				fr.Fixed = append(fr.Fixed, FixAction{Action: "env_added", Key: k, File: cfg.EnvFile, Line: lineNo})
				lineNo++
			}
			if err := os.WriteFile(envPath, []byte(sb.String()), 0o644); err != nil {
				return fr, res, fmt.Errorf("cannot write %s: %w", cfg.EnvFile, err)
			}
		}
	}
	for _, f := range res.Findings {
		if f.Category != "env_missing_in_example" {
			f.Detail = fmt.Sprintf("requires manual fix — %s", f.Detail)
			fr.Unfixed = append(fr.Unfixed, f)
		}
	}
	return fr, res, nil
}
