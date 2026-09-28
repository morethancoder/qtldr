package metrics

import (
	"testing"

	"github.com/morethancoder/qtldr/internal/model"
)

func TestRank(t *testing.T) {
	g := model.Graph{
		Nodes: []model.Node{
			{ID: "p", Kind: model.KindPackage},
			{ID: "p.a", Kind: model.KindFunc},
			{ID: "p.b", Kind: model.KindFunc},
			{ID: "p.c", Kind: model.KindFunc},
			{ID: "p.d", Kind: model.KindFunc},
		},
		Metrics: map[model.ID]model.Metrics{
			"p":   {Cognitive: model.Ptr(99)},
			"p.a": {Cognitive: model.Ptr(3), CC: model.Ptr(2)},
			"p.b": {Cognitive: model.Ptr(0), CC: model.Ptr(1)},
			"p.c": {Cognitive: model.Ptr(3), CC: model.Ptr(5)},
		},
	}
	cases := []struct {
		metric string
		n      int
		want   []model.ID
	}{
		{Cognitive, 0, []model.ID{"p.a", "p.c", "p.b"}},
		{Cognitive, 2, []model.ID{"p.a", "p.c"}},
		{CC, 1, []model.ID{"p.c"}},
	}
	for _, c := range cases {
		r, err := Rank(g, c.metric, c.n)
		if err != nil {
			t.Fatal(err)
		}
		if r.NotMeasured != 1 {
			t.Errorf("%s: not measured = %d, want 1", c.metric, r.NotMeasured)
		}
		if len(r.Items) != len(c.want) {
			t.Fatalf("%s n=%d: got %v", c.metric, c.n, r.Items)
		}
		for i, id := range c.want {
			if r.Items[i].ID != id {
				t.Errorf("%s n=%d [%d]: got %s, want %s", c.metric, c.n, i, r.Items[i].ID, id)
			}
		}
	}
	if _, err := Rank(g, "crap", 1); err == nil {
		t.Error("unknown metric should fail")
	}
}
