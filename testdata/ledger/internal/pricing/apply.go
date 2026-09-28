package pricing

import (
	"github.com/acme/ledger/internal/money"
	"github.com/acme/ledger/internal/order"
)

// Apply prices a line with the first rule for its SKU.
func Apply(rules []Rule, l order.Line) (Quote, error) {
	if err := validate(rules); err != nil {
		return Quote{}, err
	}
	for _, r := range rules {
		if r.SKU != l.SKU {
			continue
		}
		total, err := price(r, l)
		return Quote{Line: l, Total: total, Rule: r}, err
	}
	return Quote{}, ErrNoRule
}

// price runs the pricing function for the rule's kind.
func price(r Rule, l order.Line) (money.Amount, error) {
	if r.Kind == TieredRule {
		return applyTiered(r.Tiers, l.Qty, l.Unit)
	}
	return applyFlat(r, l.Qty, l.Unit), nil
}

// validate reports the first rule that cannot be used.
func validate(rules []Rule) error {
	for _, r := range rules {
		switch r.Kind {
		case FlatRule:
			if r.Flat < 0 {
				return ErrBadRule
			}
		case TieredRule:
			if len(r.Tiers) == 0 {
				return ErrNoTiers
			}
		}
	}
	return nil
}

// BestRule picks the valid rule for the line's SKU with the highest priority.
func BestRule(rules []Rule, l order.Line) (Rule, bool) {
	var best Rule
	found := false
	for _, r := range rules {
		if r.SKU != l.SKU || validate([]Rule{r}) != nil {
			continue
		}
		if r.Priority >= best.Priority {
			best, found = r, true
		}
	}
	return best, found
}
