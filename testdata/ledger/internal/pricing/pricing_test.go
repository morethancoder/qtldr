package pricing

import (
	"testing"

	"github.com/acme/ledger/internal/money"
	"github.com/acme/ledger/internal/order"
)

// These tests are deliberately weak: the fixture needs a function with
// low coverage and surviving mutants for qtldr to find.

func TestApplyTieredPercent(t *testing.T) {
	tiers := []Tier{{Floor: 1, Kind: Percent, Value: 0}, {Floor: 10, Kind: Percent, Value: 20}}
	got, err := applyTiered(tiers, 20, money.Cent)
	if err != nil {
		t.Fatal(err)
	}
	if got != 16*money.Cent {
		t.Fatalf("got %d, want %d", got, 16*money.Cent)
	}
}

func TestApplyTieredEmpty(t *testing.T) {
	if _, err := applyTiered(nil, 1, money.Cent); err == nil {
		t.Fatal("want an error for no tiers")
	}
}

func TestApply(t *testing.T) {
	rules := []Rule{
		{SKU: "a", Kind: FlatRule, Flat: 50},
		{SKU: "b", Kind: TieredRule, Tiers: []Tier{{Floor: 1, Kind: Percent, Value: 10}}},
		{SKU: "c", Kind: FlatRule},
	}
	cases := []struct {
		sku  order.SKU
		want money.Amount
	}{{"a", 100}, {"b", 1800}, {"c", 2000}}
	for _, c := range cases {
		q, err := Apply(rules, order.Line{SKU: c.sku, Qty: 2, Unit: 1000})
		if err != nil || q.Total != c.want {
			t.Errorf("%s: total %d err %v, want %d", c.sku, q.Total, err, c.want)
		}
	}
	if _, err := Apply(rules, order.Line{SKU: "zzz", Qty: 1}); err != ErrNoRule {
		t.Errorf("unknown SKU: err = %v", err)
	}
}

func TestValidate(t *testing.T) {
	if validate([]Rule{{Kind: FlatRule, Flat: -1}}) != ErrBadRule {
		t.Error("negative flat price should fail")
	}
	if validate([]Rule{{Kind: TieredRule}}) != ErrNoTiers {
		t.Error("tiered rule without tiers should fail")
	}
}

func TestDiscount(t *testing.T) {
	if got := Discount(Quote{Total: 1000}, 10).Total; got != 900 {
		t.Fatalf("Discount = %d, want 900", got)
	}
}
