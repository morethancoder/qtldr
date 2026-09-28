// Package golang is the Go language provider: it loads a module with
// go/packages and turns it into a model.Graph.
package golang

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/coverage"
	"github.com/morethancoder/qtldr/internal/lang"
	"github.com/morethancoder/qtldr/internal/model"
)

// loadMode is the smallest mode that gives syntax, types, type info, imports
// and module info for the analyzed packages. NeedDeps is deliberately absent:
// it would type-check every dependency from source. See docs/decisions.md.
const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedModule

// Provider reads Go modules.
type Provider struct{}

// New returns the Go provider.
func New() *Provider { return &Provider{} }

// Name returns "go".
func (*Provider) Name() string { return "go" }

// Detect reports whether root has a go.mod or a go.work.
func (*Provider) Detect(root string) bool {
	return exists(filepath.Join(root, "go.mod")) || exists(filepath.Join(root, "go.work"))
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Scan loads the packages matched by cfg.Include under root and returns the
// graph: module, packages, functions, types, external modules, imports and
// static calls, with cc, cognitive, loc and body hashes.
func (p *Provider) Scan(ctx context.Context, root string, cfg config.Project) (model.Graph, error) {
	patterns, err := Patterns(ctx, root, cfg.Include)
	if err != nil {
		return model.Graph{}, err
	}
	cfg.Include = patterns
	pkgs, err := load(ctx, root, patterns)
	if err != nil {
		return model.Graph{}, err
	}
	s, err := newScanner(root, cfg, pkgs)
	if err != nil {
		return model.Graph{}, err
	}
	if err := s.build(ctx); err != nil {
		return model.Graph{}, err
	}
	return s.graph(), nil
}

// build adds packages, declarations, implements, imports and (with
// [project].calls = "vta") resolved dynamic calls.
func (s *scanner) build(ctx context.Context) error {
	for _, pkg := range s.pkgs {
		s.addPackage(pkg)
	}
	s.addImplements()
	if err := s.addImports(ctx); err != nil {
		return err
	}
	if s.cfg.Calls != "vta" {
		return nil
	}
	return s.addVTAEdges(ctx, s.cfg.Include)
}

// Source returns lines from..to (1-based, inclusive) of file under root.
func (*Provider) Source(_ context.Context, root, file string, from, to int) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return "", fmt.Errorf("read source %s: %w", file, err)
	}
	lines := strings.Split(string(b), "\n")
	if from < 1 || to > len(lines) || from > to {
		return "", fmt.Errorf("%s has %d lines; lines %d–%d are out of range", file, len(lines), from, to)
	}
	return strings.Join(lines[from-1:to], "\n"), nil
}

func load(ctx context.Context, root string, patterns []string) ([]*packages.Package, error) {
	cfg := &packages.Config{Mode: loadMode, Dir: root, Context: ctx, Tests: false}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load Go packages %s in %s: %w (does `go list %s` work there?)",
			strings.Join(patterns, " "), root, err, strings.Join(patterns, " "))
	}
	return pkgs, nil
}

// mainModules returns the packages of the main module(s) (several in a
// go.work workspace) and the modules, sorted by path.
func mainModules(pkgs []*packages.Package) ([]*packages.Package, []*packages.Module, error) {
	var own []*packages.Package
	mods := map[string]*packages.Module{}
	for _, p := range pkgs {
		if p.Module != nil && p.Module.Main {
			own = append(own, p)
			mods[p.Module.Path] = p.Module
		}
	}
	if len(mods) == 0 {
		return nil, nil, errors.New("no packages of the main module matched; check [project].include in .qtldr.toml and that go.mod (or go.work) is at the root")
	}
	var list []*packages.Module
	for _, k := range slices.Sorted(maps.Keys(mods)) {
		list = append(list, mods[k])
	}
	return own, list, nil
}

// Patterns expands the include patterns for a go.work root (without a
// go.mod of its own): "./..." cannot match there, so it becomes one
// "<use>/..." per workspace module (docs/decisions.md).
func Patterns(ctx context.Context, root string, include []string) ([]string, error) {
	if exists(filepath.Join(root, "go.mod")) || !exists(filepath.Join(root, "go.work")) {
		return include, nil
	}
	cmd := exec.CommandContext(ctx, "go", "work", "edit", "-json")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("read go.work in %s: %w", root, err)
	}
	var work struct{ Use []struct{ DiskPath string } }
	if err := json.Unmarshal(out, &work); err != nil {
		return nil, fmt.Errorf("go work edit -json: %w", err)
	}
	uses := make([]string, len(work.Use))
	for i, u := range work.Use {
		uses[i] = u.DiskPath
	}
	return ExpandWorkPatterns(include, uses), nil
}

// ExpandWorkPatterns replaces "./..." and "." with each workspace module
// directory (pure).
func ExpandWorkPatterns(include, uses []string) []string {
	var out []string
	for _, p := range include {
		switch p {
		case "./...":
			for _, u := range uses {
				out = append(out, "./"+strings.TrimPrefix(path.Clean(u), "./")+"/...")
			}
		case ".":
			for _, u := range uses {
				out = append(out, "./"+strings.TrimPrefix(path.Clean(u), "./"))
			}
		default:
			out = append(out, p)
		}
	}
	return out
}

// CoverageRun runs the configured go test command with coverage for pkgs.
func (*Provider) CoverageRun(ctx context.Context, root string, pkgs []string, cfg config.Coverage, profilePath string) (coverage.RunResult, error) {
	return coverage.Run(ctx, root, cfg, pkgs, profilePath)
}

// MapCoverage attributes a Go coverage profile (import-path file names) to
// the functions of g.
func (*Provider) MapCoverage(g model.Graph, p coverage.Profile) map[model.ID]model.Coverage {
	dirs := map[string]string{} // module path → directory relative to the root
	for _, n := range g.Nodes {
		if n.Kind == model.KindModule {
			dirs[string(n.ID)] = n.Dir
		}
	}
	return coverage.Map(p.RelativizeModules(dirs), coverage.FuncsOf(g))
}

var _ lang.Provider = (*Provider)(nil)
