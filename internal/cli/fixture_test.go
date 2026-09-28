package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixtureRepo copies testdata/ledger into a new git repository with one
// commit on main and returns its directory.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("runs go test and git on the fixture")
	}
	src, err := filepath.Abs(filepath.Join("..", "..", "testdata", "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() && (d.Name() == ".qtldr" || d.Name() == "target") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{"-c", "user.name=qtldr", "-c", "user.email=qtldr@example.com", "commit", "-q", "-m", "fixture"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dst}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dst
}

// editTier changes applyTiered's floor comparison (line 29).
func editTier(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "internal", "pricing", "tier.go")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(b), "qty >= t.Floor", "qty > t.Floor", 1)
	if edited == string(b) {
		t.Fatal("tier.go no longer has the line this test edits")
	}
	if err := os.WriteFile(p, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// M1 acceptance: `check --changed` after editing applyTiered exits 1 and
// names the breach.
func TestCheckChangedOnFixture(t *testing.T) {
	dir := fixtureRepo(t)
	if r := runCLI(t, allTools, "-C", dir, "check"); r.code != 0 || !strings.Contains(r.stdout, "0 changed functions · base main · pass") {
		t.Fatalf("clean tree: %+v", r)
	}
	editTier(t, dir)
	r := runCLI(t, allTools, "-C", dir, "check", "--changed")
	if r.code != 1 {
		t.Fatalf("code %d, want 1\n%s%s", r.code, r.stdout, r.stderr)
	}
	for _, want := range []string{
		"## qtldr check · 1 changed functions · base main",
		"- [crap] pricing.applyTiered — 14.1 (limit 8.0)",
		"- [coverage] pricing.applyTiered — 70.6% (limit 80%) — add tests for lines tier.go:",
		"| pricing.applyTiered | 14.1 | 70.6% | — | 11 |",
		"pricing.applyTiered — mutation not run since last change (run: qtldr mutate --func pricing.applyTiered)",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
	// JSON has the same verdict.
	r = runCLI(t, allTools, "-C", dir, "check", "--json", "--fast")
	var rep struct {
		Pass     bool
		Breaches []struct{ Kind string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil || rep.Pass {
		t.Fatalf("json: %v %s", err, r.stdout)
	}
}

// M1 acceptance: --files-from-stdin reads a real Claude Code payload.
func TestCheckFilesFromStdin(t *testing.T) {
	dir := fixtureRepo(t)
	tier := editTier(t, dir)
	payload := capturedPayload(t, "posttooluse-edit.json", tier)

	// Coverage is stale after the edit, so fast mode checks cognitive only:
	// applyTiered passes. Seed a breach by lowering the cognitive limit.
	if err := os.WriteFile(filepath.Join(dir, ".qtldr.toml"), []byte("[thresholds]\ncognitive_max = 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	e := &env{ctx: context.Background(), stdin: bytes.NewReader(payload), stdout: &out, stderr: &errb, sys: allTools}
	code := run(e, []string{"check", "--fast", "--quiet", "--files-from-stdin"})
	if code != 2 || !strings.Contains(errb.String(), "[cognitive] pricing.applyTiered — 11 (limit 5)") || out.Len() != 0 {
		t.Fatalf("code %d\nstdout: %s\nstderr: %s", code, out.String(), errb.String())
	}

	// A payload for a non-Go file is ignored silently.
	e = &env{ctx: context.Background(), stdin: bytes.NewReader(capturedPayload(t, "posttooluse-write.json", filepath.Join(dir, "README.md"))), stdout: &out, stderr: &errb, sys: allTools}
	errb.Reset()
	if code := run(e, []string{"check", "--fast", "--quiet", "--files-from-stdin"}); code != 0 || errb.Len() != 0 {
		t.Fatalf("non-Go file: code %d stderr %s", code, errb.String())
	}

	// Broken input never blocks the agent.
	e = &env{ctx: context.Background(), stdin: strings.NewReader("not json"), stdout: &out, stderr: &errb, sys: allTools}
	if code := run(e, []string{"check", "--files-from-stdin"}); code != 0 {
		t.Fatalf("bad payload: code %d", code)
	}
}

// capturedPayload loads a payload captured from Claude Code and points its
// file_path at file (the rest of the payload is kept verbatim).
func capturedPayload(t *testing.T, name, file string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "claude-hook", name))
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	p["tool_input"].(map[string]any)["file_path"] = file
	out, _ := json.Marshal(p)
	return out
}

func TestHookInstall(t *testing.T) {
	dir := fixtureRepo(t)
	r := runCLI(t, allTools, "-C", dir, "hook", "install")
	if r.code != 0 || !strings.Contains(r.stdout, "Add this to CLAUDE.md") || !strings.Contains(r.stdout, "pre-push") {
		t.Fatalf("%+v", r)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil || !strings.Contains(string(b), "qtldr check --fast --quiet --files-from-stdin") {
		t.Fatalf("settings: %s %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "pre-push")); err != nil {
		t.Fatal(err)
	}
}

func TestExplain(t *testing.T) {
	r := runCLI(t, allTools, "-C", t.TempDir(), "explain", "crap")
	if r.code != 0 || !strings.Contains(r.stdout, "CRAP — Change Risk Anti-Patterns") || !strings.Contains(r.stdout, "target: 8 or less") {
		t.Fatalf("%+v", r)
	}
	if r := runCLI(t, allTools, "explain", "nope"); r.code != 2 || !strings.Contains(r.stderr, "known terms") {
		t.Fatalf("%+v", r)
	}
}

func TestServeStartsAndStops(t *testing.T) {
	dir := tempModule(t)
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	var out, errb bytes.Buffer
	sys := fakeSystem{found: map[string]string{"open": "", "xdg-open": ""}}
	e := &env{ctx: ctx, stdout: &out, stderr: &errb, sys: sys}
	code := run(e, []string{"-C", dir, "serve", "--port", "0", "--watch", "--open"})
	if code != 0 || !strings.Contains(out.String(), "qtldr is serving") || !strings.Contains(out.String(), "http://127.0.0.1:") {
		t.Fatalf("code %d\nstdout: %s\nstderr: %s", code, out.String(), errb.String())
	}
	if r := runCLI(t, allTools, "-C", dir, "serve", "extra"); r.code != 2 {
		t.Errorf("serve with args: %+v", r)
	}
}

func TestNoteCommands(t *testing.T) {
	dir := tempModule(t)
	if r := runCLI(t, allTools, "-C", dir, "note", "add", "a.Big", "x"); r.code != 2 || !strings.Contains(r.stderr, "qtldr analyze") {
		t.Fatalf("note before analyze: %+v", r)
	}
	runCLI(t, allTools, "-C", dir, "analyze", "--quiet")
	r := runCLI(t, allTools, "-C", dir, "note", "add", "a.Big", "--line", "7", "check", "the", "bounds")
	if r.code != 0 || !strings.Contains(r.stdout, "Added n_") {
		t.Fatalf("add: %+v", r)
	}
	id := strings.Fields(strings.TrimPrefix(r.stdout, "Added "))[0]
	r = runCLI(t, allTools, "-C", dir, "note", "list", "Big")
	if r.code != 0 || !strings.Contains(r.stdout, "a.Big:7") || !strings.Contains(r.stdout, "check the bounds") {
		t.Fatalf("list: %+v", r)
	}
	if r := runCLI(t, allTools, "-C", dir, "note", "resolve", id); r.code != 0 {
		t.Fatalf("resolve: %+v", r)
	}
	if r := runCLI(t, allTools, "-C", dir, "note", "list"); !strings.Contains(r.stdout, "No notes.") {
		t.Errorf("resolved notes hidden: %+v", r)
	}
	if r := runCLI(t, allTools, "-C", dir, "--json", "note", "list", "--all"); !strings.Contains(r.stdout, `"resolved": true`) {
		t.Errorf("--all: %+v", r)
	}
	for _, args := range [][]string{{"note"}, {"note", "zap"}, {"note", "add", "a.Big"}, {"note", "resolve"}, {"note", "resolve", "n_nope"}, {"note", "add", "a.Big", "--line", "99", "x"}} {
		if r := runCLI(t, allTools, append([]string{"-C", dir}, args...)...); r.code != 2 {
			t.Errorf("%v: %+v", args, r)
		}
	}
}
