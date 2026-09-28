// Package source annotates a function's source lines with coverage state,
// surviving mutants and notes. It is pure; the web view and MCP get_source
// both use it.
package source

import (
	"fmt"
	"strings"
	"time"

	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/notes"
)

// Annotation kinds.
const (
	Survived   = "survived"
	NotCovered = "not_covered"
	Note       = "note"
)

// Annotation is shown under the line it belongs to.
type Annotation struct {
	Kind     string    `json:"kind"`
	Title    string    `json:"title"`
	Detail   string    `json:"detail,omitempty"`
	Why      string    `json:"why,omitempty"`
	NoteID   string    `json:"note_id,omitempty"`
	Author   string    `json:"author,omitempty"`
	Created  time.Time `json:"created,omitzero"`
	Outdated bool      `json:"outdated,omitempty"`
}

// Line is one source line.
type Line struct {
	N           int          `json:"n"`
	Text        string       `json:"text"`
	State       string       `json:"state,omitempty"` // covered | uncovered | partial
	Survived    int          `json:"survived,omitempty"`
	Annotations []Annotation `json:"annotations,omitempty"`
}

// Source is an annotated function.
type Source struct {
	ID            model.ID `json:"id"`
	File          string   `json:"file"`
	From          int      `json:"from"`
	To            int      `json:"to"`
	CoverageStale bool     `json:"coverage_stale"`
	MutationStale bool     `json:"mutation_stale"`
	Lines         []Line   `json:"lines"`
}

// Annotate builds the view of text (lines from..to of n.File).
func Annotate(n model.Node, m model.Metrics, placed []notes.Placed, text string, from int) Source {
	src := Source{ID: n.ID, File: n.File, From: from}
	raw := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	byLine := map[int]*Line{}
	for i, t := range raw {
		src.Lines = append(src.Lines, Line{N: from + i, Text: t})
	}
	for i := range src.Lines {
		byLine[src.Lines[i].N] = &src.Lines[i]
	}
	src.To = from + len(raw) - 1
	if c := m.Coverage; c != nil {
		src.CoverageStale = c.Stale
		markCoverage(byLine, c.Lines)
		markUncoveredRuns(src.Lines)
	}
	notesByLine := map[int][]notes.Placed{}
	for _, p := range placed {
		notesByLine[p.Line] = append(notesByLine[p.Line], p)
	}
	if mu := m.Mutation; mu != nil {
		src.MutationStale = mu.Stale
		markSurvivors(byLine, mu.Mutants, notesByLine)
	}
	markNotes(byLine, notesByLine)
	return src
}

func markCoverage(byLine map[int]*Line, ls *model.LineStates) {
	if ls == nil {
		return
	}
	for state, lines := range map[string][]int{"covered": ls.Covered, "uncovered": ls.Uncovered, "partial": ls.Partial} {
		for _, n := range lines {
			if l, ok := byLine[n]; ok {
				l.State = state
			}
		}
	}
}

// markUncoveredRuns adds one NOT COVERED annotation at the first line of
// each run of uncovered lines (lines without a state do not break a run).
func markUncoveredRuns(lines []Line) {
	for i := 0; i < len(lines); i++ {
		if lines[i].State != "uncovered" {
			continue
		}
		end := runEnd(lines, i)
		a := Annotation{Kind: NotCovered, Title: "Never run by tests", Detail: condition(lines, i)}
		if lines[end].N > lines[i].N {
			a.Why = fmt.Sprintf("lines %d–%d", lines[i].N, lines[end].N)
		}
		lines[i].Annotations = append(lines[i].Annotations, a)
		i = end
	}
}

// runEnd is the index of the last uncovered line of the run starting at i.
func runEnd(lines []Line, i int) int {
	end := i
	for j := i + 1; j < len(lines) && lines[j].State != "covered" && lines[j].State != "partial"; j++ {
		if lines[j].State == "uncovered" {
			end = j
		}
	}
	return end
}

// condition finds the branch that guards line i: the nearest preceding
// if/case/for/select/else header, shown without its brace.
func condition(lines []Line, i int) string {
	for j := i; j >= 0 && j >= i-3; j-- {
		t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[j].Text), "}"))
		for _, kw := range []string{"if ", "else", "case ", "default:", "for ", "select"} {
			if strings.HasPrefix(t, kw) {
				return strings.TrimSpace(strings.TrimSuffix(t, "{"))
			}
		}
	}
	return strings.TrimSpace(lines[i].Text)
}

func markSurvivors(byLine map[int]*Line, ms []model.Mutant, notesByLine map[int][]notes.Placed) {
	grouped := map[int][]model.Mutant{}
	for _, m := range ms {
		if m.Status == "LIVED" {
			grouped[m.Line] = append(grouped[m.Line], m)
		}
	}
	for n, group := range grouped {
		l, ok := byLine[n]
		if !ok {
			continue
		}
		l.Survived = len(group)
		l.Annotations = append(l.Annotations, Annotation{
			Kind: Survived, Title: plural(len(group), "mutant survived", "mutants survived"),
			Detail: changes(group), Why: why(group, notesByLine[n]),
		})
	}
}

func changes(group []model.Mutant) string {
	parts := make([]string, len(group))
	for i, m := range group {
		parts[i] = m.Description
	}
	return strings.Join(parts, "   ·   ")
}

// why is a note on the line if a person or agent wrote one, else a hint for
// the first mutant's type.
func why(group []model.Mutant, lineNotes []notes.Placed) string {
	if len(lineNotes) > 0 {
		return lineNotes[0].Text
	}
	return Hint(group[0].Type)
}

func markNotes(byLine map[int]*Line, notesByLine map[int][]notes.Placed) {
	for n, list := range notesByLine {
		l, ok := byLine[n]
		if !ok {
			continue
		}
		for _, p := range list {
			l.Annotations = append(l.Annotations, Annotation{
				Kind: Note, Title: "Note from " + authorName(p.Author), Why: p.Text,
				NoteID: p.ID, Author: p.Author, Created: p.Created, Outdated: p.Outdated,
			})
		}
	}
}

func authorName(a string) string {
	if a == "user" {
		return "you"
	}
	return a
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// hints say what test is missing, per mutation type (Gremlins type names).
var hints = map[string]string{
	"CONDITIONALS_BOUNDARY":   "No test puts the value exactly on the boundary.",
	"CONDITIONALS_NEGATION":   "No test checks the opposite outcome of this condition.",
	"ARITHMETIC_BASE":         "No test checks the exact result of this calculation.",
	"INCREMENT_DECREMENT":     "No test depends on the exact count or step.",
	"INVERT_NEGATIVES":        "No test uses a negative value here.",
	"INVERT_LOGICAL":          "No test separates the two sides of this && / ||.",
	"INVERT_ASSIGNMENTS":      "No test checks the value assigned here.",
	"INVERT_BITWISE":          "No test checks the bits this operation sets.",
	"INVERT_BWASSIGN":         "No test checks the bits this assignment sets.",
	"INVERT_LOOPCTRL":         "No test depends on this break or continue.",
	"REMOVE_SELF_ASSIGNMENTS": "No test checks the value updated here.",
}

// Hint returns the missing-test hint for a mutation type.
func Hint(mutationType string) string {
	if h, ok := hints[mutationType]; ok {
		return h
	}
	return "Add an assertion that fails when this line changes."
}
