// Package golang is the Go language provider: it loads a module with
// go/packages and turns it into a model.Graph.
package golang

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/morethancoder/qtldr/internal/config"
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

// Detect reports whether root has a go.mod.
func (*Provider) Detect(root string) bool {
	_, err := os.Stat(filepath.Join(root, "go.mod"))
	return err == nil
}

// Scan loads the packages matched by cfg.Include under root and returns the
// graph: module, packages, functions, types, external modules, imports and
// static calls, with cc, cognitive, loc and body hashes.
func (p *Provider) Scan(ctx context.Context, root string, cfg config.Project) (model.Graph, error) {
	pkgs, err := load(ctx, root, cfg.Include)
	if err != nil {
		return model.Graph{}, err
	}
	s, err := newScanner(root, cfg, pkgs)
	if err != nil {
		return model.Graph{}, err
	}
	for _, pkg := range s.pkgs {
		s.addPackage(pkg)
	}
	if err := s.addImports(ctx); err != nil {
		return model.Graph{}, err
	}
	return s.graph(), nil
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

// mainModule returns the packages of the main module and its path.
func mainModule(pkgs []*packages.Package) ([]*packages.Package, *packages.Module, error) {
	var own []*packages.Package
	var mod *packages.Module
	for _, p := range pkgs {
		if p.Module != nil && p.Module.Main {
			own = append(own, p)
			mod = p.Module
		}
	}
	if mod == nil {
		return nil, nil, errors.New("no packages of the main module matched; check [project].include in .qtldr.toml and that go.mod is at the root")
	}
	return own, mod, nil
}
