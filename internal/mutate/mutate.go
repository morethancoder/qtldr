// Package mutate runs mutation testing per package behind an engine-neutral
// Mutator (Gremlins today), attributes mutants to functions, describes each
// change ("> → >="), and keeps committed per-package results in
// .qtldr/mutation/ so unchanged functions are never re-tested.
package mutate

import (
	"context"
	"time"

	"github.com/morethancoder/qtldr/internal/config"
)

// Statuses reported by engines (Gremlins' names).
const (
	Killed     = "KILLED"
	Lived      = "LIVED"
	NotCovered = "NOT COVERED"
	TimedOut   = "TIMED OUT"
	NotViable  = "NOT VIABLE"
	Skipped    = "SKIPPED"
	Runnable   = "RUNNABLE"
)

// FileMutant is one mutant as an engine reports it; File is relative to the
// module root.
type FileMutant struct {
	File   string
	Line   int
	Col    int
	Type   string
	Status string
}

// Target is one package to mutate.
type Target struct {
	// Dir is the package directory relative to the module root ("." for the root).
	Dir string
	// Baseline is how long the package's tests took in the pre-flight run;
	// engines use it to size per-mutant timeouts.
	Baseline time.Duration
}

// Mutator is a mutation engine.
type Mutator interface {
	Name() string
	// Version identifies the engine build (stored with results).
	Version(ctx context.Context) string
	// Run mutates one package and returns every mutant with its status.
	Run(ctx context.Context, root string, t Target, cfg config.Mutation) ([]FileMutant, []byte, error)
}
