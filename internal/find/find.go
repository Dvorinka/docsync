// Package find holds the shared finding types used by every checker and
// the report/JSON output contract.
package find

import "sort"

// Finding is a single documentation-drift result.
type Finding struct {
	Severity string `json:"severity"` // "critical" | "warning"
	Category string `json:"category"` // env_missing_in_example, path_not_found, ...
	Key      string `json:"key"`      // the thing that drifted
	Detail   string `json:"detail"`
	File     string `json:"file"` // file where the reference was found
	Line     int    `json:"line"` // 0 when not applicable
}

// RouteSpec configures one directory of route definitions to cross-reference.
type RouteSpec struct {
	Dir       string `yaml:"dir"`
	Framework string `yaml:"framework"`
}

// Summary counts findings by severity.
type Summary struct {
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Total    int `json:"total"`
}

// DefaultSeverity maps category -> default severity (overridable via .docsync.yml).
var DefaultSeverity = map[string]string{
	"env_missing_in_example": "critical",
	"env_orphan_in_example":  "warning",
	"path_not_found":         "critical",
	"command_not_found":      "critical",
	"route_not_found":        "warning",
	"route_undocumented":     "warning",
	"version_mismatch":       "warning",
	"pin_mismatch":           "critical",
	"pin_unpinned":           "warning",
}

// New builds a finding with the configured severity for its category.
func New(category, key, detail, file string, line int, overrides map[string]string) Finding {
	sev := DefaultSeverity[category]
	if overrides != nil {
		if s, ok := overrides[category]; ok && s != "" {
			sev = s
		}
	}
	return Finding{Severity: sev, Category: category, Key: key, Detail: detail, File: file, Line: line}
}

// Sort orders findings: critical first, then category, file, line.
func Sort(fs []Finding) {
	order := map[string]int{"critical": 0, "warning": 1}
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if order[a.Severity] != order[b.Severity] {
			return order[a.Severity] < order[b.Severity]
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
}

// Summarize counts severities.
func Summarize(fs []Finding) Summary {
	var s Summary
	for _, f := range fs {
		switch f.Severity {
		case "critical":
			s.Critical++
		default:
			s.Warning++
		}
	}
	s.Total = len(fs)
	return s
}
