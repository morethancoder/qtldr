// Package coverage parses Go coverage profiles and maps them onto functions
// (pure), and runs the configured test command to produce one (effectful,
// run.go).
package coverage

import (
	"bufio"
	"cmp"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// Block is one line of a coverage profile.
type Block struct {
	File      string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	NumStmt   int
	Count     int
}

// Profile is a parsed coverage profile with duplicate blocks merged.
type Profile struct {
	Mode   string
	Blocks []Block
}

// Parse reads a profile: a "mode: x" line, then lines of
// "file:startLine.startCol,endLine.endCol numStmt count". Blocks that appear
// more than once (several test binaries covering one package) are merged by
// summing their counts.
func Parse(r io.Reader) (Profile, error) {
	var p Profile
	merged := map[Block]int{} // block with Count 0 → index in p.Blocks
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if err := p.addLine(line, merged); err != nil {
			return Profile{}, fmt.Errorf("coverage profile line %d: %w", n, err)
		}
	}
	if err := sc.Err(); err != nil {
		return Profile{}, err
	}
	if p.Mode == "" {
		return Profile{}, fmt.Errorf("coverage profile has no mode line; is it a go test -coverprofile file?")
	}
	return p, nil
}

func (p *Profile) addLine(line string, merged map[Block]int) error {
	if line == "" {
		return nil
	}
	if mode, ok := strings.CutPrefix(line, "mode: "); ok {
		p.Mode = mode
		return nil
	}
	b, err := parseBlock(line)
	if err != nil {
		return err
	}
	key := b
	key.Count = 0
	if i, ok := merged[key]; ok {
		p.Blocks[i].Count += b.Count
		return nil
	}
	merged[key] = len(p.Blocks)
	p.Blocks = append(p.Blocks, b)
	return nil
}

func parseBlock(line string) (Block, error) {
	colon := strings.LastIndex(line, ":")
	if colon < 0 {
		return Block{}, fmt.Errorf("%q: missing ':'", line)
	}
	var b Block
	b.File = line[:colon]
	fields := strings.FieldsFunc(line[colon+1:], func(r rune) bool { return r == '.' || r == ',' || r == ' ' })
	if len(fields) != 6 {
		return Block{}, fmt.Errorf("%q: want file:l.c,l.c stmts count", line)
	}
	dst := []*int{&b.StartLine, &b.StartCol, &b.EndLine, &b.EndCol, &b.NumStmt, &b.Count}
	for i, s := range fields {
		v, err := strconv.Atoi(s)
		if err != nil {
			return Block{}, fmt.Errorf("%q: %q is not a number", line, s)
		}
		*dst[i] = v
	}
	return b, nil
}

// Files returns the distinct file names in the profile, sorted.
func (p Profile) Files() []string {
	var files []string
	for _, b := range p.Blocks {
		files = append(files, b.File)
	}
	slices.Sort(files)
	return slices.Compact(files)
}

// sortBlocks orders blocks by file and position.
func sortBlocks(bs []Block) {
	slices.SortFunc(bs, func(a, b Block) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.StartLine, b.StartLine), cmp.Compare(a.StartCol, b.StartCol))
	})
}
