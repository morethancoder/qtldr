package coverage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/morethancoder/qtldr/internal/config"
)

// tinyModule writes a one-package module with a passing test.
func tinyModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":      "module example.com/cov\n\ngo 1.26.0\n",
		"a/a.go":      "package a\n\n// Pos reports x > 0.\nfunc Pos(x int) bool {\n\treturn x > 0\n}\n",
		"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestPos(t *testing.T) {\n\tif !Pos(1) {\n\t\tt.Fatal(\"Pos\")\n\t}\n}\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func cfgOf(command string, timeout time.Duration) config.Coverage {
	return config.Coverage{Command: command, Coverpkg: "own", Timeout: config.Duration{Duration: timeout}}
}

const testCommand = "go test -covermode=count -coverprofile={profile} {packages}"

func TestRun(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	root := tinyModule(t)
	profile := filepath.Join(t.TempDir(), "cache", "coverage.out")
	// A zero timeout means no limit.
	res, err := Run(context.Background(), root, cfgOf(testCommand, 0), []string{"./..."}, profile)
	if err != nil {
		t.Fatalf("%v\n%s", err, res.Output)
	}
	if res.Profile.Mode != "count" || len(res.Profile.Blocks) == 0 || len(res.Failed) != 0 || res.Args[0] != "go" {
		t.Fatalf("result %+v", res)
	}
	if !strings.HasSuffix(res.Profile.Blocks[0].File, "a/a.go") || res.Profile.Blocks[0].Count == 0 {
		t.Errorf("block %+v", res.Profile.Blocks[0])
	}
}

func TestRunErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go")
	}
	root := tinyModule(t)
	profile := filepath.Join(t.TempDir(), "p.out")
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, command string
		timeout       time.Duration
		profile, want string
	}{
		{"no {packages}", "go test", time.Minute, profile, "must contain {packages}"},
		{"timeout", testCommand, time.Nanosecond, profile, "coverage run stopped after 1ns ([coverage].timeout): go test"},
		{"profile directory", testCommand, time.Minute, filepath.Join(notADir, "p.out"), "not a directory"},
		{"no profile written", "go list {packages}", time.Minute, profile, "`go list ./...` wrote no coverage profile"},
	}
	for _, c := range cases {
		_, err := Run(context.Background(), root, cfgOf(c.command, c.timeout), []string{"./..."}, c.profile)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want %q", c.name, err, c.want)
		}
	}
}
