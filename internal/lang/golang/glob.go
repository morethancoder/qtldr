package golang

import (
	"path"
	"strings"
)

// MatchGlob reports whether the slash-separated file path matches pattern.
// "**" matches any number of path segments (including none); other segments
// use path.Match rules. An invalid pattern matches nothing.
func MatchGlob(pattern, file string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(file, "/"))
}

func matchSegments(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			return matchAnySuffix(pat[1:], segs)
		}
		if len(segs) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], segs[0]); err != nil || !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}

func matchAnySuffix(pat, segs []string) bool {
	for i := 0; i <= len(segs); i++ {
		if matchSegments(pat, segs[i:]) {
			return true
		}
	}
	return false
}

// MatchAny reports whether file matches any of the patterns.
func MatchAny(patterns []string, file string) bool { return excluded(patterns, file) }

// excluded reports whether file matches any of the patterns.
func excluded(patterns []string, file string) bool {
	for _, p := range patterns {
		if MatchGlob(p, file) {
			return true
		}
	}
	return false
}
