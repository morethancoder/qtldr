package store

import "testing"

func TestRedact(t *testing.T) {
	cases := map[string]string{
		"postgres://u:p@host/db": "postgres://***@host/db",
		"postgres://host/db":     "postgres://host/db",
		"nonsense":               "nonsense",
	}
	for in, want := range cases {
		if got := redact(in); got != want {
			t.Errorf("redact(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("LEDGER_DSN", "")
	if got := ConfigFromEnv().DSN; got != defaultDSN {
		t.Errorf("DSN = %q, want default", got)
	}
	t.Setenv("LEDGER_DSN", "postgres://x/y")
	if got := ConfigFromEnv().DSN; got != "postgres://x/y" {
		t.Errorf("DSN = %q", got)
	}
}
