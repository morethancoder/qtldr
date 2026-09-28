// Package analysis runs the whole pipeline: scan, coverage (optional),
// churn, metrics, snapshot. The CLI, `check`, the web server and the MCP
// server all call it, so they always agree.
package analysis

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/coverage"
	"github.com/morethancoder/qtldr/internal/gitx"
	"github.com/morethancoder/qtldr/internal/lang"
	"github.com/morethancoder/qtldr/internal/lang/golang"
	"github.com/morethancoder/qtldr/internal/metrics"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/store"
)

// ToolVersion is written into snapshots.
const ToolVersion = "0.1.0"

// Options configure one run.
type Options struct {
	Root   string
	Config config.Config
	// Patterns override [project].include when set.
	Patterns []string
	// Coverage runs the tests. CoveragePackages limits the run to these
	// package IDs; nil means every scanned package.
	Coverage         bool
	CoveragePackages []model.ID
	// Scope, when set, picks the functions of interest after the scan; with
	// Coverage, only their packages' tests run.
	Scope func(g model.Graph) ([]model.ID, error)
	// Now is the run time (tests pass a fixed one).
	Now time.Time
	// Progress receives short progress lines; may be nil.
	Progress func(msg string)
	// Provider defaults to the Go provider.
	Provider lang.Provider
}

// Result is a finished run.
type Result struct {
	Snapshot model.Snapshot
	// Failed lists packages whose tests failed during this run's coverage.
	Failed []model.ID
	// Scope is what Options.Scope returned.
	Scope []model.ID
	// LogPath is the coverage log written when tests failed ("" otherwise).
	LogPath  string
	Warnings []string
}

// Run executes the pipeline and writes .qtldr/snapshot.json.
func Run(ctx context.Context, opt Options) (Result, error) {
	opt = opt.withDefaults()
	opt.progress("Scanning packages…")
	g, err := opt.Provider.Scan(ctx, opt.Root, opt.project())
	if err != nil {
		return Result{}, err
	}
	var res Result
	if err := opt.applyScope(g, &res); err != nil {
		return Result{}, err
	}
	cache, err := coverageData(ctx, opt, g, &res)
	if err != nil {
		return Result{}, err
	}
	in := metrics.Inputs{
		Coverage:       cache.Resolve(g),
		CoverageErrors: cache.PackageErrors,
		PurityAllow:    allowIDs(g, opt.Config.Purity.Allow, &res),
	}
	addChurn(ctx, opt, &in, &res)
	res.Snapshot = snapshot(metrics.Compute(g, in), gitx.Info(ctx, opt.Root), opt.Now, cache.RanAt)
	return res, store.WriteSnapshot(opt.Root, res.Snapshot)
}

func (o Options) withDefaults() Options {
	if o.Provider == nil {
		o.Provider = golang.New()
	}
	if o.Now.IsZero() {
		o.Now = time.Now().UTC().Truncate(time.Second)
	}
	return o
}

func (o Options) project() config.Project {
	p := o.Config.Project
	if len(o.Patterns) > 0 {
		p.Include = o.Patterns
	}
	return p
}

// applyScope runs Options.Scope and narrows the coverage run to its packages.
func (o *Options) applyScope(g model.Graph, res *Result) error {
	if o.Scope == nil {
		return nil
	}
	scope, err := o.Scope(g)
	if err != nil {
		return err
	}
	res.Scope = scope
	o.CoveragePackages = append([]model.ID{}, PackagesOf(g, scope)...) // non-nil: empty scope runs nothing
	return nil
}

func (o Options) progress(msg string) {
	if o.Progress != nil {
		o.Progress(msg)
	}
}

// coverageData reads the coverage cache and, with Options.Coverage, runs the
// tests first and updates it.
func coverageData(ctx context.Context, opt Options, g model.Graph, res *Result) (coverage.Cache, error) {
	cache, err := store.ReadCoverageCache(opt.Root)
	if err != nil || !opt.Coverage {
		return cache, err
	}
	pkgs := opt.CoveragePackages
	if pkgs == nil {
		pkgs = packageIDs(g)
	}
	if len(pkgs) == 0 {
		return cache, nil
	}
	return cache, runCoverage(ctx, opt, g, pkgs, &cache, res)
}

