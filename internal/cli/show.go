package cli

import (
	"flag"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/morethancoder/qtldr/internal/check"
	"github.com/morethancoder/qtldr/internal/coverage"
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
	printDetail(e.stdout, d)
	return nil
}

func (e *env) readSnapshot() (model.Snapshot, error) {
	root, err := e.moduleRoot()
	if err != nil {
		return model.Snapshot{}, err
	}
	return store.ReadSnapshot(root)
}

func printDetail(w io.Writer, d model.Detail) {
	n := d.Node
	fmt.Fprintf(w, "%s  (%s%s)\n%s\n", n.Name, kindLabel(n), purityLabel(n), n.ID)
	if n.File != "" {
		fmt.Fprintf(w, "%s\n", location(n))
	}
	if n.Signature != "" {
		fmt.Fprintf(w, "%s\n", n.Signature)
	}
	for _, msg := range n.Errors {
		fmt.Fprintf(w, "error: %s\n", msg)
	}
	if len(n.Effects) > 0 {
		fmt.Fprintf(w, "effectful because: %s\n", strings.Join(n.Effects, "; "))
	}
	m := model.Metrics{}
	if d.Metrics != nil {
		m = *d.Metrics
	}
	printRows(w, metricRows(n, m))
	printCoverageLines(w, n, m.Coverage)
	printSurvivors(w, n, m.Mutation)
	printFields(w, n.Fields)
	printIDs(w, "Callers", d.Callers)
	printIDs(w, "Callees", d.Callees)
	printIDs(w, "Imports", d.Imports)
	printIDs(w, "Imported by", d.ImportedBy)
	printIDs(w, "Contains", d.Children)
	printIDs(w, "Methods", d.Methods)
	printIDs(w, "Implements", d.Implements)
	printIDs(w, "Implemented by", d.ImplementedBy)
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

func purityLabel(n model.Node) string {
	switch {
	case n.Pure == nil:
		return ""
	case *n.Pure && n.Kind == model.KindPackage:
		return " · λ pure core"
	case *n.Pure:
		return " · λ pure"
	case n.Kind == model.KindPackage:
		return " · effectful shell"
	}
	return " · effectful"
}

func location(n model.Node) string {
	if n.EndLine > n.Line {
		return fmt.Sprintf("%s:%d–%d", n.File, n.Line, n.EndLine)
	}
	return fmt.Sprintf("%s:%d", n.File, n.Line)
}

type row struct{ label, value string }

func metricRows(n model.Node, m model.Metrics) []row {
	switch n.Kind {
	case model.KindFunc:
		return []row{
			{"CRAP", withGrade(floatOr(m.CRAP, "%.1f"), m.Grades, func(g model.Grades) int { return g.CRAP })},
			{"Cyclomatic (CC)", intOrNotMeasured(m.CC)},
			{"Cognitive", intOrNotMeasured(m.Cognitive)},
			{"Coverage", coverageText(m.Coverage, m.Grades)},
			{"Mutation score", mutationText(m.Mutation, m.Grades)},
			{"Lines", intOrNotMeasured(m.LOC)},
			{churnLabel(m.ChurnScope), intOrNotMeasured(m.Churn)},
			{"Grade", gradeText(m.Grades)},
		}
	case model.KindPackage, model.KindModule:
		rows := []row{
			{"CRAP max", floatOr(m.CrapMax, "%.1f")},
			{"CRAP average", floatOr(m.CrapAvg, "%.1f")},
			{"Coverage", coverageText(m.Coverage, nil)},
			{"Mutation score", mutationText(m.Mutation, nil)},
			{"Churn", intOrNotMeasured(m.Churn)},
			{"Grade", gradeText(m.Grades)},
		}
		if m.Worst != "" {
			rows = append(rows, row{"Riskiest", check.Short(m.Worst)})
		}
		return append(rows, errorRows(m)...)
	}
	return nil
}

func errorRows(m model.Metrics) []row {
	var rows []row
	if m.CoverageError != "" {
		rows = append(rows, row{"Coverage error", m.CoverageError})
	}
	if m.MutationError != "" {
		rows = append(rows, row{"Mutation error", m.MutationError})
	}
	return rows
}

func churnLabel(scope string) string {
	if scope == "function" {
		return "Churn (approx.)"
	}
	return "Churn (file)"
}

func printRows(w io.Writer, rows []row) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintln(w)
	for _, r := range rows {
		fmt.Fprintf(w, "  %-16s %s\n", r.label, r.value)
	}
}

