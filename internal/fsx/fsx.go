package fsx

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SkipDirs are directory names never descended into during repo walks.
var SkipDirs = map[string]bool{
	".git": true, ".svn": true, ".hg": true,
	"node_modules": true, "vendor": true, "dist": true, "build": true,
	"out": true, "coverage": true, "testdata": true, "__pycache__": true,
	"target": true, ".next": true, ".turbo": true, ".cache": true,
	".idea": true, ".vscode": true, ".gradle": true,
}

// MatchGlob matches a slash-separated path against a glob pattern.
// '*' matches within one segment, '**' matches zero or more whole segments.
func MatchGlob(pattern, name string) bool {
	p := strings.Split(filepath.ToSlash(pattern), "/")
	n := strings.Split(filepath.ToSlash(name), "/")
	return matchSegs(p, n)
}

func matchSegs(p, n []string) bool {
	for len(p) > 0 {
		if p[0] == "**" {
			for i := 0; i <= len(n); i++ {
				if matchSegs(p[1:], n[i:]) {
					return true
				}
			}
			return false
		}
		if len(n) == 0 {
			return false
		}
		ok, err := filepath.Match(p[0], n[0])
		if err != nil || !ok {
			return false
		}
		p, n = p[1:], n[1:]
	}
	return len(n) == 0
}

// HasGlob reports whether a pattern contains glob metacharacters.
func HasGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// GlobRoot expands a glob pattern relative to root, returning repo-relative
// paths of existing files. Skips SkipDirs. Non-glob patterns return the path
// itself if the file exists.
func GlobRoot(root, pattern string) []string {
	if !HasGlob(pattern) {
		if st, err := os.Stat(filepath.Join(root, pattern)); err == nil && !st.IsDir() {
			return []string{filepath.ToSlash(pattern)}
		}
		return nil
	}
	var out []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && SkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if MatchGlob(pattern, rel) {
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// WalkFiles walks root, calling fn for every regular file with its
// repo-relative slash path. SkipDirs are pruned.
func WalkFiles(root string, fn func(rel string)) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() != "." && SkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			rel, _ := filepath.Rel(root, path)
			fn(filepath.ToSlash(rel))
		}
		return nil
	})
}
