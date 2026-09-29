// Package notes stores code notes written by people and agents in
// .qtldr/notes.json and anchors them to current code (PLAN.md §5.4).
package notes

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/store"
)

// Note is one stored note. A note on a function line records the offset
// from the function's first line and the line's trimmed text.
type Note struct {
	ID         string    `json:"id"`
	Target     model.ID  `json:"target"`
	LineOffset *int      `json:"line_offset,omitempty"`
	LineText   string    `json:"line_text,omitempty"`
	Author     string    `json:"author"`
	Text       string    `json:"text"`
	Created    time.Time `json:"created"`
	Resolved   bool      `json:"resolved"`
}

// Placed is a note located in the current code. Line is 0 for notes
// without a line; Outdated means the anchored line was not found.
type Placed struct {
	Note
	Line     int  `json:"line,omitempty"`
	Outdated bool `json:"outdated"`
}

// New builds a note on target; line is absolute (0 = no line) and lines are
// the function's source lines starting at fnStart.
func New(id string, target model.ID, fnStart, line int, lines []string, text, author string, now time.Time) (Note, error) {
	n := Note{ID: id, Target: target, Author: author, Text: strings.TrimSpace(text), Created: now}
	if n.Text == "" {
		return Note{}, errors.New("note text is empty")
	}
	if line == 0 {
		return n, nil
	}
	off := line - fnStart
	if off < 0 || off >= len(lines) {
		return Note{}, fmt.Errorf("line %d is outside %s (lines %d–%d)", line, target, fnStart, fnStart+len(lines)-1)
	}
	n.LineOffset, n.LineText = &off, strings.TrimSpace(lines[off])
	return n, nil
}

// Place anchors n in the current function source: the recorded offset if
// its text still matches, else the nearest line with the same text, else
// the old offset marked outdated.
func Place(n Note, fnStart int, lines []string) Placed {
	p := Placed{Note: n}
	if n.LineOffset == nil {
		return p
	}
	off := *n.LineOffset
	if off < len(lines) && strings.TrimSpace(lines[off]) == n.LineText {
		p.Line = fnStart + off
		return p
	}
	if i := nearest(lines, n.LineText, off); i >= 0 {
		p.Line = fnStart + i
		return p
	}
	p.Line, p.Outdated = fnStart+min(off, max(len(lines)-1, 0)), true
	return p
}

// nearest is the index of the line with text closest to off, or -1.
func nearest(lines []string, text string, off int) int {
	best := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == text && (best < 0 || abs(i-off) < abs(best-off)) {
			best = i
		}
	}
	return best
}

func abs(v int) int { return max(v, -v) }

// ForTarget returns the unresolved notes of id.
func ForTarget(all []Note, id model.ID) []Note {
	var out []Note
	for _, n := range all {
		if n.Target == id && !n.Resolved {
			out = append(out, n)
		}
	}
	return out
}

// NewID returns a sortable unique ID, "n_" + a ULID.
func NewID(now time.Time) string {
	var rnd [10]byte
	_, _ = rand.Read(rnd[:])
	return "n_" + ulid(now, rnd)
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ulid encodes 48 bits of milliseconds and 80 random bits in Crockford
// base32 (26 characters).
func ulid(t time.Time, rnd [10]byte) string {
	var b [16]byte
	ms := uint64(t.UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	copy(b[6:], rnd[:])
	out := make([]byte, 26)
	var acc uint64
	bits, j := 0, 0
	out[j] = crockford[b[0]>>5] // first char: top 3 bits, with 2 leading zero bits
	j++
	acc, bits = uint64(b[0]&0x1f), 5
	for _, x := range b[1:] {
		acc = acc<<8 | uint64(x)
		bits += 8
		for bits >= 5 {
			out[j] = crockford[(acc>>(bits-5))&0x1f]
			j++
			bits -= 5
		}
	}
	return string(out[:j])
}

// Path is .qtldr/notes.json under root.
func Path(root string) string { return filepath.Join(root, store.Dir, "notes.json") }

// Load reads every note; a missing file means none.
func Load(root string) ([]Note, error) {
	b, err := os.ReadFile(Path(root))
	if errors.Is(err, fs.ErrNotExist) {
		return []Note{}, nil
	}
	if err != nil {
		return nil, err
	}
	var all []Note
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", Path(root), err)
	}
	return all, nil
}

// Save writes the notes, oldest first.
func Save(root string, all []Note) error {
	slices.SortStableFunc(all, func(a, b Note) int { return a.Created.Compare(b.Created) })
	return store.WriteJSON(Path(root), all)
}

// Update applies fn to the note with id and saves.
func Update(root, id string, fn func(*Note)) (Note, error) {
	all, err := Load(root)
	if err != nil {
		return Note{}, err
	}
	i := slices.IndexFunc(all, func(n Note) bool { return n.ID == id })
	if i < 0 {
		return Note{}, fmt.Errorf("no note %q; run `qtldr note list` to see IDs", id)
	}
	fn(&all[i])
	return all[i], Save(root, all)
}

// Append adds n and saves.
func Append(root string, n Note) error {
	all, err := Load(root)
	if err != nil {
		return err
	}
	return Save(root, append(all, n))
}

// FuncLines reads a function's lines (first to last) from its file under
// root; nil for other nodes or unreadable files.
func FuncLines(root string, n model.Node) []string {
	if n.Kind != model.KindFunc || n.Line < 1 {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(n.File)))
	if err != nil {
		return nil
	}
	lines := strings.Split(string(b), "\n")
	if n.EndLine > len(lines) {
		return nil
	}
	return lines[n.Line-1 : n.EndLine]
}

// Add resolves target in snap, builds the note and saves it.
func Add(root string, snap model.Snapshot, target string, line int, text, author string, now time.Time) (Note, error) {
	id, err := model.Resolve(snap.IDs(), target)
	if err != nil {
		return Note{}, err
	}
	node, _ := snap.Node(id)
	n, err := New(NewID(now), id, node.Line, line, FuncLines(root, node), text, author, now)
	if err != nil {
		return Note{}, err
	}
	return n, Append(root, n)
}

// PlaceAll anchors the open notes of node.
func PlaceAll(root string, all []Note, node model.Node) []Placed {
	lines := FuncLines(root, node)
	placed := []Placed{}
	for _, n := range ForTarget(all, node.ID) {
		placed = append(placed, Place(n, node.Line, lines))
	}
	return placed
}
