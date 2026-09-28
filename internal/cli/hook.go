package cli

import (
	"flag"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/morethancoder/qtldr/internal/hooks"
)

func hookFlags(fs *flag.FlagSet) {
	fs.Bool("claude", false, "add a PostToolUse hook to .claude/settings.json")
	fs.Bool("git", false, "add a git pre-push hook")
}

func runHook(e *env, fs *flag.FlagSet, args []string) error {
	if !slices.Equal(args, []string{"install"}) {
		return fmt.Errorf("%w: hook install [--claude] [--git]", errUsage)
	}
	claude, git := hookTargets(fs)
	root, err := e.moduleRoot()
	if err != nil {
		return err
	}
	if claude {
		if err := installClaude(e, root); err != nil {
			return err
		}
	}
	if git {
		return installGit(e, root)
	}
	return nil
}

// hookTargets reads --claude and --git; neither means both.
func hookTargets(fs *flag.FlagSet) (claude, git bool) {
	claude, git = boolFlag(fs, "claude"), boolFlag(fs, "git")
	if !claude && !git {
		return true, true
	}
	return claude, git
}

func installClaude(e *env, root string) error {
	path, changed, err := hooks.InstallClaude(root)
	if err != nil {
		return err
	}
	if changed {
		fmt.Fprintf(e.stdout, "Added the qtldr PostToolUse hook to %s.\n", path)
	} else {
		fmt.Fprintf(e.stdout, "%s already has the qtldr hook.\n", path)
	}
	fmt.Fprintf(e.stdout, "\nAdd this to CLAUDE.md:\n\n%s\n", hooks.Snippet)
	return nil
}

func installGit(e *env, root string) error {
	hookPath, err := gitOutput(root, "rev-parse", "--git-path", "hooks/pre-push")
	if err != nil {
		return fmt.Errorf("--git needs a git repository at %s: %w", root, err)
	}
	if !filepath.IsAbs(hookPath) {
		hookPath = filepath.Join(root, hookPath)
	}
	prefix, err := gitOutput(root, "rev-parse", "--show-prefix")
	if err != nil {
		return err
	}
	if err := hooks.InstallGit(hookPath, prefix); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "Wrote %s: it runs `qtldr check --changed` before each push.\n", hookPath)
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}
