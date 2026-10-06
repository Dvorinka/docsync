package parsers

import (
	"bufio"
	"io"
	"os"
	"regexp"
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
	if len(ds) != len(cs) {
		return false
	}
	for i := range ds {
		if isParamSeg(cs[i]) || isParamSeg(ds[i]) {
			continue
		}
		if !strings.EqualFold(ds[i], cs[i]) {
			return false
		}
	}
	return true
}

func isParamSeg(s string) bool {
	return strings.HasPrefix(s, ":") ||
		(strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) ||
		strings.HasPrefix(s, "*")
}
