package parsers

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func bufioScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return sc
}

// Route is a route definition found in source code.
type Route struct {
	Method string // GET/POST/... or "ANY" for method-agnostic registrations
	Path   string
	File   string
	Line   int
}

var (
	// http.HandleFunc("GET /x", ...), mux.HandleFunc("/x"), r.Method("GET", "/x") —
	// any receiver ending in Handle/HandleFunc/Method/MethodFunc.
	goHandleRe = regexp.MustCompile(`\w+\.(?:HandleFunc|Handle|Method|MethodFunc)\(\s*"(?:(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s+)?(/[^"]*)"`)
	// router.GET("/x"), r.Post("/x"), e.DELETE("/x") — any receiver, verb name.
	goVerbRe = regexp.MustCompile(`\w+\.(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS|Get|Post|Put|Delete|Patch|Head|Options)\(\s*"(/[^"]*)"`)
	// app.get("/x"), router.post("/x"), app.use("/x"), app.all("/x")
	expressRe = regexp.MustCompile(`\w+\.(get|post|put|delete|patch|use|all|head|options)\(\s*['"](/[^'"]*)['"]`)
	// export function GET / export const POST = in Next.js route handlers.
	nextExportRe = regexp.MustCompile(`export\s+(?:async\s+)?(?:function|const|let)\s+(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\b`)
)

func scanLines(path string, fn func(lineNo int, line string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufioScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		fn(lineNo, sc.Text())
	}
}

func normVerb(v string) string {
	return strings.ToUpper(v)
}

// GoRoutes extracts route registrations from a .go file (net/http, gin,
// echo, chi, and mux-style receivers).
func GoRoutes(absPath, relPath string) []Route {
	var out []Route
	scanLines(absPath, func(n int, line string) {
		for _, m := range goHandleRe.FindAllStringSubmatch(line, -1) {
			method := "ANY"
			if m[1] != "" {
				method = m[1]
			}
			out = append(out, Route{Method: method, Path: cleanRoutePath(m[2]), File: relPath, Line: n})
		}
		for _, m := range goVerbRe.FindAllStringSubmatch(line, -1) {
			out = append(out, Route{Method: normVerb(m[1]), Path: cleanRoutePath(m[2]), File: relPath, Line: n})
		}
	})
	return out
}

// ExpressRoutes extracts route registrations from a TS/JS file.
func ExpressRoutes(absPath, relPath string) []Route {
	var out []Route
	scanLines(absPath, func(n int, line string) {
		for _, m := range expressRe.FindAllStringSubmatch(line, -1) {
			method := strings.ToUpper(m[1])
			if method == "USE" || method == "ALL" {
				method = "ANY"
			}
			out = append(out, Route{Method: method, Path: cleanRoutePath(m[2]), File: relPath, Line: n})
		}
	})
	return out
}

// NextjsRoute converts a file path under a Next.js root to an API route.
// dir is the scanned directory — the app/pages root ("app", "src/app",
// "pages"); rel is the file path relative to it:
//
//	app/api/users/route.ts      → /api/users     (methods from exports)
//	app/api/users/[id]/route.ts → /api/users/{id}
//	pages/api/health.ts         → /api/health    (ANY)
//
// Page files (page.tsx, index.tsx, any non-route file outside api/) are
// not API routes — returns ("", nil) for them.
func NextjsRoute(dir, rel string) (path string, methods []string) {
	rel = filepath.ToSlash(rel)
	ext := filepath.Ext(rel)
	stem := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(rel), ext), ".d")
	dirLast := filepath.Base(filepath.ToSlash(dir))
	switch {
	case stem == "route":
		// App router: directory path is the route path.
		segs := strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/")
		return "/" + joinNextjsSegs(segs, dirLast), nil
	case strings.HasPrefix(rel, "api/") || dirLast == "api":
		// Pages router: the file itself is the route.
		rel = strings.TrimSuffix(rel, ext)
		segs := strings.Split(rel, "/")
		if stem := segs[len(segs)-1]; stem == "index" {
			segs = segs[:len(segs)-1]
		}
		if len(segs) == 0 {
			return "", nil
		}
		return "/" + joinNextjsSegs(segs, dirLast), []string{"ANY"}
	default:
		return "", nil
	}
}

// joinNextjsSegs maps path segments to a route, prepending dirLast when
// dir itself is the api directory (pages/api as the configured root).
func joinNextjsSegs(segs []string, dirLast string) string {
	var out []string
	if dirLast == "api" && (len(segs) == 0 || segs[0] != "api") {
		out = append(out, "api")
	}
	for _, s := range segs {
		if s == "" || s == "." {
			continue
		}
		out = append(out, nextjsSeg(s))
	}
	return strings.Join(out, "/")
}

// nextjsSeg maps a Next.js path segment to a RouteMatch-compatible one:
// [id] → {id}, [...slug] and [[...slug]] → *slug (matches rest).
func nextjsSeg(s string) string {
	if strings.HasPrefix(s, "[[...") && strings.HasSuffix(s, "]]") {
		return "*" + s[5:len(s)-2]
	}
	if strings.HasPrefix(s, "[...") && strings.HasSuffix(s, "]") {
		return "*" + s[4:len(s)-1]
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		return "{" + s[1:len(s)-1] + "}"
	}
	return s
}

// NextjsExportedMethods scans a route handler file for exported HTTP verbs.
func NextjsExportedMethods(absPath string) []string {
	set := map[string]bool{}
	scanLines(absPath, func(_ int, line string) {
		for _, m := range nextExportRe.FindAllStringSubmatch(line, -1) {
			set[m[1]] = true
		}
	})
	var out []string
	for m := range set {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

func cleanRoutePath(p string) string {
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "/"
	}
	return p
}

// RouteMatch reports whether a documented route plausibly matches a code
// route. Method "ANY" matches everything. Parameter segments (:id, {id},
// {id:regex}) match any single literal segment.
func RouteMatch(method, path string, r Route) bool {
	if r.Method != "ANY" && method != "ANY" && !strings.EqualFold(method, r.Method) {
		return false
	}
	ds := strings.Split(strings.Trim(path, "/"), "/")
	cs := strings.Split(strings.Trim(r.Path, "/"), "/")
	for i := range cs {
		if i >= len(ds) {
			return false
		}
		// Catch-all segment (*slug, from Next.js [...slug]) matches the
		// rest of the documented path.
		if strings.HasPrefix(cs[i], "*") {
			return true
		}
		if isParamSeg(cs[i]) || isParamSeg(ds[i]) {
			continue
		}
		if !strings.EqualFold(ds[i], cs[i]) {
			return false
		}
	}
	return len(ds) == len(cs)
}

func isParamSeg(s string) bool {
	return strings.HasPrefix(s, ":") ||
		(strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) ||
		strings.HasPrefix(s, "*")
}
