package cli

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/store"
)

// ignoreRules are the .gitignore lines init adds (PLAN.md §5.6).
var ignoreRules = []string{".qtldr/snapshot.json", ".qtldr/focus.json", ".qtldr/cache/", ".qtldr/logs/"}

func runInit(e *env, _ *flag.FlagSet, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%w: init takes no arguments", errUsage)
	}
	root, err := e.moduleRoot()
	if err != nil {
		return err
	}
	if err := writeConfig(e, filepath.Join(root, config.FileName)); err != nil {
		return err
	}
	if err := addIgnoreRules(e, filepath.Join(root, ".gitignore")); err != nil {
		return err
	}
	return doctor(e, e.editorConfig())
}

func writeConfig(e *env, path string) error {
	if _, err := os.Stat(path); err == nil {
		e.info("kept existing %s", path)
		return nil
	}
	if err := store.WriteFile(path, []byte(config.DefaultTOML)); err != nil {
		return err
	}
	e.info("wrote %s", path)
	return nil
}

// addIgnoreRules appends the rules that .gitignore does not have yet.
func addIgnoreRules(e *env, path string) error {
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	text := string(b)
	missing := missingLines(text, ignoreRules)
	if len(missing) == 0 {
		return nil
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "# qtldr\n" + strings.Join(missing, "\n") + "\n"
	if err := store.WriteFile(path, []byte(text)); err != nil {
		return err
	}
	e.info("added %d rules to %s", len(missing), path)
	return nil
}

func missingLines(text string, want []string) []string {
	have := strings.Split(text, "\n")
	for i := range have {
		have[i] = strings.TrimSpace(have[i])
	}
	var missing []string
	for _, w := range want {
		if !slices.Contains(have, w) {
			missing = append(missing, w)
		}
	}
	return missing
}
