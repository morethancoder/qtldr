package lib

import "testing"

func TestDouble(t *testing.T) {
	if Double(2) != 4 {
		t.Fatal("x")
	}
}
