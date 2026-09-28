package pricing

import "github.com/acme/ledger/internal/money"

// Discount takes pct percent off the quote's total.
func Discount(q Quote, pct int) Quote {
	if pct <= 0 || pct > 100 {
		return q
	}
	q.Total = money.Percent(q.Total, 100-pct)
	return q
}
