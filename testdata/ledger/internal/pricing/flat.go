package pricing

import "github.com/acme/ledger/internal/money"

// applyFlat prices a line at the rule's flat unit price, or the list price.
func applyFlat(r Rule, qty int, unit money.Amount) money.Amount {
	if r.Flat > 0 {
		return money.Mul(r.Flat, qty)
	}
	return money.Mul(unit, qty)
}
