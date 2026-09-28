package cli

import (
	"flag"
	"fmt"
	"path"
	"strings"

	"github.com/morethancoder/qtldr/internal/analysis"
	"github.com/morethancoder/qtldr/internal/check"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/mutate"
)

func mutateFlags(fs *flag.FlagSet) {
	fs.String("func", "", "mutate the package of this function")
	fs.String("pkg", "", "mutate this package (import path, directory, or unique suffix)")
	fs.Bool("changed", false, "mutate packages with functions changed since [project].base_ref")
	fs.Bool("force", false, "re-test even when results are current")
}

func runMutate(e *env, fs *flag.FlagSet, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%w: mutate takes flags only", errUsage)
	}
	root, cfg, err := e.setup()
	if err != nil {
		return err
	}
	opt := analysis.Options{Root: root, Config: cfg, Mutate: true, MutateForce: boolFlag(fs, "force"), Progress: e.progressLine}
	opt.Scope = mutateScope(e, root, cfg.Project.BaseRef, fs)
	res, err := analysis.Run(e.ctx, opt)
	if res.Mutation != nil {
		printMutation(e, *res.Mutation)
	}
	if err != nil {
		return err
	}
	return mutationExit(*res.Mutation)
}

// mutateScope turns --func, --pkg or --changed into a scope; nil = all.
func mutateScope(e *env, root, base string, fs *flag.FlagSet) func(model.Graph) ([]model.ID, error) {
	fn, pkg := fs.Lookup("func").Value.String(), fs.Lookup("pkg").Value.String()
	switch {
	case fn != "":
		return func(g model.Graph) ([]model.ID, error) { return resolveOne(g, fn) }
	case pkg != "":
		return func(g model.Graph) ([]model.ID, error) { return resolvePackage(g, pkg) }
	case boolFlag(fs, "changed"):
		return changedScope(e, root, base)
	}
	return nil
}

func resolveOne(g model.Graph, id string) ([]model.ID, error) {
	full, err := model.Resolve(g.IDs(), id)
	return []model.ID{full}, err
}

// resolvePackage accepts an import path, a directory ("internal/pricing",
// "./internal/pricing") or a unique suffix.
func resolvePackage(g model.Graph, p string) ([]model.ID, error) {
	dir := path.Clean(strings.TrimPrefix(p, "./"))
	for _, n := range g.Nodes {
		if n.Kind == model.KindPackage && (n.Dir == dir || string(n.ID) == p) {
			return []model.ID{n.ID}, nil
		}
	}
	return resolveOne(g, p)
}

func printMutation(e *env, rep mutate.Report) {
	if e.g.json {
		_ = e.printJSON(rep)
		return
	}
	fmt.Fprintf(e.stdout, "Mutation testing with %s %s\n", rep.Engine, rep.Version)
	for _, p := range rep.Packages {
		fmt.Fprintf(e.stdout, "  %-28s %s\n", check.Short(p.Pkg), packageLine(p))
	}
}

func packageLine(p mutate.PackageReport) string {
	switch p.Status {
	case "ran":
		score := "—"
		if p.Killed+p.Lived > 0 {
			score = fmt.Sprintf("%.0f%%", 100*float64(p.Killed)/float64(p.Killed+p.Lived))
		}
		return fmt.Sprintf("%s score · %d killed, %d survived, %d not covered · %.1fs", score, p.Killed, p.Lived, p.NotCov, p.Seconds)
	case "failed", "skipped":
		return p.Status + ": " + p.Message
	}
	return p.Status
}

// mutationExit is exit 2 when a package could not be tested.
func mutationExit(rep mutate.Report) error {
	for _, p := range rep.Packages {
		if p.Status == "failed" {
			return exitCode(ExitError)
		}
	}
	return nil
}
