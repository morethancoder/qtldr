package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"github.com/morethancoder/qtldr/internal/analysis"
	"github.com/morethancoder/qtldr/internal/check"
	"github.com/morethancoder/qtldr/internal/gitx"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/store"
)

// analyzeSummary is what analyze prints.
type analyzeSummary struct {
	Module    string     `json:"module"`
	Packages  int        `json:"packages"`
	Functions int        `json:"functions"`
	Types     int        `json:"types"`
	Coverage  bool       `json:"coverage_ran"`
	Failed    []model.ID `json:"tests_failed,omitempty"`
	Log       string     `json:"log,omitempty"`
	Errors    []string   `json:"errors,omitempty"`
	Warnings  []string   `json:"warnings,omitempty"`
	Snapshot  string     `json:"snapshot"`
	Seconds   float64    `json:"seconds"`
}

func analyzeFlags(fs *flag.FlagSet) {
	fs.Bool("coverage", false, "run the tests with coverage (slower)")
	fs.Bool("changed", false, "with --coverage: only test packages with functions changed since [project].base_ref")
}

func runAnalyze(e *env, fs *flag.FlagSet, args []string) error {
	root, cfg, err := e.setup()
	if err != nil {
		return err
	}
	opt := analysis.Options{Root: root, Config: cfg, Patterns: args, Coverage: boolFlag(fs, "coverage"), Progress: e.progressLine}
	if boolFlag(fs, "changed") {
		opt.Scope = changedScope(e, root, cfg.Project.BaseRef)
	}
	start := time.Now()
	res, err := analysis.Run(e.ctx, opt)
	if err != nil {
		return err
	}
	sum := summarize(res, store.SnapshotPath(root), time.Since(start))
	sum.Coverage = opt.Coverage
	return printSummary(e, root, sum)
}

// changedScope returns a scope function for functions changed against base.
func changedScope(e *env, root, base string) func(model.Graph) ([]model.ID, error) {
	return func(g model.Graph) ([]model.ID, error) {
		ch, err := gitx.ReadChanges(e.ctx, root, base)
		if err != nil {
			return nil, fmt.Errorf("--changed: %w", err)
		}
		return check.Changed(g, ch), nil
	}
}

func summarize(res analysis.Result, path string, took time.Duration) analyzeSummary {
	s := res.Snapshot
	sum := analyzeSummary{Module: s.Module, Snapshot: path, Seconds: took.Round(time.Millisecond).Seconds(),
		Failed: res.Failed, Log: res.LogPath, Warnings: res.Warnings}
	for _, n := range s.Nodes {
		switch n.Kind {
		case model.KindPackage:
			sum.Packages++
			for _, msg := range n.Errors {
				sum.Errors = append(sum.Errors, n.Name+": "+msg)
			}
		case model.KindFunc:
			sum.Functions++
		case model.KindType:
			sum.Types++
		}
	}
	return sum
}

func printSummary(e *env, root string, s analyzeSummary) error {
	if e.g.json {
		return e.printJSON(s)
	}
	for _, msg := range s.Errors {
		fmt.Fprintln(e.stderr, "warning: package does not type-check, results may be partial:", msg)
	}
	for _, w := range s.Warnings {
		fmt.Fprintln(e.stderr, "warning:", w)
	}
	if len(s.Failed) > 0 {
		fmt.Fprintf(e.stderr, "warning: tests failed in %d packages, their coverage is not measured; output in %s\n", len(s.Failed), s.Log)
	}
	rel, err := filepath.Rel(root, s.Snapshot)
	if err != nil {
		rel = s.Snapshot
	}
	if !e.g.quiet {
		fmt.Fprintf(e.stdout, "Scanned %s: %d packages, %d functions, %d types in %.1fs. Wrote %s.\n",
			s.Module, s.Packages, s.Functions, s.Types, s.Seconds, rel)
	}
	return nil
}

func boolFlag(fs *flag.FlagSet, name string) bool {
	return fs.Lookup(name).Value.(flag.Getter).Get().(bool)
}
