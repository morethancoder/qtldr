package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"

	"golang.org/x/mod/semver"
)

// Releases are Go module versions; upgrade installs them with go install.
const (
	modulePath = "github.com/morethancoder/qtldr"
	cmdPath    = modulePath + "/cmd/qtldr"
)

// What upgrade found or did.
const (
	upgradeUpToDate  = "up to date"
	upgradeAvailable = "available"
	upgradeAhead     = "ahead" // this build is newer than the latest release
	upgradeDone      = "upgraded"
)

type upgradeResult struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Status  string `json:"status"`
	Path    string `json:"path,omitempty"`
	Warning string `json:"warning,omitempty"`
}

func upgradeFlags(fs *flag.FlagSet) {
	fs.Bool("check", false, "only say whether a newer release exists")
}

func runUpgrade(e *env, fs *flag.FlagSet, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%w: upgrade takes no arguments", errUsage)
	}
	latest, err := latestRelease(e)
	if err != nil {
		return err
	}
	res := upgradeResult{Current: version(debug.ReadBuildInfo()), Latest: latest}
	res.Status = upgradePlan(res.Current, latest)
	if res.Status == upgradeAvailable && !fs.Lookup("check").Value.(flag.Getter).Get().(bool) {
		e.info("Installing qtldr %s…", latest)
		if res.Path, err = installRelease(e, latest); err != nil {
			return err
		}
		res.Status = upgradeDone
		res.Warning = pathWarning(e, res.Path)
	}
	if e.g.json {
		return e.printJSON(res)
	}
	fmt.Fprintln(e.stdout, upgradeMessage(res))
	if res.Warning != "" {
		fmt.Fprintln(e.stderr, "warning: "+res.Warning)
	}
	return nil
}

// upgradePlan compares this build with the latest release. A build without a
// release version (a local "-dev" build) is replaced by the release.
func upgradePlan(current, latest string) string {
	if !semver.IsValid(current) {
		return upgradeAvailable
	}
	switch c := semver.Compare(current, latest); {
	case c == 0:
		return upgradeUpToDate
	case c > 0:
		return upgradeAhead
	}
	return upgradeAvailable
}

func upgradeMessage(r upgradeResult) string {
	switch r.Status {
	case upgradeUpToDate:
		return fmt.Sprintf("qtldr %s is the latest release.", r.Current)
	case upgradeAhead:
		return fmt.Sprintf("This build (%s) is newer than the latest release (%s); nothing to do.", r.Current, r.Latest)
	case upgradeDone:
		return fmt.Sprintf("Upgraded qtldr %s → %s (%s).", r.Current, r.Latest, r.Path)
	}
	return fmt.Sprintf("qtldr %s is available (you have %s). Run: qtldr upgrade", r.Latest, r.Current)
}

// latestRelease asks the Go module proxy (or the repository, for private
// modules) for the newest release.
func latestRelease(e *env) (string, error) {
	out, err := e.sys.Output(e.ctx, "go", "list", "-m", "-f", "{{.Version}}", modulePath+"@latest")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	v := strings.TrimSpace(lines[len(lines)-1])
	if err != nil || !semver.IsValid(v) {
		return "", fmt.Errorf("could not find the latest release of %s:\n%s\nCheck the network; for a private repository set GOPRIVATE=%s", modulePath, out, modulePath)
	}
	return v, nil
}

// installRelease builds release v into GOBIN and returns the binary's path.
func installRelease(e *env, v string) (string, error) {
	if out, err := e.sys.Output(e.ctx, "go", "install", cmdPath+"@"+v); err != nil {
		return "", fmt.Errorf("go install %s@%s failed: %s", cmdPath, v, out)
	}
	out, err := e.sys.Output(e.ctx, "go", "env", "-json", "GOBIN", "GOPATH", "GOEXE")
	var goenv struct{ GOBIN, GOPATH, GOEXE string }
	if err != nil || json.Unmarshal([]byte(out), &goenv) != nil {
		return "", fmt.Errorf("installed %s, but could not read go env to say where: %s", v, out)
	}
	return binPath(goenv.GOBIN, goenv.GOPATH, goenv.GOEXE), nil
}

// binPath is where go install puts qtldr: GOBIN, else the first GOPATH's bin.
func binPath(gobin, gopath, exe string) string {
	dir := gobin
	if dir == "" {
		first, _, _ := strings.Cut(gopath, string(filepath.ListSeparator))
		dir = filepath.Join(first, "bin")
	}
	return filepath.Join(dir, "qtldr"+exe)
}

// pathWarning says when running `qtldr` would not start the upgraded binary.
func pathWarning(e *env, installed string) string {
	found, err := e.sys.LookPath("qtldr")
	if err != nil {
		return fmt.Sprintf("qtldr is not on your PATH; add %s to PATH", filepath.Dir(installed))
	}
	if resolvePath(found) == resolvePath(installed) {
		return ""
	}
	return fmt.Sprintf("qtldr on your PATH is %s, not the upgraded %s; remove the old one or put %s first in PATH", found, installed, filepath.Dir(installed))
}

func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}
