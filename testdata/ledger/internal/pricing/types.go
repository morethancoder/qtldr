package pricing

import (
	"errors"

	"github.com/acme/ledger/internal/money"
	"github.com/acme/ledger/internal/order"
)

// Rule prices the lines of one SKU, flat or by tiers.
type Rule struct {
	SKU      order.SKU
	Kind     RuleKind
	Priority int
	Tiers    []Tier
	Flat     money.Amount
}

// Tier is one step of a tiered rule.
type Tier struct {
	Floor int
	Kind  TierKind
	Value int
	Off   money.Amount
	Cap   money.Amount
}

// Quote is the priced result for one line.
type Quote struct {
	Line  order.Line
	Total money.Amount
	Rule  Rule
}

// RuleKind picks flat or tiered pricing.
type RuleKind int

// Rule kinds.
const (
	FlatRule RuleKind = iota
	TieredRule
)

// Errors returned while pricing.
var (
	ErrNoRule  = errors.New("pricing: no rule for this SKU")
	ErrBadRule = errors.New("pricing: flat price is negative")
	ErrNoTiers = errors.New("pricing: tiered rule has no tiers")
	ErrBadQty  = errors.New("pricing: quantity must be positive")
	ErrBadTier = errors.New("pricing: percent tier above 100")
)
