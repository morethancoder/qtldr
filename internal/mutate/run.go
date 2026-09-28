package mutate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/store"
)

// Options configure a mutate run.
type Options struct {
	Root   string
	Graph  model.Graph
	Config config.Mutation
	// Scope limits the run to these packages (nil = all).
	Scope []model.ID
	// Force re-runs packages even when their results are current.
	Force    bool
	Engine   Mutator
	Progress func(msg string)
}

// PackageReport is what happened to one package.
type PackageReport struct {
	Pkg     model.ID `json:"package"`
	Status  string   `json:"status"` // ran | failed | up to date | skipped
	Killed  int      `json:"killed,omitempty"`
	Lived   int      `json:"survived,omitempty"`
	NotCov  int      `json:"not_covered,omitempty"`
	Message string   `json:"message,omitempty"`
	Seconds float64  `json:"seconds,omitempty"`
}

// Report is a finished run.
type Report struct {
	Engine   string          `json:"engine"`
	Version  string          `json:"engine_version"`
	Packages []PackageReport `json:"packages,omitempty"`
}

// PreflightFailed is the reason recorded when a package's tests fail before
// mutation (PLAN.md §7.3).
const PreflightFailed = "tests fail before mutation; results would be meaningless"

// Run tests the planned packages, one at a time, and saves their results.
func Run(ctx context.Context, opt Options) (Report, error) {
	if opt.Engine == nil {
		opt.Engine = Gremlins{}
	}
	if opt.Config.Timeout.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opt.Config.Timeout.Duration)
		defer cancel()
	}
	caches, err := loadCaches(opt.Root, opt.Graph)
	if err != nil {
		return Report{}, err
	}
	plan := MakePlan(opt.Graph, caches, opt.Scope, opt.Force, opt.Config.MaxFunctions)
	rep := Report{Engine: opt.Engine.Name(), Version: opt.Engine.Version(ctx)}
	if err := runPlan(ctx, opt, plan, caches, &rep); err != nil {
		return rep, err
	}
	rep.Packages = append(rep.Packages, notRun(plan, opt.Config.MaxFunctions)...)
	return rep, nil
}

// runPlan tests each planned package and saves its cache.
func runPlan(ctx context.Context, opt Options, plan Plan, caches map[model.ID]Cache, rep *Report) error {
	for i, it := range plan.Run {
		progress(opt, fmt.Sprintf("Running mutation… %d/%d packages (%s)", i+1, len(plan.Run), it.Dir))
		c := caches[it.Pkg]
		pr := runPackage(ctx, opt, *rep, it, &c)
		if err := Save(opt.Root, c); err != nil {
			return err
		}
		rep.Packages = append(rep.Packages, pr)
		if ctx.Err() != nil {
			return fmt.Errorf("mutation stopped after [mutation].timeout (%v)", opt.Config.Timeout.Duration)
		}
	}
	return nil
}

// notRun reports the up-to-date and skipped packages.
func notRun(plan Plan, max int) []PackageReport {
	var out []PackageReport
	for _, id := range plan.UpToDate {
		out = append(out, PackageReport{Pkg: id, Status: "up to date"})
	}
	for _, it := range plan.Skipped {
		out = append(out, PackageReport{Pkg: it.Pkg, Status: "skipped",
			Message: fmt.Sprintf("over [mutation].max_functions (%d) for this run; run qtldr mutate again", max)})
	}
	return out
}

func progress(opt Options, msg string) {
	if opt.Progress != nil {
		opt.Progress(msg)
	}
}

func loadCaches(root string, g model.Graph) (map[model.ID]Cache, error) {
	out := map[model.ID]Cache{}
	for _, n := range g.Nodes {
		if n.Kind != model.KindPackage {
			continue
		}
		c, err := Load(root, n.ID)
		if err != nil {
			return nil, err
		}
		out[n.ID] = c
	}
	return out, nil
}

// runPackage runs the pre-flight, then the engine, and records the outcome in c.
func runPackage(ctx context.Context, opt Options, rep Report, it Item, c *Cache) PackageReport {
	start := time.Now()
	pr := PackageReport{Pkg: it.Pkg}
	baseline, out, err := Preflight(ctx, opt.Root, it.Pkg)
	if err != nil {
		return fail(opt, c, pr, PreflightFailed, out)
	}
	mutants, out, err := opt.Engine.Run(ctx, opt.Root, Target{Dir: it.Dir, Baseline: baseline}, opt.Config)
	if err != nil {
		return fail(opt, c, pr, err.Error(), out)
	}
	results := Attribute(opt.Graph, it.Pkg, mutants, sources(opt.Root, opt.Graph, it.Pkg))
	c.Record(opt.Graph, results, rep.Engine, rep.Version, Clock())
	for _, mu := range results {
		pr.Killed, pr.Lived, pr.NotCov = pr.Killed+mu.Killed, pr.Lived+mu.Survived, pr.NotCov+mu.NotCovered
	}
	pr.Status, pr.Seconds = "ran", time.Since(start).Round(100*time.Millisecond).Seconds()
	return pr
}

// fail records a failed package with its output saved under .qtldr/logs/.
func fail(opt Options, c *Cache, pr PackageReport, reason string, output []byte) PackageReport {
	name := "mutation-" + strings.ReplaceAll(string(filepath.Base(string(pr.Pkg))), "/", "_")
	if log, err := store.WriteLog(opt.Root, name, Clock(), output); err == nil {
		reason += "; output in " + log
	}
	c.Fail(reason, Clock())
	pr.Status, pr.Message = "failed", reason
	return pr
}

// Preflight runs the package's tests once, uncached, and returns how long
// they took. A failure means mutation results would be meaningless.
func Preflight(ctx context.Context, root string, pkg model.ID) (time.Duration, []byte, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", string(pkg))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return time.Since(start), out, err
}

// sources reads the files of the package's functions.
func sources(root string, g model.Graph, pkg model.ID) map[string][]byte {
	out := map[string][]byte{}
	for _, n := range g.Nodes {
		if n.Kind != model.KindFunc || n.Parent != pkg || out[n.File] != nil {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(n.File))); err == nil {
			out[n.File] = b
		}
	}
	return out
}