func withGrade(v string, g *model.Grades, pick func(model.Grades) int) string {
	if g == nil || v == "not measured" {
		return v
	}
	return fmt.Sprintf("%s  (grade %d)", v, pick(*g))
}

func coverageText(c *model.Coverage, g *model.Grades) string {
	switch {
	case c == nil:
		return "not measured"
	case c.Percent == nil:
		return "no statements"
	}
	s := fmt.Sprintf("%g%% (%d of %d statements)", *c.Percent, c.Covered, c.Stmts)
	if g != nil {
		s += fmt.Sprintf("  (grade %d)", g.Coverage)
	}
	if c.Stale {
		s += "  stale"
	}
	return s
}

func mutationText(mu *model.Mutation, g *model.Grades) string {
	switch {
	case mu == nil:
		return "not measured"
	case mu.Sites() == 0:
		return "no mutation sites"
	case mu.Score == nil:
		return fmt.Sprintf("— (%d not covered)", mu.NotCovered)
	}
	s := fmt.Sprintf("%g%% (%d killed, %d survived, %d not covered)", *mu.Score, mu.Killed, mu.Survived, mu.NotCovered)
	if g != nil && g.Mutation != nil {
		s += fmt.Sprintf("  (grade %d)", *g.Mutation)
	}
	if mu.Stale {
		s += "  stale"
	}
	return s
}

func gradeText(g *model.Grades) string {
	if g == nil {
		return "—"
	}
	mut := "n/a"
	if g.Mutation != nil {
		mut = fmt.Sprint(*g.Mutation)
	}
	return fmt.Sprintf("%d of 10 (CRAP %d, mutation %s, coverage %d)", g.Combined, g.CRAP, mut, g.Coverage)
}

func floatOr(v *float64, format string) string {
	if v == nil {
		return "not measured"
	}
	return fmt.Sprintf(format, *v)
}

func intOrNotMeasured(v *int) string {
	if v == nil {
		return "not measured"
	}
	return fmt.Sprint(*v)
}

func printCoverageLines(w io.Writer, n model.Node, c *model.Coverage) {
	if c == nil || c.Lines == nil || n.Kind != model.KindFunc {
		return
	}
	file := path.Base(n.File)
	if len(c.Lines.Uncovered) > 0 {
		fmt.Fprintf(w, "\nNot covered: %s:%s\n", file, strings.Join(coverage.Ranges(c.Lines.Uncovered), ", "))
	}
	if len(c.Lines.Partial) > 0 {
		fmt.Fprintf(w, "Partly covered: %s:%s\n", file, strings.Join(coverage.Ranges(c.Lines.Partial), ", "))
	}
}

func printSurvivors(w io.Writer, n model.Node, mu *model.Mutation) {
	if mu == nil || mu.Survived == 0 {
		return
	}
	fmt.Fprintf(w, "\nSurviving mutants (%d)\n", mu.Survived)
	for _, m := range mu.Mutants {
		if m.Status == "LIVED" {
			fmt.Fprintf(w, "  %s:%d  %s\n", path.Base(n.File), m.Line, m.Description)
		}
	}
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

func printIDs(w io.Writer, title string, ids []model.ID) {
	if len(ids) == 0 {
		return
	}
	short := make([]string, len(ids))
	for i, id := range ids {
		short[i] = check.Short(id)
	}
	fmt.Fprintf(w, "\n%s (%d): %s\n", title, len(ids), strings.Join(short, ", "))
}
