package mutate

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/morethancoder/qtldr/internal/config"
)

// Gremlins runs `gremlins unleash` (verified flags and JSON: docs/decisions.md #5).
type Gremlins struct {
	// Bin is the executable; default "gremlins".
	Bin string
}

// Name returns "gremlins".
func (Gremlins) Name() string { return "gremlins" }

func (g Gremlins) bin() string {
	if g.Bin != "" {
		return g.Bin
	}
	return "gremlins"
}

// Version is the module version of the installed binary (`go version -m`),
// which `gremlins --version` does not show for `go install` builds.
func (g Gremlins) Version(ctx context.Context) string {
	p, err := exec.LookPath(g.bin())
	if err != nil {
		return "unknown"
	}
	out, err := exec.CommandContext(ctx, "go", "version", "-m", p).Output()
	if err != nil {
		return "unknown"
	}
	return ModuleVersion(string(out), "github.com/go-gremlins/gremlins")
}

// ModuleVersion finds "mod <path> <version>" in `go version -m` output.
func ModuleVersion(out, module string) string {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && f[0] == "mod" && f[1] == module {
			return f[2]
		}
	}
	return "unknown"
}

// Run executes `gremlins unleash ./<dir> -o <json> --timeout-coefficient N`
// with the Go test cache disabled (a cached coverage run makes Gremlins'
// timeouts near zero, so every mutant "times out").
func (g Gremlins) Run(ctx context.Context, root string, t Target, cfg config.Mutation) ([]FileMutant, []byte, error) {
	report, err := os.CreateTemp("", "qtldr-gremlins-*.json")
	if err != nil {
		return nil, nil, err
	}
	report.Close()
	defer os.Remove(report.Name())
	args := g.Args(t, report.Name(), cfg.Args)
	out, err := g.exec(ctx, root, args)
	if err != nil {
		return nil, out, err
	}
	b, err := os.ReadFile(report.Name())
	if err != nil {
		return nil, out, fmt.Errorf("gremlins wrote no report: %w", err)
	}
	mutants, err := ParseReport(b, t.Dir)
	return mutants, out, err
}

// Args is the unleash command line for one package.
func (Gremlins) Args(t Target, report string, extra []string) []string {
	return append([]string{"unleash", "./" + t.Dir, "-o", report,
		"--timeout-coefficient", fmt.Sprint(TimeoutCoefficient(t.Baseline))}, extra...)
}

func (g Gremlins) exec(ctx context.Context, root string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, g.bin(), args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS="+strings.TrimSpace(os.Getenv("GOFLAGS")+" -count=1"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("`%s %s` failed: %v", g.bin(), strings.Join(args, " "), err)
	}
	return out, nil
}

// TimeoutCoefficient makes the per-mutant timeout (Gremlins: coverage time ×
// coefficient) at least the baseline plus 15 s, and never below Gremlins'
// default of 3. A small package's tests run in about a second, but each
// mutant also recompiles the test binary.
func TimeoutCoefficient(baseline time.Duration) int {
	if baseline <= 0 {
		baseline = time.Second
	}
	return max(3, int(math.Ceil(float64(baseline+15*time.Second)/float64(baseline))))
}

// report is the part of Gremlins' JSON qtldr reads.
type report struct {
	Files []struct {
		FileName  string `json:"file_name"`
		Mutations []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
		} `json:"mutations"`
	} `json:"files"`
}

// ParseReport reads Gremlins JSON. File names are relative to the directory
// Gremlins ran in (dir, relative to the module root); they are made
// module-relative.
func ParseReport(b []byte, dir string) ([]FileMutant, error) {
	var r report
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("gremlins report is not valid JSON: %w", err)
	}
	var out []FileMutant
	for _, f := range r.Files {
		file := path.Clean(path.Join(filepath.ToSlash(dir), f.FileName))
		for _, m := range f.Mutations {
			out = append(out, FileMutant{File: file, Line: m.Line, Col: m.Column, Type: m.Type, Status: m.Status})
		}
	}
	return out, nil
}

// Clock lets tests fix time; the runner uses it for ran_at.
var Clock = func() time.Time { return time.Now().UTC().Truncate(time.Second) }
