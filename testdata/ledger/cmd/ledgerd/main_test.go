package main

import "testing"

func TestAddr(t *testing.T) {
	t.Setenv("LEDGER_ADDR", "")
	if got := addr(); got != "127.0.0.1:8080" {
		t.Fatalf("addr = %q", got)
	}
	t.Setenv("LEDGER_ADDR", ":9")
	if got := addr(); got != ":9" {
		t.Fatalf("addr = %q", got)
	}
}
