// Package lang defines the language provider interface. The Go provider lives
// in lang/golang; the external provider arrives in M5.
package lang

import (
	"context"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/coverage"
	"github.com/morethancoder/qtldr/internal/model"
)

// Provider reads one language's code into a graph.
type Provider interface {
	// Name is the provider's short name, e.g. "go".
	Name() string
	// Detect reports whether root holds code this provider reads.
	Detect(root string) bool
	// Scan returns nodes, edges, cc, cognitive, loc and body hashes.
	Scan(ctx context.Context, root string, cfg config.Project) (model.Graph, error)
	// CoverageRun runs the tests of pkgs with coverage, writing the profile
	// to profilePath. Failing packages are listed in the result.
	CoverageRun(ctx context.Context, root string, pkgs []string, cfg config.Coverage, profilePath string) (coverage.RunResult, error)
	// MapCoverage attributes a profile to the functions of g (pure).
	MapCoverage(g model.Graph, p coverage.Profile) map[model.ID]model.Coverage
	// Source returns lines from..to (1-based, inclusive) of file, relative to root.
	Source(ctx context.Context, root, file string, from, to int) (string, error)
}
