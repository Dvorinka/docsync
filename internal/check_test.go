package internal_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Dvorinka/docsync/internal"
	"github.com/Dvorinka/docsync/internal/find"
)

func driftRoot(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../testdata/fixtures/repo-with-drift")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func cleanRoot(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../testdata/fixtures/clean-repo")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func findSet(res internal.Result) map[string]find.Finding {
	m := map[string]find.Finding{}
	for _, f := range res.Findings {
		m[f.Category+"|"+f.Key] = f
	}
	return m
}

func TestDriftRepoFindings(t *testing.T) {
	cfg, err := internal.LoadConfig(driftRoot(t), "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := internal.RunCheck(driftRoot(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := findSet(res)

	want := []struct{ cat, key, sev string }{
		{"env_missing_in_example", "INTAKE_TOKEN", "critical"},
		{"env_missing_in_example", "RESEND_WEBHOOK_SECRET", "critical"},
		{"env_orphan_in_example", "OLD_API_KEY", "warning"},
		{"path_not_found", "apps/mobile/plugins/withGradleWrapper.js", "critical"},
		{"path_not_found", "apps/api/dochtml.go", "critical"},
		{"command_not_found", "npm run test:mobile", "critical"},
		{"version_mismatch", "node", "warning"},
		{"pin_mismatch", "node", "critical"},
	}
	for _, w := range want {
		f, ok := got[w.cat+"|"+w.key]
		if !ok {
			t.Errorf("missing finding %s %q", w.cat, w.key)
			continue
		}
		if f.Severity != w.sev {
			t.Errorf("%s %q: severity %q, want %q", w.cat, w.key, f.Severity, w.sev)
		}
	}
	if len(res.Findings) != len(want) {
		var extras []string
		for _, f := range res.Findings {
			extras = append(extras, f.Category+":"+f.Key)
		}
		sort.Strings(extras)
		t.Errorf("expected %d findings, got %d: %v", len(want), len(res.Findings), extras)
	}
	if internal.ExitCode(res) != 2 {
		t.Errorf("exit code = %d, want 2", internal.ExitCode(res))
	}
}

func TestCleanRepoNoFindings(t *testing.T) {
	cfg, err := internal.LoadConfig(cleanRoot(t), "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := internal.RunCheck(cleanRoot(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 0 {
		for _, f := range res.Findings {
			t.Errorf("unexpected finding: %+v", f)
		}
	}
	if internal.ExitCode(res) != 0 {
		t.Errorf("exit code = %d, want 0", internal.ExitCode(res))
	}
}

func TestFixAddsMissingKeys(t *testing.T) {
	// Copy the drift repo's env file into a temp dir shape we can mutate.
	tmp := t.TempDir()
	for _, rel := range []string{".env.example", "apps/api/intake.go", "apps/api/resend.go", "apps/api/main.go"} {
		src := filepath.Join(driftRoot(t), rel)
		dst := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(src)
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := internal.DefaultConfig()
	fr, _, err := internal.Fix(tmp, cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fr.Fixed) != 2 {
		t.Fatalf("expected 2 fixed, got %d", len(fr.Fixed))
	}
	b, _ := os.ReadFile(filepath.Join(tmp, ".env.example"))
	s := string(b)
	for _, k := range []string{"INTAKE_TOKEN", "RESEND_WEBHOOK_SECRET"} {
		if !containsLine(s, k+"=") {
			t.Errorf(".env.example missing added key %s:\n%s", k, s)
		}
	}
	for _, k := range []string{"DATABASE_URL=", "JWT_SECRET=", "OLD_API_KEY="} {
		if !containsLine(s, k) {
			t.Errorf("existing entry %s was removed:\n%s", k, s)
		}
	}
}

func TestFixDryRun(t *testing.T) {
	tmp := t.TempDir()
	for _, rel := range []string{".env.example", "apps/api/intake.go", "apps/api/resend.go", "apps/api/main.go"} {
		src := filepath.Join(driftRoot(t), rel)
		dst := filepath.Join(tmp, rel)
		os.MkdirAll(filepath.Dir(dst), 0o755)
		b, _ := os.ReadFile(src)
		os.WriteFile(dst, b, 0o644)
	}
	before, _ := os.ReadFile(filepath.Join(tmp, ".env.example"))
	cfg := internal.DefaultConfig()
	fr, _, err := internal.Fix(tmp, cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(fr.Fixed) != 2 {
		t.Fatalf("expected 2 planned fixes, got %d", len(fr.Fixed))
	}
	after, _ := os.ReadFile(filepath.Join(tmp, ".env.example"))
	if string(before) != string(after) {
		t.Error("dry-run modified .env.example")
	}
}

func containsLine(s, prefix string) bool {
	for _, l := range splitLines(s) {
		if len(l) >= len(prefix) && l[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func TestBaselineSuppresses(t *testing.T) {
	root := driftRoot(t)
	cfg, err := internal.LoadConfig(root, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := internal.RunCheck(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	blPath := filepath.Join(t.TempDir(), ".docsync-baseline")
	if err := internal.WriteBaseline(blPath, res.Findings); err != nil {
		t.Fatal(err)
	}
	bl, err := internal.LoadBaseline(blPath)
	if err != nil {
		t.Fatal(err)
	}
	filtered := internal.ApplyBaseline(res, bl)
	if filtered.Summary.Total != 0 || filtered.Summary.Suppressed != len(res.Findings) {
		t.Fatalf("expected all suppressed, got %+v", filtered.Summary)
	}
	if internal.ExitCode(filtered) != 0 {
		t.Fatal("baselined findings must not gate")
	}
	// A new finding still fires.
	res.Findings = append(res.Findings, find.New("path_not_found", "new/missing.go", "d", "README.md", 1, nil))
	filtered = internal.ApplyBaseline(res, bl)
	if filtered.Summary.Total != 1 {
		t.Fatalf("new drift should survive the baseline, got %+v", filtered.Summary)
	}
}

func TestSARIFOutput(t *testing.T) {
	root := driftRoot(t)
	cfg, err := internal.LoadConfig(root, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := internal.RunCheck(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := internal.WriteSARIF(&buf, res, "test"); err != nil {
		t.Fatal(err)
	}
	var log struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name  string `json:"name"`
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID string `json:"ruleId"`
				Level  string `json:"level"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("invalid sarif: %v", err)
	}
	if log.Version != "2.1.0" || len(log.Runs) != 1 || log.Runs[0].Tool.Driver.Name != "docsync" {
		t.Fatalf("bad sarif envelope: %+v", log)
	}
	if len(log.Runs[0].Results) != len(res.Findings) {
		t.Fatalf("results %d != findings %d", len(log.Runs[0].Results), len(res.Findings))
	}
	for _, r := range log.Runs[0].Results {
		if r.Level != "error" && r.Level != "warning" {
			t.Fatalf("bad level %q", r.Level)
		}
	}
}

func TestNextjsRoutes(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	write("app/api/users/route.ts", "export async function GET() {}\nexport function POST() {}\n")
	write("app/api/users/[id]/route.ts", "export function GET() {}\n")
	write("app/dashboard/page.tsx", "export default function Page() {}")
	write("README.md", "# api\n\n`GET /api/users` `POST /api/users` `GET /api/users/42`\n")
	os.WriteFile(filepath.Join(root, ".docsync.yml"),
		[]byte("api_routes:\n  - dir: app\n    framework: nextjs\n"), 0o644)

	cfg, err := internal.LoadConfig(root, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := internal.RunCheck(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Findings {
		if f.Category == "route_not_found" || f.Category == "route_undocumented" {
			t.Fatalf("nextjs routes should all match: %+v", f)
		}
	}
}

func TestStrictPaths(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	os.WriteFile(filepath.Join(root, ".env.example"), []byte("X=1\n"), 0o644)
	os.WriteFile(filepath.Join(root, "docs/guide.md"),
		[]byte("see `tests/e2e` and `x/crypto/argon2`\n"), 0o644)
	cfg, _ := internal.LoadConfig(root, "")
	cfg.Docs = []string{"docs/*.md"}
	res, err := internal.RunCheck(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Findings {
		if f.Category == "path_not_found" {
			t.Fatalf("namespace heuristic should skip both, got %+v", f)
		}
	}
	cfg.StrictPaths = true
	res, _ = internal.RunCheck(root, cfg)
	var strictPaths []string
	for _, f := range res.Findings {
		if f.Category == "path_not_found" {
			strictPaths = append(strictPaths, f.Key)
		}
	}
	found := false
	for _, k := range strictPaths {
		if k == "tests/e2e" {
			found = true
		}
	}
	if !found {
		t.Fatalf("strict_paths should flag tests/e2e, got %v", strictPaths)
	}
}
