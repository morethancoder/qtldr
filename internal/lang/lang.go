// Package lang defines the language provider interface. The Go provider lives
// in lang/golang; coverage methods join the interface in M1 and the external
// provider in M5.
package lang

import (
	"context"

	"github.com/morethancoder/qtldr/internal/config"
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
	// Source returns lines from..to (1-based, inclusive) of file, relative to root.
	Source(ctx context.Context, root, file string, from, to int) (string, error)
}
