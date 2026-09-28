package model

import (
	"errors"
	"testing"
)

func TestResolve(t *testing.T) {
	ids := []ID{
		"github.com/acme/ledger/internal/pricing",
		"github.com/acme/ledger/internal/pricing.applyTiered",
		"github.com/acme/ledger/internal/pricing.Apply",
		"github.com/acme/ledger/internal/order.Order.Validate",
		"github.com/acme/ledger/internal/pricing.validate",
		"github.com/acme/ledger/internal/store.Store.Close",
		"github.com/acme/ledger/internal/httpapi.Server.Close",
	}
	cases := []struct {
		query     string
		want      ID
		ambiguous int
		notFound  bool
	}{
		{query: "github.com/acme/ledger/internal/pricing.Apply", want: ids[2]},
		{query: "pricing.applyTiered", want: ids[1]},
		{query: "applyTiered", want: ids[1]},
		{query: "internal/pricing", want: ids[0]},
		{query: "Order.Validate", want: ids[3]},
		{query: "Validate", want: ids[3]},
		{query: "validate", want: ids[4]},
		{query: "Close", ambiguous: 2},
		{query: "Tiered", notFound: true},
		{query: "ricing.Apply", notFound: true},
		{query: "", notFound: true},
	}
	for _, c := range cases {
		got, err := Resolve(ids, c.query)
		var amb *AmbiguousError
		var nf *NotFoundError
		switch {
		case c.ambiguous > 0:
			if !errors.As(err, &amb) || len(amb.Candidates) != c.ambiguous {
				t.Errorf("%q: want %d candidates, got %v", c.query, c.ambiguous, err)
			}
		case c.notFound:
			if !errors.As(err, &nf) {
				t.Errorf("%q: want not found, got %q %v", c.query, got, err)
			}
		case err != nil || got != c.want:
			t.Errorf("%q: got %q %v, want %q", c.query, got, err, c.want)
		}
	}
}

func TestNeighbors(t *testing.T) {
	g := Graph{Edges: []Edge{
		{From: "a", To: "b", Kind: EdgeCalls},
		{From: "c", To: "b", Kind: EdgeCallsDynamic},
		{From: "b", To: "d", Kind: EdgeCalls},
		{From: "b", To: "e", Kind: EdgeImports},
	}}
	in, out := g.Neighbors("b", EdgeCalls, EdgeCallsDynamic)
	if len(in) != 2 || in[0] != "a" || in[1] != "c" || len(out) != 1 || out[0] != "d" {
		t.Fatalf("in %v out %v", in, out)
	}
}

func TestDetailOfType(t *testing.T) {
	g := Graph{
		Nodes: []Node{
			{ID: "p", Kind: KindPackage},
			{ID: "p.T", Kind: KindType, Name: "T", Parent: "p"},
			{ID: "p.I", Kind: KindType, Name: "I", Parent: "p"},
			{ID: "p.T.M", Kind: KindFunc, Name: "T.M", Parent: "p", Recv: "T"},
			{ID: "p.F", Kind: KindFunc, Name: "F", Parent: "p"},
		},
		Edges: []Edge{{From: "p.T", To: "p.I", Kind: EdgeImplements}},
	}
	d, ok := g.Detail("p.T")
	if !ok || len(d.Methods) != 1 || d.Methods[0] != "p.T.M" || len(d.Implements) != 1 || d.Implements[0] != "p.I" {
		t.Fatalf("%+v", d)
	}
	if d, _ := g.Detail("p.I"); len(d.ImplementedBy) != 1 || len(d.Methods) != 0 {
		t.Fatalf("interface %+v", d)
	}
	if n, e, m := g.Counts(); n != 5 || e != 1 || m != 0 {
		t.Errorf("counts %d %d %d", n, e, m)
	}
}
