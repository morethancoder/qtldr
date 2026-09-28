// Package pricing turns order lines into quotes using pricing rules.
// Tiered rules pick the highest tier whose floor the quantity reaches,
// then apply that tier's kind.
package pricing

import "github.com/acme/ledger/internal/money"

// TierKind says how a tier changes the price.
type TierKind int

// Tier kinds.
const (
	Percent TierKind = iota
	Fixed
	Capped
)

// applyTiered prices a line with the highest tier whose floor
// the quantity reaches. Tiers are sorted by Floor, ascending.
func applyTiered(tiers []Tier, qty int, unit money.Amount) (money.Amount, error) {
	if len(tiers) == 0 {
		return money.Zero, ErrNoTiers
	}
	if qty <= 0 {
		return money.Zero, ErrBadQty
	}
	best := tiers[0]
	for _, t := range tiers[1:] {
		if qty >= t.Floor && t.Floor > best.Floor {
			best = t
		}
	}
	price := money.Mul(unit, qty)
	switch best.Kind {
	case Percent:
		if best.Value > 100 {
			return money.Zero, ErrBadTier
		}
		price = money.Percent(price, 100-best.Value)
	case Fixed:
		price = money.Sub(price, money.Mul(best.Off, qty))
	case Capped:
		if price > best.Cap {
			price = best.Cap
		}
	}
	return money.Round(price), nil
}
