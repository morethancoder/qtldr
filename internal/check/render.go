package check

import (
	"fmt"
	"strings"
)

// MaxLines is the Markdown report's hard limit.
const MaxLines = 60

// Markdown renders the report in at most MaxLines lines. When it would be
// longer, the functions table is cut first, then "not measured", then the
// breaches; every cut section ends with "… and N more".
func Markdown(r Report) string {
	header := []string{title(r)}
	sections := []section{
		{keep: 2, lines: breachLines(r)},
		{keep: 1, lines: notMeasuredLines(r)},
		{keep: 1, lines: noteLines(r)},
		{keep: 3, lines: tableLines(r)},
	}
	fit(sections, MaxLines-len(header))
	out := header
	for _, s := range []int{0, 3, 1, 2} { // print order: breaches, table, not measured, notes
		out = append(out, sections[s].lines...)
	}
	return strings.Join(out, "\n") + "\n"
}

type section struct {
	keep  int // header lines that are never cut
	lines []string
}

// fit trims sections, lowest priority last in the slice first, until the
// total fits budget.
func fit(ss []section, budget int) {
	for i := len(ss) - 1; i >= 0 && total(ss) > budget; i-- {
		over := total(ss) - budget
		s := &ss[i]
		body := len(s.lines) - s.keep
		if body <= 0 {
			continue
		}
		cut := min(body, over+1) // +1 for the "… and N more" line
		if cut == body && body == 1 {
			continue
		}
		s.lines = append(s.lines[:len(s.lines)-cut], fmt.Sprintf("- … and %d more", cut))
	}
}

func total(ss []section) int {
	n := 0
	for _, s := range ss {
		n += len(s.lines)
	}
	return n
}

func title(r Report) string {
	what := fmt.Sprintf("%d functions", len(r.Functions))
	if r.Scope == "changed" {
		what = fmt.Sprintf("%d changed functions", len(r.Functions))
	}
	t := "## qtldr check · " + what
	if r.Base != "" {
		t += " · base " + r.Base
	}
	if r.Fast {
		t += " · fast"
	}
	if r.Pass {
		return t + " · pass"
	}
	return t
}

func breachLines(r Report) []string {
	if len(r.Breaches) == 0 {
		return nil
	}
	lines := []string{fmt.Sprintf("### Breaches (%d)", len(r.Breaches))}
	for _, b := range r.Breaches {
		lines = append(lines, fmt.Sprintf("- [%s] %s — %s (limit %s) — %s", b.Kind, b.Name, value(b.Kind, b.Value), value(b.Kind, b.Limit), b.Hint))
		if len(b.Details) > 0 {
			lines = append(lines, "  - "+strings.Join(b.Details, "   · "))
		}
	}
	return lines
}

func value(kind string, v float64) string {
	switch kind {
	case KindCoverage, KindMutation:
		return fmt.Sprintf("%g%%", v)
	case KindCognitive:
		return fmt.Sprintf("%g", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func tableLines(r Report) []string {
	if len(r.Functions) == 0 {
		return []string{"No functions in scope."}
	}
	heading := "### Functions"
	if r.Scope == "changed" {
		heading = "### Changed"
	}
	lines := []string{heading, "| function | CRAP | cov | mut | cognitive |", "|---|---|---|---|---|"}
	for _, row := range r.Functions {
		stale := ""
		if row.Stale {
			stale = " (stale)"
		}
		lines = append(lines, fmt.Sprintf("| %s%s | %s | %s | %s | %s |", row.Name, stale,
			num(row.CRAP, "%.1f"), num(row.Coverage, "%g%%"), num(row.Mutation, "%g%%"), intOr(row.Cognitive)))
	}
	return lines
}

func num(v *float64, format string) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf(format, *v)
}

func intOr(v *int) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprint(*v)
}

func notMeasuredLines(r Report) []string {
	if len(r.NotMeasured) == 0 {
		return nil
	}
	lines := []string{"### Not measured"}
	for _, m := range r.NotMeasured {
		lines = append(lines, fmt.Sprintf("- %s — %s", m.Name, m.Reason))
	}
	return lines
}

func noteLines(r Report) []string {
	if len(r.Notes) == 0 {
		return nil
	}
	lines := []string{"### Notes"}
	for _, n := range r.Notes {
		at := n.Name
		if n.Line > 0 {
			at = fmt.Sprintf("%s:%d", n.Name, n.Line)
		}
		lines = append(lines, fmt.Sprintf("- %s (%s): %s", at, n.Author, n.Text))
	}
	return lines
}
