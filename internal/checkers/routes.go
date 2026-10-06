package checkers

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Dvorinka/docsync/internal/find"
	"github.com/Dvorinka/docsync/internal/fsx"
	"github.com/Dvorinka/docsync/internal/parsers"
)

// Routes cross-references documented API routes (GET /api/...) against route
// definitions in code. Undocumented code routes are only flagged when the
// docs enumerate several routes — a passing mention is not a contract.
func Routes(root string, docs []parsers.Doc, specs []find.RouteSpec, sev map[string]string) []find.Finding {
	if len(specs) == 0 {
		return nil // route checks are opt-in via api_routes
	}
	var docRoutes []parsers.RouteRef
	for _, d := range docs {
		docRoutes = append(docRoutes, d.Routes...)
	}
	if len(docRoutes) == 0 {
		return nil
	}
	var code []parsers.Route
	for _, spec := range specs {
		dir := filepath.Join(root, spec.Dir)
		fsx.WalkFiles(dir, func(rel string) {
			full := filepath.ToSlash(filepath.Join(spec.Dir, rel))
			switch strings.ToLower(spec.Framework) {
			case "go":
				if strings.HasSuffix(rel, ".go") {
					code = append(code, parsers.GoRoutes(filepath.Join(dir, rel), full)...)
				}
			case "express":
				switch filepath.Ext(rel) {
				case ".ts", ".tsx", ".js", ".jsx":
					code = append(code, parsers.ExpressRoutes(filepath.Join(dir, rel), full)...)
				}
			}
		})
	}
	var out []find.Finding
	seen := map[string]bool{}
	for _, dr := range docRoutes {
		matched := false
		for _, cr := range code {
			if parsers.RouteMatch(dr.Method, dr.Path, cr) {
				matched = true
				break
			}
		}
		if !matched {
			key := dr.Method + " " + dr.Path
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, find.New("route_not_found", key,
				fmt.Sprintf("documented but no matching route definition found in configured api_routes"),
				firstDocFile(docs, dr), dr.Line, sev))
		}
	}
	// Only flag undocumented code routes when docs enumerate a real API list.
	if len(docRoutes) >= 3 {
		docSet := map[string]bool{}
		for _, dr := range docRoutes {
			docSet[dr.Method+" "+dr.Path] = true
		}
		for _, cr := range code {
			covered := false
			for _, dr := range docRoutes {
				if parsers.RouteMatch(dr.Method, dr.Path, cr) {
					covered = true
					break
				}
			}
			if !covered {
				key := cr.Method + " " + cr.Path
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, find.New("route_undocumented", key,
					fmt.Sprintf("defined in %s:%d but not mentioned in docs", cr.File, cr.Line),
					cr.File, cr.Line, sev))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func firstDocFile(docs []parsers.Doc, target parsers.RouteRef) string {
	for _, d := range docs {
		for _, r := range d.Routes {
			if r.Method == target.Method && r.Path == target.Path && r.Line == target.Line {
				return d.Path
			}
		}
	}
	return docs[0].Path
}
