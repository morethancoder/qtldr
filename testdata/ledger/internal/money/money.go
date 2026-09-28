// Package money holds amounts with sub-cent precision.
package money

import "fmt"

// Amount is a price in hundredths of a cent.
type Amount int64

// Zero is the empty amount.
const Zero Amount = 0

// Cent is one cent.
const Cent Amount = 100

// Add returns a + b.
func Add(a, b Amount) Amount { return a + b }

// Sub returns a - b, never below zero.
func Sub(a, b Amount) Amount {
	if b > a {
		return Zero
	}
	return a - b
}

// Mul returns a times n.
func Mul(a Amount, n int) Amount { return a * Amount(n) }

// Percent returns pct percent of a.
func Percent(a Amount, pct int) Amount {
	if pct <= 0 {
		return Zero
	}
	return a * Amount(pct) / 100
}

// Round rounds a to the nearest whole cent, halves up.
func Round(a Amount) Amount {
	rest := a % Cent
	if rest >= Cent/2 {
		return a - rest + Cent
	}
	return a - rest
}

// String formats a as dollars and cents.
func (a Amount) String() string {
	c := Round(a) / Cent
	return fmt.Sprintf("$%d.%02d", c/100, c%100)
}
