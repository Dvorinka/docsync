// Package parsers contains pure file-format parsers: env files, markdown
// references, route definitions, workflow YAML, and package manifests.
package parsers

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

var envKeyRe = regexp.MustCompile(`^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=`)

// EnvFileKeys returns the key names declared in a dotenv-style file.
// Values are never read beyond the '=' — only key names are collected.
func EnvFileKeys(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var keys []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if m := envKeyRe.FindStringSubmatch(line); m != nil {
			keys = append(keys, m[1])
		}
	}
	return keys, sc.Err()
}
