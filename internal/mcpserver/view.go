// Package mcpserver is `qtldr mcp`: the same snapshot the CLI and web UI use,
// served to agents as MCP tools over stdio (PLAN.md §9.1).
package mcpserver

import (
	"fmt"
	"path"
	"strings"

	"github.com/morethancoder/qtldr/internal/check"
	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/coverage"
	"github.com/morethancoder/qtldr/internal/glossary"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/notes"
	"github.com/morethancoder/qtldr/internal/source"
)

// Term is the glossary text attached to results: what a metric means and
// which direction is good (with qtldr's targets).
type Term struct {
	Short string `json:"short"`
	Good  string `json:"good"`
}

// Scores are a node's metrics next to their targets. Nil means not measured.
type Scores struct {
	CRAP            *float64      `json:"crap"`
	CRAPTarget      float64       `json:"crap_target"`
	CrapMax         *float64      `json:"crap_max,omitempty"`
	CrapAvg         *float64      `json:"crap_avg,omitempty"`
	CC              *int          `json:"cc,omitempty"`
	Cognitive       *int          `json:"cognitive,omitempty"`
	CognitiveTarget int           `json:"cognitive_target"`
	Coverage        *float64      `json:"coverage_percent"`
	CoverageTarget  float64       `json:"coverage_target"`
	CoverageStale   bool          `json:"coverage_stale,omitempty"`
	Mutation        *float64      `json:"mutation_score"`
	MutationTarget  float64       `json:"mutation_target"`
	MutationStale   bool          `json:"mutation_stale,omitempty"`
	Killed          int           `json:"killed"`
	Survived        int           `json:"survived"`
	NotCovered      int           `json:"not_covered_mutants"`
	Churn           *int          `json:"churn,omitempty"`
	Grades          *model.Grades `json:"grades,omitempty"`
	CoverageError   string        `json:"coverage_error,omitempty"`
	MutationError   string        `json:"mutation_error,omitempty"`
}

// Survivor is one mutant no test caught.
type Survivor struct {
	At     string `json:"at"` // file:line
	Change string `json:"change"`
	Type   string `json:"type"`
	Hint   string `json:"hint"`
}

// NoteView is a note in the current code.
type NoteView struct {
	ID       string `json:"id"`
	Line     int    `json:"line,omitempty"`
	Author   string `json:"author"`
	Text     string `json:"text"`
	Outdated bool   `json:"outdated,omitempty"`
}

// NodeView is everything an agent needs about one node.
type NodeView struct {
	ID        model.ID        `json:"id"`
	Kind      model.Kind      `json:"kind"`
	Name      string          `json:"name"`
	File      string          `json:"file,omitempty"`
	Lines     string          `json:"lines,omitempty"`
	Signature string          `json:"signature,omitempty"`
	Pure      *bool           `json:"pure,omitempty"`
	Effects   []string        `json:"effects,omitempty"`
	Scores    Scores          `json:"scores"`
	Worst     string          `json:"riskiest_function,omitempty"`
	Callers   []string        `json:"callers,omitempty"`
	Callees   []string        `json:"callees,omitempty"`
	Children  []string        `json:"contains,omitempty"`
	Uncovered []string        `json:"uncovered_lines,omitempty"`
	Partial   []string        `json:"partly_covered_lines,omitempty"`
	Survivors []Survivor      `json:"surviving_mutants,omitempty"`
	Notes     []NoteView      `json:"notes,omitempty"`
	Verify    string          `json:"verify"`
	Glossary  map[string]Term `json:"glossary,omitempty"`
}

// termsUsed are attached to every node view.
var termsUsed = []string{"crap", "cc", "cognitive", "coverage", "mutation", "survived", "not_covered", "stale", "not_measured", "purity"}

func terms(g glossary.Glossary, keys ...string) map[string]Term {
	out := map[string]Term{}
	for k, t := range g.Subset(keys...) {
		out[k] = Term{Short: t.Short, Good: t.Good}
	}
	return out
}

