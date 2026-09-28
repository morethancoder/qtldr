// Package config loads and validates .qtldr.toml.
package config

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// FileName is the config file at the module root.
const FileName = ".qtldr.toml"

// DefaultTOML is the file `qtldr init` writes.
//
//go:embed default.toml
var DefaultTOML string

// Config is the whole of .qtldr.toml.
type Config struct {
	Project    Project    `toml:"project"`
	Coverage   Coverage   `toml:"coverage"`
	Mutation   Mutation   `toml:"mutation"`
	Thresholds Thresholds `toml:"thresholds"`
	Purity     Purity     `toml:"purity"`
	Churn      Churn      `toml:"churn"`
	UI         UI         `toml:"ui"`
	Editor     Editor     `toml:"editor"`
	Agent      Agent      `toml:"agent"`
}

// Project says what to analyze.
type Project struct {
	Include         []string `toml:"include"`
	Exclude         []string `toml:"exclude"`
	BaseRef         string   `toml:"base_ref"`
	ExternalModules string   `toml:"external_modules"`
	ShowStdlib      bool     `toml:"show_stdlib"`
}

// Coverage says how tests are run for coverage.
type Coverage struct {
	Command  string   `toml:"command"`
	Coverpkg string   `toml:"coverpkg"`
	Timeout  Duration `toml:"timeout"`
}

// Mutation configures the mutation engine.
type Mutation struct {
	Engine       string   `toml:"engine"`
	Args         []string `toml:"args"`
	Timeout      Duration `toml:"timeout"`
	MaxFunctions int      `toml:"max_functions"`
}

// Thresholds are the limits used by `qtldr check` and UI problem chips.
type Thresholds struct {
	CrapMax       float64  `toml:"crap_max"`
	CognitiveMax  int      `toml:"cognitive_max"`
	CoverageMin   float64  `toml:"coverage_min"`
	MutationMin   float64  `toml:"mutation_min"`
	CriticalPaths []string `toml:"critical_paths"`
}

// Purity overrides the purity heuristic.
type Purity struct {
	Allow []string `toml:"allow"`
}

// Churn configures the git history window.
type Churn struct {
	WindowMonths int `toml:"window_months"`
}

// UI configures the web UI.
type UI struct {
	Theme      string `toml:"theme"`
	ThemeLight string `toml:"theme_light"`
	ThemeDark  string `toml:"theme_dark"`
	Port       int    `toml:"port"`
}

// Editor configures "open in editor".
type Editor struct {
	Preset     string `toml:"preset"`
	Command    string `toml:"command"`
	NvimSocket string `toml:"nvim_socket"`
	Terminal   string `toml:"terminal"`
}

// Agent configures "Send to agent".
type Agent struct {
	TmuxTarget string `toml:"tmux_target"`
}

// Duration is a time.Duration written as "10m" in TOML.
type Duration struct{ time.Duration }

// UnmarshalText parses a Go duration string.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("%q is not a duration (examples: 90s, 10m, 1h)", b)
	}
	d.Duration = v
	return nil
}

// MarshalText formats the duration.
func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// Editor presets.
var Presets = []string{"vscode", "cursor", "zed", "goland", "nvim-remote", "vim-tmux", "custom"}

// Default returns the configuration used when .qtldr.toml is absent.
func Default() Config {
	var c Config
	if _, err := toml.Decode(DefaultTOML, &c); err != nil {
		panic("config: default.toml does not parse: " + err.Error())
	}
	return c
}

// Load reads path over the defaults. A missing file yields the defaults.
// Unknown keys inside known tables are returned as warnings.
func Load(path string) (Config, []string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil, nil
	}
	if err != nil {
		return Config{}, nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(path, string(b))
}

// Parse decodes TOML text over the defaults and validates it. name is used in
// messages.
func Parse(name, text string) (Config, []string, error) {
	c := Default()
	md, err := toml.Decode(text, &c)
	if err != nil {
		return Config{}, nil, fmt.Errorf("%s: %w", name, err)
	}
	warnings, err := unknownKeys(name, md.Undecoded())
	if err != nil {
		return Config{}, nil, err
	}
	if err := c.validate(); err != nil {
		return Config{}, nil, fmt.Errorf("%s: %w", name, err)
	}
	return c, warnings, nil
}

var tables = []string{"project", "coverage", "mutation", "thresholds", "purity", "churn", "ui", "editor", "agent"}

// unknownKeys turns undecoded keys into an error (unknown table or top-level
// key) or warnings (unknown key inside a known table).
func unknownKeys(name string, keys []toml.Key) ([]string, error) {
	var warnings []string
	for _, k := range keys {
		if len(k) == 1 || !slices.Contains(tables, k[0]) {
			return nil, fmt.Errorf("%s: unknown table or key %q; known tables: %s", name, k[0], strings.Join(tables, ", "))
		}
		warnings = append(warnings, fmt.Sprintf("%s: unknown key %q in [%s]; it is ignored", name, k[1], k[0]))
	}
	return warnings, nil
}

func (c Config) validate() error {
	checks := []struct {
		key, val string
		allowed  []string
	}{
		{"project.external_modules", c.Project.ExternalModules, []string{"collapsed", "hidden"}},
		{"coverage.coverpkg", c.Coverage.Coverpkg, []string{"own", "module"}},
		{"mutation.engine", c.Mutation.Engine, []string{"gremlins"}},
		{"editor.preset", c.Editor.Preset, Presets},
	}
	for _, ch := range checks {
		if !slices.Contains(ch.allowed, ch.val) {
			return fmt.Errorf("%s = %q; use one of: %s", ch.key, ch.val, strings.Join(ch.allowed, ", "))
		}
	}
	if c.Editor.Preset == "custom" && c.Editor.Command == "" {
		return errors.New(`editor.preset = "custom" needs editor.command, e.g. "nvim +{line} {file}"`)
	}
	return nil
}
