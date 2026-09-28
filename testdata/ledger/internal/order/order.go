// Package order models customer orders.
package order

import (
	"errors"
	"strings"

	"github.com/acme/ledger/internal/money"
)

// Status is the life-cycle state of an order.
type Status int

// Order states.
const (
	Open Status = iota
	Paid
	Shipped
)

// SKU identifies a product.
type SKU string

// Line is one product and quantity in an order.
type Line struct {
	SKU  SKU
	Qty  int
	Unit money.Amount
}

// Order is a set of lines.
type Order struct {
	ID     string
	Lines  []Line
	Status Status
}

// NewLine builds a line, trimming the SKU.
func NewLine(sku string, qty int, unit money.Amount) Line {
	return Line{SKU: SKU(strings.TrimSpace(sku)), Qty: qty, Unit: unit}
}

// Subtotal is the line's price before rules.
func (l Line) Subtotal() money.Amount { return money.Mul(l.Unit, l.Qty) }

// Total is the sum of all line subtotals.
func (o Order) Total() money.Amount {
	total := money.Zero
	for _, l := range o.Lines {
		total = money.Add(total, l.Subtotal())
	}
	return total
}

// Validate reports the first problem with the order.
func (o Order) Validate() error {
	if o.ID == "" {
		return errors.New("order has no ID")
	}
	for _, l := range o.Lines {
		if l.SKU == "" || l.Qty <= 0 {
			return errors.New("order has an empty line")
		}
	}
	return nil
}

// Merge adds lines for the same SKU together.
func Merge(lines []Line) []Line {
	idx := map[SKU]int{}
	var out []Line
	for _, l := range lines {
		if i, ok := idx[l.SKU]; ok {
			out[i].Qty += l.Qty
			continue
		}
		idx[l.SKU] = len(out)
		out = append(out, l)
	}
	return out
}

// Pay moves an open order to paid.
func (o Order) Pay() (Order, error) {
	if o.Status != Open {
		return o, errors.New("order is not open")
	}
	o.Status = Paid
	return o, nil
}

// Ship moves a paid order to shipped.
func (o Order) Ship() (Order, error) {
	if o.Status != Paid {
		return o, errors.New("order is not paid")
	}
	o.Status = Shipped
	return o, nil
}

// Label names a status.
func (s Status) Label() string {
	switch s {
	case Open:
		return "open"
	case Paid:
		return "paid"
	case Shipped:
		return "shipped"
	default:
		return "unknown"
	}
}