// nodeView builds the view of d (pure).
func nodeView(d model.Detail, th config.Thresholds, g glossary.Glossary, placed []notes.Placed) NodeView {
	n := d.Node
	m := model.Metrics{}
	if d.Metrics != nil {
		m = *d.Metrics
	}
	v := NodeView{
		ID: n.ID, Kind: n.Kind, Name: n.Name, File: n.File, Signature: n.Signature, Pure: n.Pure, Effects: n.Effects,
		Scores: scores(m, th), Callers: shorts(d.Callers), Callees: shorts(d.Callees), Children: shorts(d.Children),
		Verify: "qtldr check " + checkTarget(n), Glossary: terms(g, termsUsed...),
	}
	if n.Line > 0 {
		v.Lines = fmt.Sprintf("%d-%d", n.Line, max(n.Line, n.EndLine))
	}
	if m.Worst != "" {
		v.Worst = check.Short(m.Worst)
	}
	v.Uncovered, v.Partial = lineRanges(n.File, m.Coverage)
	v.Survivors = survivors(n.File, m.Mutation)
	for _, p := range placed {
		v.Notes = append(v.Notes, NoteView{ID: p.ID, Line: p.Line, Author: p.Author, Text: p.Text, Outdated: p.Outdated})
	}
	return v
}

func checkTarget(n model.Node) string {
	if n.Kind == model.KindPackage {
		return n.Name
	}
	return check.Short(n.ID)
}

func scores(m model.Metrics, th config.Thresholds) Scores {
	s := Scores{
		CRAP: m.CRAP, CRAPTarget: th.CrapMax, CrapMax: m.CrapMax, CrapAvg: m.CrapAvg, CC: m.CC, Cognitive: m.Cognitive,
		CognitiveTarget: th.CognitiveMax, CoverageTarget: th.CoverageMin, MutationTarget: th.MutationMin,
		Churn: m.Churn, Grades: m.Grades, CoverageError: m.CoverageError, MutationError: m.MutationError,
	}
	if c := m.Coverage; c != nil {
		s.Coverage, s.CoverageStale = c.Percent, c.Stale
	}
	if mu := m.Mutation; mu != nil {
		s.Mutation, s.MutationStale = mu.Score, mu.Stale
		s.Killed, s.Survived, s.NotCovered = mu.Killed, mu.Survived, mu.NotCovered
	}
	return s
}

func lineRanges(file string, c *model.Coverage) (uncovered, partial []string) {
	if c == nil || c.Lines == nil {
		return nil, nil
	}
	base := path.Base(file)
	for _, r := range coverage.Ranges(c.Lines.Uncovered) {
		uncovered = append(uncovered, base+":"+r)
	}
	for _, r := range coverage.Ranges(c.Lines.Partial) {
		partial = append(partial, base+":"+r)
	}
	return uncovered, partial
}

func survivors(file string, mu *model.Mutation) []Survivor {
	if mu == nil {
		return nil
	}
	var out []Survivor
	for _, m := range mu.Mutants {
		if m.Status == "LIVED" {
			out = append(out, Survivor{At: fmt.Sprintf("%s:%d", path.Base(file), m.Line), Change: m.Description, Type: m.Type, Hint: source.Hint(m.Type)})
		}
	}
	return out
}

func shorts(ids []model.ID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = check.Short(id)
	}
	return out
}

// sourceText renders an annotated source as numbered lines with markers,
// easy for a model to read: "  29 C | if qty >= t.Floor … ⟵ SURVIVED: >= → >".
func sourceText(s source.Source) string {
	var b strings.Builder
	marks := map[string]string{"covered": "C", "uncovered": "U", "partial": "P"}
	for _, l := range s.Lines {
		mark := marks[l.State]
		if mark == "" {
			mark = " "
		}
		fmt.Fprintf(&b, "%4d %s | %s\n", l.N, mark, l.Text)
		for _, a := range l.Annotations {
			fmt.Fprintf(&b, "       ^ %s: %s %s %s\n", strings.ToUpper(strings.ReplaceAll(a.Kind, "_", " ")), a.Title, a.Detail, a.Why)
		}
	}
	return b.String()
}
