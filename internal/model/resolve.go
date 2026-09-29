package model

import (
	"fmt"
	"slices"
	"strings"
)

// AmbiguousError is returned when a query matches more than one ID.
type AmbiguousError struct {
	Query      string
	Candidates []ID
}

func (e *AmbiguousError) Error() string {
	ids := make([]string, len(e.Candidates))
	for i, c := range e.Candidates {
		ids[i] = "  " + string(c)
	}
	return fmt.Sprintf("%q matches %d nodes; use a longer ID:\n%s", e.Query, len(e.Candidates), strings.Join(ids, "\n"))
}

// NotFoundError is returned when a query matches no ID.
type NotFoundError struct{ Query string }

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("no node matches %q; run `qtldr worst` to list functions, or re-run `qtldr analyze` if the code is new", e.Query)
}

// Resolve finds the ID that equals query or ends with it at a segment
// boundary ("/" or "."). An exact match always wins.
func Resolve(ids []ID, query string) (ID, error) {
	if slices.Contains(ids, ID(query)) {
		return ID(query), nil
	}
	found := suffixMatches(ids, query)
	switch len(found) {
	case 0:
		return "", &NotFoundError{Query: query}
	case 1:
		return found[0], nil
	default:
		slices.Sort(found)
		return "", &AmbiguousError{Query: query, Candidates: found}
	}
}

// suffixMatches returns the ids that end with query at a segment boundary.
func suffixMatches(ids []ID, query string) []ID {
	var found []ID
	for _, id := range ids {
		if hasSegmentSuffix(string(id), query) {
			found = append(found, id)
		}
	}
	return found
}

func hasSegmentSuffix(id, suffix string) bool {
	if suffix == "" || !strings.HasSuffix(id, suffix) {
		return false
	}
	rest := id[:len(id)-len(suffix)]
	return strings.HasSuffix(rest, "/") || strings.HasSuffix(rest, ".")
}
