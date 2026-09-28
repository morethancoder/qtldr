package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestUpgradePlan(t *testing.T) {
	cases := []struct {
		current, latest, want string
	}{
		{"0.1.0-dev", "v0.1.0", upgradeAvailable},                          // local build without VCS info
		{"v0.0.0-20260928195440-29024d6cdaa5", "v0.1.0", upgradeAvailable}, // local build before the first tag
		{"v0.1.0", "v0.2.0", upgradeAvailable},                             // older release
		{"v0.1.0", "v0.1.0", upgradeUpToDate},                              // latest release
		{"v0.1.0+dirty", "v0.1.0", upgradeUpToDate},                        // build metadata is ignored
		{"v0.1.1-0.20261001120000-abcdefabcdef", "v0.1.0", upgradeAhead},   // local build after the tag
		{"v0.3.0", "v0.2.0", upgradeAhead},                                 // retracted or not yet on the proxy
	}
	for _, c := range cases {
		if got := upgradePlan(c.current, c.latest); got != c.want {
			t.Errorf("upgradePlan(%q, %q) = %q, want %q", c.current, c.latest, got, c.want)
		}
	}
}

func TestBinPath(t *testing.T) {
	cases := []struct {
		gobin, gopath, exe, want string
	}{
		{"/gobin", "/gp", "", "/gobin/qtldr"},
		{"", "/a:/b", ".exe", "/a/bin/qtldr.exe"},
		{"", "", "", "bin/qtldr"},
	}
	for _, c := range cases {
		if got := binPath(c.gobin, c.gopath, c.exe); got != c.want {
			t.Errorf("binPath(%q, %q, %q) = %q, want %q", c.gobin, c.gopath, c.exe, got, c.want)
		}
	}
}

// goSystem answers commands by their full argument list and records them.
type goSystem struct {
	fakeSystem
	out  map[string]string
	fail map[string]bool
	ran  *[]string
}

func (g goSystem) Output(_ context.Context, name string, args ...string) (string, error) {
	cmd := strings.Join(append([]string{name}, args...), " ")
	*g.ran = append(*g.ran, cmd)
	if g.fail[cmd] {
		return g.out[cmd], errors.New("exit status 1")
	}
	return g.out[cmd], nil
}

const (
	listLatest = "go list -m -f {{.Version}} github.com/morethancoder/qtldr@latest"
	installV2  = "go install github.com/morethancoder/qtldr/cmd/qtldr@v0.2.0"
	goEnv      = "go env -json GOBIN GOPATH GOEXE"
)

func newGoSystem(onPath string) goSystem {
	found := map[string]string{}
	if onPath != "" {
		found["qtldr"] = onPath
	}
	return goSystem{
		fakeSystem: fakeSystem{found: found},
		out: map[string]string{
			listLatest: "go: downloading something\nv0.2.0",
			goEnv:      `{"GOBIN": "/home/u/go/bin", "GOEXE": "", "GOPATH": "/home/u/go"}`,
		},
		fail: map[string]bool{},
		ran:  &[]string{},
	}
}

// Test binaries carry no release version, so the current build always counts
// as older than the latest release here; upgradePlan covers the rest.
func TestUpgrade(t *testing.T) {
	sys := newGoSystem("qtldr")
	r := runCLI(t, sys, "upgrade", "--check")
	if r.code != 0 || !strings.Contains(r.stdout, "qtldr v0.2.0 is available") || len(*sys.ran) != 1 {
		t.Fatalf("--check: %+v ran %v", r, *sys.ran)
	}

	sys = newGoSystem("qtldr")
	r = runCLI(t, sys, "upgrade")
	if r.code != 0 || !strings.Contains(r.stdout, "→ v0.2.0 (/home/u/go/bin/qtldr)") {
		t.Fatalf("upgrade: %+v", r)
	}
	if got := strings.Join(*sys.ran, "\n"); !strings.Contains(got, installV2) {
		t.Errorf("did not install the release:\n%s", got)
	}
	if !strings.Contains(r.stderr, "qtldr on your PATH is /bin/qtldr, not the upgraded /home/u/go/bin/qtldr") {
		t.Errorf("no PATH warning: %q", r.stderr)
	}

	r = runCLI(t, newGoSystem(""), "--json", "upgrade")
	for _, want := range []string{`"status": "upgraded"`, `"latest": "v0.2.0"`, `is not on your PATH`} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("json: missing %q in %s", want, r.stdout)
		}
	}
}

func TestUpgradeErrors(t *testing.T) {
	sys := newGoSystem("qtldr")
	sys.fail[listLatest] = true
	sys.out[listLatest] = "go: module github.com/morethancoder/qtldr: not found"
	r := runCLI(t, sys, "upgrade")
	if r.code != ExitError || !strings.Contains(r.stderr, "could not find the latest release of github.com/morethancoder/qtldr") || !strings.Contains(r.stderr, "GOPRIVATE") {
		t.Errorf("lookup: %+v", r)
	}

	sys = newGoSystem("qtldr")
	sys.fail[installV2] = true
	sys.out[installV2] = "build failed"
	if r := runCLI(t, sys, "upgrade"); r.code != ExitError || !strings.Contains(r.stderr, "go install github.com/morethancoder/qtldr/cmd/qtldr@v0.2.0 failed: build failed") {
		t.Errorf("install: %+v", r)
	}

	sys = newGoSystem("qtldr")
	sys.out[goEnv] = "not json"
	if r := runCLI(t, sys, "upgrade"); r.code != ExitError || !strings.Contains(r.stderr, "could not read go env") {
		t.Errorf("go env: %+v", r)
	}

	if r := runCLI(t, newGoSystem("qtldr"), "upgrade", "now"); r.code != ExitError || !strings.Contains(r.stderr, "upgrade takes no arguments") {
		t.Errorf("args: %+v", r)
	}
}

func TestUpgradeMessage(t *testing.T) {
	cases := []struct {
		r    upgradeResult
		want string
	}{
		{upgradeResult{Current: "v0.2.0", Latest: "v0.2.0", Status: upgradeUpToDate}, "qtldr v0.2.0 is the latest release."},
		{upgradeResult{Current: "v0.2.1-0.x", Latest: "v0.2.0", Status: upgradeAhead}, "This build (v0.2.1-0.x) is newer than the latest release (v0.2.0); nothing to do."},
	}
	for _, c := range cases {
		if got := upgradeMessage(c.r); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}
