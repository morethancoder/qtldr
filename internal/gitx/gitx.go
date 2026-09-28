// Package gitx reads git state: HEAD, churn, and the lines changed against a
// base ref. Parsers are pure; the functions that run git are thin wrappers.
package gitx

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/morethancoder/qtldr/internal/model"
)

// ErrNotRepo means the directory is not inside a git work tree.
var ErrNotRepo = errors.New("not a git repository")

// Info returns HEAD and whether the work tree has changes, or nil when root
// is not inside a git work tree (or git is not installed).
func Info(ctx context.Context, root string) *model.GitInfo {
	if !IsRepo(ctx, root) {
		return nil
	}
	head, err := git(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		head = "" // a repo with no commits yet
	}
	status, err := git(ctx, root, "status", "--porcelain", "--", ".")
	return &model.GitInfo{Head: head, Dirty: err != nil || status != ""}
}

// IsRepo reports whether root is inside a git work tree.
func IsRepo(ctx context.Context, root string) bool {
	out, err := git(ctx, root, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && len(ee.Stderr) > 0 {
		err = fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
	}
	return strings.TrimSpace(string(out)), err
}

// Churn counts commits per file and per directory in the last months, for
// paths under root (relative to root).
type Churn struct {
	Files map[string]int
	Dirs  map[string]int
}

// ReadChurn runs one git log pass over root. It returns ErrNotRepo outside a
// git work tree.
func ReadChurn(ctx context.Context, root string, months int) (Churn, error) {
	if !IsRepo(ctx, root) {
		return Churn{}, ErrNotRepo
	}
	out, err := git(ctx, root, "log", fmt.Sprintf("--since=%d months ago", months),
		"--format=%x00commit", "--name-only", "--relative", "--no-renames", "--", ".")
	if err != nil {
		return Churn{}, err
	}
	return ParseChurn(out), nil
}

// ParseChurn reads `git log --format=%x00commit --name-only` output. A file
// counts once per commit; a directory counts once per commit that touched
// any file directly in it ("." for the root).
func ParseChurn(out string) Churn {
	c := Churn{Files: map[string]int{}, Dirs: map[string]int{}}
	for _, commit := range strings.Split(out, "\x00commit") {
		dirs := map[string]bool{}
		for _, f := range strings.Split(commit, "\n") {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			c.Files[f]++
			dirs[path.Dir(f)] = true
		}
		for d := range dirs {
			c.Dirs[d]++
		}
	}
	return c
}

// Range is an inclusive line range in the new version of a file. Deleted is
// true for a pure deletion between lines Start and Start+1.
type Range struct {
	Start, End int
	Deleted    bool
}

// Changes are the files and line ranges that differ from a base ref, plus
// untracked files (paths relative to root).
type Changes struct {
	Base      string
	Hunks     map[string][]Range
	Untracked []string
}

// ReadChanges diffs the work tree under root against base.
func ReadChanges(ctx context.Context, root, base string) (Changes, error) {
	if !IsRepo(ctx, root) {
		return Changes{}, ErrNotRepo
	}
	if _, err := git(ctx, root, "rev-parse", "--verify", "--quiet", base+"^{commit}"); err != nil {
		return Changes{}, fmt.Errorf("base ref %q not found; set [project].base_ref in .qtldr.toml or use --all", base)
	}
	diff, err := git(ctx, root, "diff", "-U0", "--relative", "--no-color", "--no-ext-diff", base, "--", ".")
	if err != nil {
		return Changes{}, err
	}
	untracked, err := git(ctx, root, "ls-files", "--others", "--exclude-standard", "--", ".")
	if err != nil {
		return Changes{}, err
	}
	ch := Changes{Base: base, Hunks: ParseDiff(diff)}
	if untracked != "" {
		ch.Untracked = strings.Split(untracked, "\n")
	}
	return ch, nil
}

// ParseDiff reads `git diff -U0` output into new-file line ranges per file.
func ParseDiff(diff string) map[string][]Range {
	hunks := map[string][]Range{}
	file := ""
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			file = diffPath(strings.TrimPrefix(line, "+++ "))
		case strings.HasPrefix(line, "@@ ") && file != "":
			if r, ok := parseHunk(line); ok {
				hunks[file] = append(hunks[file], r)
			}
		}
	}
	return hunks
}

func diffPath(p string) string {
	if p == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(p, "b/")
}

// parseHunk reads "@@ -a,b +c,d @@ ..." and returns the new-side range.
func parseHunk(line string) (Range, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 || !strings.HasPrefix(fields[2], "+") {
		return Range{}, false
	}
	startText, countText, hasCount := strings.Cut(fields[2][1:], ",")
	start, err := strconv.Atoi(startText)
	if err != nil {
		return Range{}, false
	}
	count := 1
	if hasCount {
		if count, err = strconv.Atoi(countText); err != nil {
			return Range{}, false
		}
	}
	if count == 0 {
		return Range{Start: start, End: start, Deleted: true}, true
	}
	return Range{Start: start, End: start + count - 1}, true
}

// Touches reports whether r changes a function spanning lines [from, to]. A
// deletion counts when it sits strictly inside the function.
func (r Range) Touches(from, to int) bool {
	if r.Deleted {
		return r.Start >= from && r.Start+1 <= to
	}
	return r.Start <= to && r.End >= from
}
