package gitx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These tests run real git in temp directories, isolated from the user's
// git config.
func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	all := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
	if out, err := exec.Command("git", all...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// repo is a git repository with one commit of a.go (3 lines).
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFile(t, dir, "a.go", "one\ntwo\nthree\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func TestNotARepo(t *testing.T) {
	needGit(t)
	ctx, dir := context.Background(), t.TempDir()
	if IsRepo(ctx, dir) || Info(ctx, dir) != nil {
		t.Error("a plain directory is not a repo")
	}
	if _, err := ReadChurn(ctx, dir, 12); !errors.Is(err, ErrNotRepo) {
		t.Errorf("churn: %v", err)
	}
	if _, err := ReadChanges(ctx, dir, "main"); !errors.Is(err, ErrNotRepo) {
		t.Errorf("changes: %v", err)
	}
	if _, err := ReadFunctionChurn(ctx, dir, 12, nil); !errors.Is(err, ErrNotRepo) {
		t.Errorf("function churn: %v", err)
	}
}

func TestInfo(t *testing.T) {
	needGit(t)
	ctx := context.Background()
	empty := t.TempDir()
	gitRun(t, empty, "init", "-q")
	if info := Info(ctx, empty); info == nil || info.Head != "" || info.Dirty {
		t.Errorf("repo without commits: %+v", info)
	}
	dir := repo(t)
	info := Info(ctx, dir)
	if info == nil || len(info.Head) != 40 || info.Dirty {
		t.Errorf("clean repo: %+v", info)
	}
	writeFile(t, dir, "a.go", "one\n2\nthree\n")
	if info := Info(ctx, dir); info == nil || !info.Dirty {
		t.Errorf("changed file: %+v", info)
	}
}

func TestReadFromRepo(t *testing.T) {
	needGit(t)
	ctx, dir := context.Background(), repo(t)
	if ch, err := ReadChanges(ctx, dir, "HEAD"); err != nil || len(ch.Hunks) != 0 || ch.Untracked != nil {
		t.Errorf("no changes: %+v %v", ch, err)
	}
	writeFile(t, dir, "a.go", "one\n2\nthree\n")
	writeFile(t, dir, "new.go", "x\n")
	ch, err := ReadChanges(ctx, dir, "HEAD")
	if err != nil || ch.Base != "HEAD" || !reflect.DeepEqual(ch.Hunks["a.go"], []Range{{2, 2, false}}) || !reflect.DeepEqual(ch.Untracked, []string{"new.go"}) {
		t.Errorf("changes: %+v %v", ch, err)
	}
	if _, err := ReadChanges(ctx, dir, "nope"); err == nil || err.Error() != `base ref "nope" not found; set [project].base_ref in .qtldr.toml or use --all` {
		t.Errorf("missing base: %v", err)
	}
	churn, err := ReadChurn(ctx, dir, 12)
	if err != nil || churn.Files["a.go"] != 1 || churn.Dirs["."] != 1 {
		t.Errorf("churn: %+v %v", churn, err)
	}
	fc, err := ReadFunctionChurn(ctx, dir, 12, []Span{{ID: "f", File: "a.go", From: 1, To: 3}, {ID: "g", File: "b.go", From: 1, To: 3}})
	if err != nil || fc["f"] != 1 || fc["g"] != 0 || len(fc) != 2 {
		t.Errorf("function churn: %v %v", fc, err)
	}
}

func TestGitErrors(t *testing.T) {
	needGit(t)
	ctx := context.Background()
	if _, err := git(ctx, t.TempDir(), "log"); err == nil || !strings.HasPrefix(err.Error(), "git log: fatal:") {
		t.Errorf("stderr is kept: %v", err)
	}
	// --quiet: exit status 1 and no stderr, so the error is git's own
	if _, err := git(ctx, repo(t), "rev-parse", "--verify", "--quiet", "nope"); err == nil || err.Error() != "exit status 1" {
		t.Errorf("no stderr: %v", err)
	}
}
