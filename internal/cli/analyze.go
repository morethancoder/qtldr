package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"github.com/morethancoder/qtldr/internal/gitx"
	"github.com/morethancoder/qtldr/internal/lang/golang"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/store"
)

// analyzeSummary is what analyze prints.
type analyzeSummary struct {
	Module    string   `json:"module"`
	Packages  int      `json:"packages"`
	Functions int      `json:"functions"`
	Types     int      `json:"types"`
	Errors    []string `json:"errors,omitempty"`
	Snapshot  string   `json:"snapshot"`
	Seconds   float64  `json:"seconds"`
}

func runAnalyze(e *env, _ *flag.FlagSet, args []string) error {
	root, err := e.moduleRoot()
	if err != nil {
		return err
	}
	cfg, err := e.loadConfig(root)
	if err != nil {
		return err
	}
	if len(args) > 0 {
		cfg.Project.Include = args
	}
	start := time.Now()
	e.debug("scanning %v in %s", cfg.Project.Include, root)
	g, err := golang.New().Scan(e.ctx, root, cfg.Project)
	if err != nil {
		return err
	}
	snap := newSnapshot(g, gitx.Info(e.ctx, root), start.UTC().Truncate(time.Second))
	if err := store.WriteSnapshot(root, snap); err != nil {
		return err
	}
	sum := summarize(snap, store.SnapshotPath(root), time.Since(start))
	return printSummary(e, root, sum)
}

func newSnapshot(g model.Graph, git *model.GitInfo, at time.Time) model.Snapshot {
	s := model.Snapshot{
		Schema: model.SchemaVersion, ToolVersion: Version, Generated: at, Git: git,
		Runs: model.Runs{Structure: &at}, Graph: g,
	}
	for _, n := range g.Nodes {
		if n.Kind == model.KindModule {
			s.Module = string(n.ID)
		}
	}
	return s
}

func summarize(s model.Snapshot, path string, took time.Duration) analyzeSummary {
	sum := analyzeSummary{Module: s.Module, Snapshot: path, Seconds: took.Round(time.Millisecond).Seconds()}
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
