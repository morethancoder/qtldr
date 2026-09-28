// Package argv splits command templates into argument lists without a shell
// and fills {placeholders}.
package argv

import (
	"errors"
	"fmt"
	"strings"
)

// Split splits a command line on spaces, honoring single and double quotes.
// It never runs a shell.
func Split(s string) ([]string, error) {
	var sp splitter
	for _, r := range s {
		sp.feed(r)
	}
	if sp.quote != 0 {
		return nil, fmt.Errorf("unclosed %c quote in %q", sp.quote, s)
	}
	sp.flush()
	if len(sp.words) == 0 {
		return nil, errors.New("command is empty")
	}
	return sp.words, nil
}

// splitter is the state of splitWords.
type splitter struct {
	words  []string
	cur    strings.Builder
	quote  rune
	inWord bool
}

func (sp *splitter) feed(r rune) {
	if sp.quote != 0 {
		sp.feedQuoted(r)
		return
	}
	switch {
	case r == '"' || r == '\'':
		sp.quote, sp.inWord = r, true
	case r == ' ' || r == '\t':
		sp.flush()
	default:
		sp.cur.WriteRune(r)
		sp.inWord = true
	}
}

// feedQuoted handles a rune inside quotes.
func (sp *splitter) feedQuoted(r rune) {
	if r == sp.quote {
		sp.quote = 0
		return
	}
	sp.cur.WriteRune(r)
}

func (sp *splitter) flush() {
	if sp.inWord {
		sp.words = append(sp.words, sp.cur.String())
		sp.cur.Reset()
		sp.inWord = false
	}
}

// Fill replaces {name} in every argument with vars[name].
func Fill(args []string, vars map[string]string) []string {
	pairs := make([]string, 0, 2*len(vars))
	for k, v := range vars {
		pairs = append(pairs, "{"+k+"}", v)
	}
	r := strings.NewReplacer(pairs...)
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = r.Replace(a)
	}
	return out
}
