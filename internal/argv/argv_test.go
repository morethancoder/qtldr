package argv

import (
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		in   string
		want string
		err  bool
	}{
		{"go test ./...", "go|test|./...", false},
		{`  a  "b c"  'd "e"' f`, `a|b c|d "e"|f`, false},
		{`x ""`, "x|", false},
		{`"open`, "", true},
		{"   ", "", true},
	}
	for _, c := range cases {
		got, err := Split(c.in)
		if (err != nil) != c.err || (!c.err && strings.Join(got, "|") != c.want) {
			t.Errorf("Split(%q) = %q, %v", c.in, got, err)
		}
	}
}

func TestFill(t *testing.T) {
	got := Fill([]string{"nvim", "+{line}", "{file}", "{file}:{line}"}, map[string]string{"line": "29", "file": "/a b/tier.go"})
	if strings.Join(got, "|") != "nvim|+29|/a b/tier.go|/a b/tier.go:29" {
		t.Fatalf("got %q", got)
	}
	// no variables: arguments are copied as they are
	if got := Fill([]string{"code", "{file}"}, nil); strings.Join(got, "|") != "code|{file}" {
		t.Errorf("no vars: %q", got)
	}
}
