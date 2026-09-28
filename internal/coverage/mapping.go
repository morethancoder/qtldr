package coverage

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/morethancoder/qtldr/internal/model"
)

// Func is the part of a function node the mapping needs.
type Func struct {
	ID      model.ID
	File    string // relative to the module root
	Line    int
	EndLine int
}

// FuncsOf lists the function nodes of g.
func FuncsOf(g model.Graph) []Func {
	var fs []Func
	for _, n := range g.Nodes {
		if n.Kind == model.KindFunc {
			fs = append(fs, Func{ID: n.ID, File: n.File, Line: n.Line, EndLine: n.EndLine})
		}
	}
	return fs
}

// Relativize rewrites profile file names ("github.com/acme/ledger/internal/x/a.go")
// to module-relative paths ("internal/x/a.go") and drops blocks from other
// modules.
func (p Profile) Relativize(module string) Profile {
	out := Profile{Mode: p.Mode}
	for _, b := range p.Blocks {
		if rel, ok := strings.CutPrefix(b.File, module+"/"); ok {
			b.File = rel
			out.Blocks = append(out.Blocks, b)
		}
	}
	return out
}

// Without drops blocks of files in the given package directories (relative,
// "." for the root package).
func (p Profile) Without(dirs []string) Profile {
	out := Profile{Mode: p.Mode}
	for _, b := range p.Blocks {
		if !slices.Contains(dirs, dirOf(b.File)) {
			out.Blocks = append(out.Blocks, b)
		}
	}
	return out
}

func dirOf(file string) string {
	if i := strings.LastIndex(file, "/"); i >= 0 {
		return file[:i]
	}
	return "."
}

// Map computes coverage per function from a relativized profile: the blocks
// inside [Line, EndLine] of the function's file. Functions of files the
// profile does not mention get no entry (not measured). A function with no
// statements gets Percent nil.
func Map(p Profile, funcs []Func) map[model.ID]model.Coverage {
	byFile := map[string][]Block{}
	for _, b := range p.Blocks {
		byFile[b.File] = append(byFile[b.File], b)
	}
	for _, bs := range byFile {
		sortBlocks(bs)
	}
	out := map[model.ID]model.Coverage{}
	for _, f := range funcs {
		bs, ok := byFile[f.File]
		if !ok {
			continue
		}
		out[f.ID] = funcCoverage(inside(bs, f.Line, f.EndLine))
	}
	return out
}

func inside(bs []Block, from, to int) []Block {
	var out []Block
	for _, b := range bs {
		if b.StartLine >= from && b.EndLine <= to {
			out = append(out, b)
		}
	}
	return out
}

func funcCoverage(bs []Block) model.Coverage {
	c := model.Coverage{Lines: LineStates(bs)}
	for _, b := range bs {
		c.Stmts += b.NumStmt
		if b.Count > 0 {
			c.Covered += b.NumStmt
		}
	}
	c.Percent = Percent(c.Covered, c.Stmts)
	return c
}

// Percent is covered/stmts × 100 rounded to one decimal; nil with no statements.
func Percent(covered, stmts int) *float64 {
	if stmts == 0 {
		return nil
	}
	pct := math.Round(float64(covered)/float64(stmts)*1000) / 10
	return &pct
}

// LineStates classifies each line overlapped by a block: covered if every
// overlapping block ran, uncovered if none did, partial if mixed. Lines no
// block overlaps are left out.
func LineStates(bs []Block) *model.LineStates {
	ran := map[int]bool{}
	missed := map[int]bool{}
	for _, b := range bs {
		for l := b.StartLine; l <= b.EndLine; l++ {
			if b.Count > 0 {
				ran[l] = true
			} else {
				missed[l] = true
			}
		}
	}
	s := &model.LineStates{Covered: []int{}, Uncovered: []int{}, Partial: []int{}}
	for l := range ran {
		if missed[l] {
			s.Partial = append(s.Partial, l)
		} else {
			s.Covered = append(s.Covered, l)
		}
	}
	for l := range missed {
		if !ran[l] {
			s.Uncovered = append(s.Uncovered, l)
		}
	}
	slices.Sort(s.Covered)
	slices.Sort(s.Uncovered)
	slices.Sort(s.Partial)
	return s
}

// Ranges compresses sorted line numbers into "a–b" runs: [25 37 41 42 43 44]
// → ["25", "37", "41–44"].
func Ranges(lines []int) []string {
	var out []string
	for i := 0; i < len(lines); {
		j := i
		for j+1 < len(lines) && lines[j+1] == lines[j]+1 {
			j++
		}
		out = append(out, rangeText(lines[i], lines[j]))
		i = j + 1
	}
	return out
}

func rangeText(a, b int) string {
	if a == b {
		return strconv.Itoa(a)
	}
	return strconv.Itoa(a) + "–" + strconv.Itoa(b)
}
