// Package theme reads the built-in themes (web/src/themes.json) and custom
// themes (.qtldr/themes/*.json) and validates them against the same schema.
package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/morethancoder/qtldr/internal/store"
	"github.com/morethancoder/qtldr/web"
)

// Theme is one palette. Custom is set for themes from .qtldr/themes/.
type Theme struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Dark   bool              `json:"dark"`
	Shiki  string            `json:"shiki,omitempty"`
	UI     map[string]string `json:"ui"`
	Grade  map[string]string `json:"grade"`
	Syntax map[string]string `json:"syntax"`
	Custom bool              `json:"custom,omitempty"`
}

var (
	uiKeys     = []string{"bg", "surface", "surface2", "border", "border2", "text", "muted", "faint", "accent"}
	gradeKeys  = []string{"red", "orange", "yellow", "green"}
	syntaxKeys = []string{"kw", "str", "num", "com", "fn", "ty", "plain"}
	hexColor   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	themeID    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
)

// Builtin returns the themes shipped in web/src/themes.json.
func Builtin() ([]Theme, error) {
	var f struct {
		Schema int     `json:"schema"`
		Themes []Theme `json:"themes"`
	}
	if err := json.Unmarshal(web.Themes, &f); err != nil {
		return nil, fmt.Errorf("web/src/themes.json: %w", err)
	}
	for _, t := range f.Themes {
		if err := Validate(t); err != nil {
			return nil, fmt.Errorf("web/src/themes.json: %w", err)
		}
	}
	return f.Themes, nil
}

// Validate checks the id, name and every color key.
func Validate(t Theme) error {
	if !themeID.MatchString(t.ID) || t.Name == "" {
		return fmt.Errorf("theme %q needs an id (lower-case letters, digits, -) and a name", t.ID)
	}
	for _, group := range []struct {
		name string
		keys []string
		vals map[string]string
	}{{"ui", uiKeys, t.UI}, {"grade", gradeKeys, t.Grade}, {"syntax", syntaxKeys, t.Syntax}} {
		for _, k := range group.keys {
			if !hexColor.MatchString(group.vals[k]) {
				return fmt.Errorf("theme %q: %s.%s must be a #rrggbb color, got %q", t.ID, group.name, k, group.vals[k])
			}
		}
	}
	return nil
}

// LoadCustom reads .qtldr/themes/*.json. Invalid files are skipped with a
// warning naming the file and the problem.
func LoadCustom(root string, builtin []Theme) ([]Theme, []string) {
	files, _ := filepath.Glob(filepath.Join(root, store.Dir, "themes", "*.json"))
	slices.Sort(files)
	var out []Theme
	var warnings []string
	for _, f := range files {
		t, err := readTheme(f)
		if err == nil && exists(builtin, t.ID) {
			err = fmt.Errorf("id %q is a built-in theme; pick another id", t.ID)
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", f, err))
			continue
		}
		t.Custom = true
		out = append(out, t)
	}
	return out, warnings
}

func readTheme(path string) (Theme, error) {
	var t Theme
	b, err := os.ReadFile(path)
	if err != nil {
		return t, err
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return t, err
	}
	return t, Validate(t)
}

func exists(ts []Theme, id string) bool {
	return slices.ContainsFunc(ts, func(t Theme) bool { return t.ID == id })
}

// All returns built-in then custom themes, plus warnings.
func All(root string) ([]Theme, []string, error) {
	b, err := Builtin()
	if err != nil {
		return nil, nil, err
	}
	custom, warnings := LoadCustom(root, b)
	return append(b, custom...), warnings, nil
}

// Find reports whether id names one of ts.
func Find(ts []Theme, id string) bool { return exists(ts, strings.TrimSpace(id)) }
