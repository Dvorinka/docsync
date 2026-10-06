package internal_test

import (
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