// runCoverage runs the tests of pkgs, maps the profile and updates the cache.
func runCoverage(ctx context.Context, opt Options, g model.Graph, pkgs []model.ID, cache *coverage.Cache, res *Result) error {
	opt.progress(fmt.Sprintf("Running coverage… %d packages", len(pkgs)))
	run, err := opt.Provider.CoverageRun(ctx, opt.Root, idStrings(pkgs), opt.Config.Coverage, store.CachePath(opt.Root, "coverage.out"))
	if err != nil {
		return withLog(opt, run.Output, err)
	}
	failed, err := recordFailures(opt, run, res)
	if err != nil {
		return err
	}
	measured := opt.Provider.MapCoverage(g, run.Profile.Without(dirsOf(g, res.Failed)))
	cache.Update(g, measured, pkgs, failed, opt.Now)
	return store.WriteCoverageCache(opt.Root, *cache)
}

// withLog saves the command output and names the log in the error.
func withLog(opt Options, output []byte, err error) error {
	log, lerr := store.WriteLog(opt.Root, "coverage", opt.Now, output)
	if lerr != nil {
		return err
	}
	return fmt.Errorf("%w (full output: %s)", err, log)
}

// recordFailures logs the output of a run with failing packages and returns
// the reason per failed package.
func recordFailures(opt Options, run coverage.RunResult, res *Result) (map[model.ID]string, error) {
	failed := map[model.ID]string{}
	if len(run.Failed) == 0 {
		return failed, nil
	}
	log, err := store.WriteLog(opt.Root, "coverage", opt.Now, run.Output)
	if err != nil {
		return nil, err
	}
	res.LogPath = log
	for _, p := range run.Failed {
		failed[model.ID(p)] = "tests failed; output in " + log
		res.Failed = append(res.Failed, model.ID(p))
	}
	return failed, nil
}

func addChurn(ctx context.Context, opt Options, in *metrics.Inputs, res *Result) {
	churn, err := gitx.ReadChurn(ctx, opt.Root, opt.Config.Churn.WindowMonths)
	switch {
	case errors.Is(err, gitx.ErrNotRepo):
		res.Warnings = append(res.Warnings, "not a git repository: churn, --changed and base-ref scope are not available")
	case err != nil:
		res.Warnings = append(res.Warnings, "churn not measured: "+err.Error())
	default:
		in.FileChurn, in.PackageChurn = churn.Files, churn.Dirs
	}
}

// allowIDs resolves [purity].allow entries (full IDs or unique suffixes).
func allowIDs(g model.Graph, allow []string, res *Result) []model.ID {
	var ids []model.ID
	for _, a := range allow {
		id, err := model.Resolve(g.IDs(), a)
		if err != nil {
			res.Warnings = append(res.Warnings, "[purity].allow: "+err.Error())
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func snapshot(g model.Graph, git *model.GitInfo, at time.Time, coverageAt *time.Time) model.Snapshot {
	s := model.Snapshot{
		Schema: model.SchemaVersion, ToolVersion: ToolVersion, Generated: at, Git: git,
		Runs: model.Runs{Structure: &at, Coverage: coverageAt}, Graph: g,
	}
	for _, n := range g.Nodes {
		if n.Kind == model.KindModule {
			s.Module = string(n.ID)
		}
	}
	return s
}

func packageIDs(g model.Graph) []model.ID {
	var ids []model.ID
	for _, n := range g.Nodes {
		if n.Kind == model.KindPackage {
			ids = append(ids, n.ID)
		}
	}
	slices.Sort(ids)
	return ids
}

// PackagesOf returns the packages that contain the given nodes (packages
// map to themselves).
func PackagesOf(g model.Graph, ids []model.ID) []model.ID {
	var pkgs []model.ID
	for _, id := range ids {
		n, ok := g.Node(id)
		switch {
		case !ok:
		case n.Kind == model.KindPackage:
			pkgs = append(pkgs, n.ID)
		case n.Parent != "":
			pkgs = append(pkgs, n.Parent)
		}
	}
	slices.Sort(pkgs)
	return slices.Compact(pkgs)
}

func dirsOf(g model.Graph, pkgs []model.ID) []string {
	var dirs []string
	for _, p := range pkgs {
		if n, ok := g.Node(p); ok {
			dirs = append(dirs, n.Dir)
		}
	}
	return dirs
}

func idStrings(ids []model.ID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

// RelFile turns a path (absolute or relative to cwd) into a path relative to
// root with forward slashes; ok is false when it is outside root.
func RelFile(root, cwd, p string) (string, bool) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	if real, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		p = filepath.Join(real, filepath.Base(p))
	}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
