package internal

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Dvorinka/docsync/internal/find"
)

var categoryTitles = map[string]string{
	"env_missing_in_example": "ENVIRONMENT VARIABLES",
	"env_orphan_in_example":  "ENVIRONMENT VARIABLES",
	"path_not_found":         "FILE PATHS",
	"command_not_found":      "COMMANDS",
	"route_not_found":        "API ROUTES",
	"route_undocumented":     "API ROUTES",
	"version_mismatch":       "VERSIONS",
	"pin_mismatch":           "BUILD PINS",
	"pin_unpinned":           "BUILD PINS",
}

var categoryOrder = []string{
	"ENVIRONMENT VARIABLES", "FILE PATHS", "COMMANDS",
	"API ROUTES", "VERSIONS", "BUILD PINS",
}

func icon(sev string) string {
	switch sev {
	case "critical":
		return "✗ CRITICAL "
	default:
		return "⚠ WARNING "
	}
}

// Report writes the human-readable grouped report.
func Report(w io.Writer, res Result) {
	sum := res.Summary
	fmt.Fprintf(w, "Documentation drift report — %d finding%s\n\n", sum.Total, plural(sum.Total))
	groups := map[string][]find.Finding{}
	for _, f := range res.Findings {
		title := categoryTitles[f.Category]
		if title == "" {
			title = strings.ToUpper(strings.ReplaceAll(f.Category, "_", " "))
		}
		groups[title] = append(groups[title], f)
	}
	for _, title := range categoryOrder {
		fs := groups[title]
		if len(fs) == 0 {
			continue
		}
		fmt.Fprintf(w, "%s\n", title)
		sort.SliceStable(fs, func(i, j int) bool {
			if fs[i].Severity != fs[j].Severity {
				return fs[i].Severity == "critical"
			}
			return fs[i].Key < fs[j].Key
		})
		for _, f := range fs {
			fmt.Fprintf(w, "  %s  %-38s %s\n", icon(f.Severity), f.Key, f.Detail)
		}
		fmt.Fprintln(w)
	}
	for _, warn := range res.Warnings {
		fmt.Fprintf(w, "note: %s\n", warn)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
