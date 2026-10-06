package internal

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Dvorinka/docsync/internal/find"

	"gopkg.in/yaml.v3"
)

// Config mirrors .docsync.yml. All fields optional — defaults apply.
type Config struct {
	Docs            []string          `yaml:"docs"`
	EnvFile         string            `yaml:"env_file"`
	CodeDirs        []string          `yaml:"code_dirs"`
	Languages       []string          `yaml:"languages"`
	APIRoutes       []find.RouteSpec  `yaml:"api_routes"`
	DependencyFiles []string          `yaml:"dependency_files"`
	Severity        map[string]string `yaml:"severity"`
	Ignore          []string          `yaml:"ignore"`
}

// DefaultConfig returns sensible defaults for a repo without .docsync.yml.
func DefaultConfig() Config {
	return Config{
		Docs:      []string{"README.md", "AGENTS.md", "CLAUDE.md", "docs/**/*.md"},
		EnvFile:   ".env.example",
		CodeDirs:  []string{"."},
		Languages: []string{"go", "typescript", "javascript", "bash", "yaml"},
		DependencyFiles: []string{
			"**/package.json", "**/go.mod",
			".github/workflows/*.yml", ".github/workflows/*.yaml",
		},
	}
}

// LoadConfig reads .docsync.yml if present, else returns defaults.
// configPath overrides the default location when non-empty.
func LoadConfig(root, configPath string) (Config, error) {
	cfg := DefaultConfig()
	p := configPath
	if p == "" {
		p = ".docsync.yml"
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) && configPath == "" {
			return cfg, nil
		}
		return cfg, fmt.Errorf("cannot read %s: %w", p, err)
	}
	var raw Config
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return cfg, fmt.Errorf("malformed .docsync.yml: %w", err)
	}
	// Merge: explicit fields win, zero fields keep defaults.
	if len(raw.Docs) > 0 {
		cfg.Docs = raw.Docs
	}
	if raw.EnvFile != "" {
		cfg.EnvFile = raw.EnvFile
	}
	if len(raw.CodeDirs) > 0 {
		cfg.CodeDirs = raw.CodeDirs
	}
	if len(raw.Languages) > 0 {
		cfg.Languages = raw.Languages
	}
	cfg.APIRoutes = raw.APIRoutes
	if len(raw.DependencyFiles) > 0 {
		cfg.DependencyFiles = raw.DependencyFiles
	}
	cfg.Severity = raw.Severity
	cfg.Ignore = raw.Ignore
	return cfg, nil
}
