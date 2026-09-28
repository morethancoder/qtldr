package cli

import (
	"flag"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/store"
)

func runShow(e *env, _ *flag.FlagSet, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("%w: show takes exactly one ID", errUsage)
	}
	snap, err := e.readSnapshot()
	if err != nil {
		return err
	}
	id, err := model.Resolve(snap.IDs(), args[0])
	if err != nil {
		return err
	}
	d, _ := snap.Detail(id)
	if e.g.json {
		return e.printJSON(d)
	}
	printDetail(e.stdout, snap.Module, d)
	return nil
}

func (e *env) readSnapshot() (model.Snapshot, error) {
	root, err := e.moduleRoot()
	if err != nil {
		return model.Snapshot{}, err
	}
	return store.ReadSnapshot(root)
}

func printDetail(w io.Writer, module string, d model.Detail) {
	n := d.Node
	fmt.Fprintf(w, "%s  (%s)\n%s\n", n.Name, kindLabel(n), n.ID)
	if n.File != "" {
		fmt.Fprintf(w, "%s\n", location(n))
	}
	if n.Signature != "" {
		fmt.Fprintf(w, "%s\n", n.Signature)
	}
	for _, msg := range n.Errors {
		fmt.Fprintf(w, "error: %s\n", msg)
	}
	printMetrics(w, n, d.Metrics)
	printFields(w, n.Fields)
	printIDs(w, "Callers", module, d.Callers)
	printIDs(w, "Callees", module, d.Callees)
	printIDs(w, "Imports", module, d.Imports)
	printIDs(w, "Imported by", module, d.ImportedBy)
	printIDs(w, "Contains", module, d.Children)
}

func kindLabel(n model.Node) string {
	switch {
	case n.Kind == model.KindFunc && n.Exported != nil && *n.Exported:
		return "function · exported"
	case n.Kind == model.KindFunc:
		return "function · unexported"
	case n.Kind == model.KindType:
		return "type · " + n.TypeKind
	case n.Kind == model.KindExternal:
		return "external module"
	}
	return string(n.Kind)
}

func location(n model.Node) string {
	if n.EndLine > n.Line {
		return fmt.Sprintf("%s:%d–%d", n.File, n.Line, n.EndLine)
	}
	return fmt.Sprintf("%s:%d", n.File, n.Line)
}

// printMetrics prints function metrics; values not measured yet say so.
func printMetrics(w io.Writer, n model.Node, m *model.Metrics) {
	if n.Kind != model.KindFunc {
		return
	}
	if m == nil {
		m = &model.Metrics{}
	}
	fmt.Fprintln(w)
	rows := []struct {
		label string
		value *int
	}{{"Cyclomatic (CC)", m.CC}, {"Cognitive", m.Cognitive}, {"Lines", m.LOC}}
	for _, r := range rows {
		fmt.Fprintf(w, "  %-16s %s\n", r.label, intOrNotMeasured(r.value))
	}
	for _, label := range []string{"CRAP", "Coverage", "Mutation score"} {
		fmt.Fprintf(w, "  %-16s %s\n", label, "not measured")
	}
}

func intOrNotMeasured(v *int) string {
	if v == nil {
		return "not measured"
	}
	return fmt.Sprint(*v)
}

func printFields(w io.Writer, fields []model.Field) {
	if len(fields) == 0 {
		return
	}
	fmt.Fprintln(w, "\nFields")
	for _, f := range fields {
		fmt.Fprintf(w, "  %-16s %s\n", f.Name, f.Type)
	}
}

func printIDs(w io.Writer, title, module string, ids []model.ID) {
	if len(ids) == 0 {
		return
	}
	short := make([]string, len(ids))
	for i, id := range ids {
		short[i] = shortID(module, id)
	}
	fmt.Fprintf(w, "\n%s (%d): %s\n", title, len(ids), strings.Join(short, ", "))
}

// shortID drops the module path and leading directories:
// "github.com/acme/ledger/internal/pricing.applyTiered" → "pricing.applyTiered".
func shortID(module string, id model.ID) string {
	s := string(id)
	if rest, ok := strings.CutPrefix(s, module+"/"); ok {
		return path.Base(rest)
	}
	return s
}
