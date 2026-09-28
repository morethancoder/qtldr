package coverage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/morethancoder/qtldr/internal/argv"
	"github.com/morethancoder/qtldr/internal/config"
)

// RunResult is the outcome of one test command run.
type RunResult struct {
	Profile Profile
	// Failed lists import paths whose tests or build failed.
	Failed []string
	// Output is the combined stdout and stderr of the command.
	Output []byte
	// Args is the command that ran.
	Args []string
}

// Run executes the configured coverage command for pkgs (import paths or
// patterns) in root, writing the profile to profilePath. Failing packages are
// reported in Failed, not as an error; an error means no usable profile.
func Run(ctx context.Context, root string, cfg config.Coverage, pkgs []string, profilePath string) (RunResult, error) {
	args, err := BuildArgs(cfg.Command, cfg.Coverpkg, profilePath, pkgs)
	if err != nil {
		return RunResult{}, err
	}
	if cfg.Timeout.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout.Duration)
		defer cancel()
	}
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		return RunResult{}, err
	}
	_ = os.Remove(profilePath)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = root
	out, runErr := cmd.CombinedOutput()
	res := RunResult{Output: out, Args: args, Failed: FailedPackages(string(out))}
	if ctx.Err() != nil {
		return res, fmt.Errorf("coverage run stopped after %v ([coverage].timeout): %s", cfg.Timeout.Duration, strings.Join(args, " "))
	}
	f, err := os.Open(profilePath)
	if err != nil {
		return res, fmt.Errorf("`%s` wrote no coverage profile (%v): %s", strings.Join(args, " "), runErr, tail(out, 20))
	}
	defer f.Close()
	res.Profile, err = Parse(f)
	return res, err
}

// BuildArgs splits the command template into argv (no shell) and fills
// {profile} and {packages}. coverpkg "module" adds -coverpkg=./... before the
// packages.
func BuildArgs(template, coverpkg, profile string, pkgs []string) ([]string, error) {
	words, err := argv.Split(template)
	if err != nil {
		return nil, fmt.Errorf("[coverage].command: %w", err)
	}
	if !slices.Contains(words, "{packages}") {
		return nil, errors.New("[coverage].command must contain {packages}")
	}
	var args []string
	for _, w := range words {
		switch {
		case w == "{packages}" && coverpkg == "module":
			args = append(append(args, "-coverpkg=./..."), pkgs...)
		case w == "{packages}":
			args = append(args, pkgs...)
		default:
			args = append(args, strings.ReplaceAll(w, "{profile}", profile))
		}
	}
	return args, nil
}

var failLine = regexp.MustCompile(`(?m)^FAIL\t(\S+)`)

// FailedPackages finds "FAIL\t<pkg>" summary lines in go test output
// (including "[build failed]" and "[setup failed]").
func FailedPackages(out string) []string {
	var pkgs []string
	for _, m := range failLine.FindAllStringSubmatch(out, -1) {
		pkgs = append(pkgs, m[1])
	}
	slices.Sort(pkgs)
	return slices.Compact(pkgs)
}

func tail(b []byte, n int) string {
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return string(bytes.Join(lines, []byte("\n")))
}
