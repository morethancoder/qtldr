// Package gitx reads git state: HEAD and dirtiness now; changed hunks and
// churn from M1.
package gitx

import (
	"context"
	"os/exec"
	"strings"

	"github.com/morethancoder/qtldr/internal/model"
)

// Info returns HEAD and whether the work tree has changes, or nil when root
// is not inside a git work tree (or git is not installed).
func Info(ctx context.Context, root string) *model.GitInfo {
	if out, err := git(ctx, root, "rev-parse", "--is-inside-work-tree"); err != nil || out != "true" {
		return nil
	}
	head, err := git(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		head = "" // a repo with no commits yet
	}
	status, err := git(ctx, root, "status", "--porcelain")
	return &model.GitInfo{Head: head, Dirty: err != nil || status != ""}
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
