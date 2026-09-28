package golang

import "testing"

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, file string
		want          bool
	}{
		{"**/*_mock.go", "store_mock.go", true},
		{"**/*_mock.go", "internal/store/store_mock.go", true},
		{"**/*_mock.go", "internal/store/store.go", false},
		{"**/mocks/**", "internal/mocks/a.go", true},
		{"**/mocks/**", "mocks/a.go", true},
		{"**/mocks/**", "internal/mocksy/a.go", false},
		{"internal/*.go", "internal/a.go", true},
		{"internal/*.go", "internal/x/a.go", false},
		{"internal/**/a.go", "internal/a.go", true},
		{"internal/**/a.go", "internal/x/y/a.go", true},
		{"[", "a.go", false},
	}
	for _, c := range cases {
		if got := MatchGlob(c.pattern, c.file); got != c.want {
			t.Errorf("MatchGlob(%q, %q) = %v, want %v", c.pattern, c.file, got, c.want)
		}
	}
}
