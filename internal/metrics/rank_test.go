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
		r, err := Rank(g, c.metric, c.n, nil)
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
	if _, err := Rank(g, "loc", 1, nil); err == nil {
		t.Error("unknown metric should fail")
	}
}

func TestRankLowIsWorse(t *testing.T) {
	g := model.Graph{
		Nodes: []model.Node{{ID: "a", Kind: model.KindFunc}, {ID: "b", Kind: model.KindFunc}, {ID: "c", Kind: model.KindFunc}},
		Metrics: map[model.ID]model.Metrics{
			"a": {Coverage: &model.Coverage{Percent: model.Ptr(90.0)}},
			"b": {Coverage: &model.Coverage{Percent: model.Ptr(10.0), Stale: true}},
			"c": {Coverage: &model.Coverage{}}, // no statements
		},
	}
	r, err := Rank(g, Coverage, 0, nil)
	if err != nil || len(r.Items) != 2 || r.Items[0].ID != "b" || !r.Items[0].Stale || r.NotMeasured != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	r, _ = Rank(g, Coverage, 0, []model.ID{"a"})
	if len(r.Items) != 1 || r.Items[0].ID != "a" {
		t.Fatalf("scoped: %+v", r)
	}
}
