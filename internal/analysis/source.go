package analysis

import (
	"context"
	"fmt"

	"github.com/morethancoder/qtldr/internal/lang/golang"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/notes"
	"github.com/morethancoder/qtldr/internal/source"
)

// Source reads a function's source (from its doc comment to its closing
// brace) and annotates it with coverage, survivors and notes. The web view
// and MCP get_source both use it.
func Source(root string, snap model.Snapshot, n model.Node) (source.Source, error) {
	if n.Kind != model.KindFunc {
		return source.Source{}, fmt.Errorf("%s is a %s; source is shown for functions", n.ID, n.Kind)
	}
	from := n.Line
	if n.DocLine > 0 {
		from = n.DocLine
	}
	text, err := golang.New().Source(context.Background(), root, n.File, from, n.EndLine)
	if err != nil {
		return source.Source{}, fmt.Errorf("%v (the file changed since the last scan; re-run qtldr analyze)", err)
	}
	all, err := notes.Load(root)
	if err != nil {
		return source.Source{}, err
	}
	return source.Annotate(n, snap.Metrics[n.ID], notes.PlaceAll(root, all, n), text, from), nil
}
