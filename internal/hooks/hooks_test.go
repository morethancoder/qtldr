package hooks

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeClaudeSettings(t *testing.T) {
	existing := `{
  "model": "opus",
  "hooks": {
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "guard"}]}],
    "PostToolUse": [{"matcher": "Edit", "hooks": [{"type": "command", "command": "gofmt -w"}]}]
  },
  "permissions": {"allow": ["Bash(go test:*)"]}
}`
	out, changed, err := MergeClaudeSettings([]byte(existing))
	if err != nil || !changed {
		t.Fatalf("changed %v err %v", changed, err)
	}
	s := string(out)
	if strings.Index(s, `"model"`) > strings.Index(s, `"hooks"`) || strings.Index(s, `"hooks"`) > strings.Index(s, `"permissions"`) {
		t.Errorf("key order changed:\n%s", s)
	}
	var parsed struct {
		Hooks map[string][]group `json:"hooks"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	post := parsed.Hooks["PostToolUse"]
	if len(post) != 2 || post[0].Hooks[0].Command != "gofmt -w" || post[1].Hooks[0].Command != ClaudeCommand || post[1].Hooks[0].Timeout != 30 {
		t.Errorf("PostToolUse = %+v", post)
	}
	if len(parsed.Hooks["PreToolUse"]) != 1 {
		t.Error("other events must be kept")
	}
	again, changed, err := MergeClaudeSettings(out)
	if err != nil || changed || string(again) != string(out) {
		t.Errorf("second merge must be a no-op: changed %v err %v", changed, err)
	}
}

func TestMergeClaudeSettingsEmptyAndInvalid(t *testing.T) {
	out, changed, err := MergeClaudeSettings(nil)
	if err != nil || !changed || !strings.Contains(string(out), ClaudeCommand) {
		t.Fatalf("empty: %s %v %v", out, changed, err)
	}
	for _, bad := range []string{"[]", `{"hooks": []}`, `{"hooks": {"PostToolUse": {}}}`} {
		if _, _, err := MergeClaudeSettings([]byte(bad)); err == nil {
			t.Errorf("%s: want error", bad)
		}
	}
}

func TestInstallGit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks", "pre-push")
	if err := InstallGit(path, "testdata/ledger"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), GitMarker) || !strings.Contains(string(b), `/testdata/ledger" || exit 1`) {
		t.Fatalf("hook:\n%s", b)
	}
	if err := InstallGit(path, ""); err != nil {
		t.Fatalf("re-install over our own hook: %v", err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nmake lint\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := InstallGit(path, ""); !errors.Is(err, ErrForeignHook) {
		t.Fatalf("foreign hook: err = %v", err)
	}
}
