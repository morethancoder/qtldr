package money

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(3, 4); got != 7 {
		t.Fatalf("Add = %d, want 7", got)
	}
}

func TestSub(t *testing.T) {
	cases := []struct{ a, b, want Amount }{{10, 3, 7}, {3, 3, 0}, {3, 10, 0}}
	for _, c := range cases {
		if got := Sub(c.a, c.b); got != c.want {
			t.Errorf("Sub(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestMul(t *testing.T) {
	if got := Mul(250, 4); got != 1000 {
		t.Fatalf("Mul = %d, want 1000", got)
	}
}

func TestPercent(t *testing.T) {
	cases := []struct {
		a    Amount
		pct  int
		want Amount
	}{{1000, 50, 500}, {1000, 1, 10}, {1000, 0, 0}, {1000, -5, 0}}
	for _, c := range cases {
		if got := Percent(c.a, c.pct); got != c.want {
			t.Errorf("Percent(%d, %d) = %d, want %d", c.a, c.pct, got, c.want)
		}
	}
}

func TestRound(t *testing.T) {
	cases := []struct{ in, want Amount }{{149, 100}, {150, 200}, {151, 200}, {200, 200}, {49, 0}}
	for _, c := range cases {
		if got := Round(c.in); got != c.want {
			t.Errorf("Round(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestString(t *testing.T) {
	if got := Amount(123456).String(); got != "$12.35" {
		t.Fatalf("String = %q, want $12.35", got)
	}
}
