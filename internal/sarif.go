package internal

import (
	"encoding/json"
	"io"
)

// SARIF 2.1.0 output — feeds GitHub code scanning so findings become PR
// annotations instead of a pass/fail gate alone.

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string          `json:"id"`
	ShortDescription sarifMessage    `json:"shortDescription"`
	DefaultLevel     string          `json:"defaultConfigurationLevel,omitempty"`
	DefaultConfig    *sarifLevelWrap `json:"defaultConfiguration,omitempty"`
}

type sarifLevelWrap struct {
	Level string `json:"level"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           *sarifRegion  `json:"region,omitempty"`
}

type sarifArtifact struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

// WriteSARIF renders a Result as a SARIF 2.1.0 log.
func WriteSARIF(w io.Writer, res Result, version string) error {
	cats := map[string]string{}
	seenOrder := []string{}
	var results []sarifResult
	for _, f := range res.Findings {
		if _, ok := cats[f.Category]; !ok {
			cats[f.Category] = f.Severity
			seenOrder = append(seenOrder, f.Category)
		}
		r := sarifResult{
			RuleID:  f.Category,
			Level:   sarifLevel(f.Severity),
			Message: sarifMessage{Text: f.Detail},
		}
		if f.File != "" {
			loc := sarifLocation{PhysicalLocation: sarifPhysical{
				ArtifactLocation: sarifArtifact{URI: f.File}}}
			if f.Line > 0 {
				loc.PhysicalLocation.Region = &sarifRegion{StartLine: f.Line}
			}
			r.Locations = []sarifLocation{loc}
		}
		results = append(results, r)
	}
	var rules []sarifRule
	for _, c := range seenOrder {
		rules = append(rules, sarifRule{
			ID:               c,
			ShortDescription: sarifMessage{Text: c},
			DefaultConfig:    &sarifLevelWrap{Level: sarifLevel(cats[c])},
		})
	}
	log := sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "docsync",
				Version:        version,
				InformationURI: "https://github.com/Dvorinka/docsync",
				Rules:          rules,
			}},
			Results: results,
		}},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}

func sarifLevel(sev string) string {
	if sev == "critical" {
		return "error"
	}
	return "warning"
}
