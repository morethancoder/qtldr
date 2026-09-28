package order

import "testing"

func TestNewLineAndTotal(t *testing.T) {
	o := Order{ID: "o1", Lines: []Line{NewLine(" a ", 2, 100), NewLine("b", 1, 50)}}
	if o.Lines[0].SKU != "a" {
		t.Fatalf("SKU = %q, want a", o.Lines[0].SKU)
	}
	if got := o.Total(); got != 250 {
		t.Fatalf("Total = %d, want 250", got)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		o    Order
		ok   bool
	}{
		{"ok", Order{ID: "o", Lines: []Line{{SKU: "a", Qty: 1}}}, true},
		{"no id", Order{}, false},
		{"empty sku", Order{ID: "o", Lines: []Line{{Qty: 1}}}, false},
		{"zero qty", Order{ID: "o", Lines: []Line{{SKU: "a"}}}, false},
	}
	for _, c := range cases {
		if err := c.o.Validate(); (err == nil) != c.ok {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

func TestMerge(t *testing.T) {
	got := Merge([]Line{{SKU: "a", Qty: 1}, {SKU: "b", Qty: 2}, {SKU: "a", Qty: 3}})
	if len(got) != 2 || got[0].Qty != 4 || got[1].Qty != 2 {
		t.Fatalf("Merge = %+v", got)
	}
}

func TestPayShip(t *testing.T) {
	o, err := Order{}.Pay()
	if err != nil || o.Status != Paid {
		t.Fatalf("Pay: %v %v", o.Status, err)
	}
	if _, err := o.Pay(); err == nil {
		t.Fatal("paying twice should fail")
	}
	o, err = o.Ship()
	if err != nil || o.Status != Shipped {
		t.Fatalf("Ship: %v %v", o.Status, err)
	}
	if _, err := (Order{}).Ship(); err == nil {
		t.Fatal("shipping an open order should fail")
	}
}

func TestLabel(t *testing.T) {
	for s, want := range map[Status]string{Open: "open", Paid: "paid", Shipped: "shipped", 9: "unknown"} {
		if got := s.Label(); got != want {
			t.Errorf("Label(%d) = %q, want %q", s, got, want)
		}
	}
}
