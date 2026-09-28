package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/acme/ledger/internal/order"
	"github.com/acme/ledger/internal/pricing"
)

type fakeRules struct {
	rules []pricing.Rule
	err   error
}

func (f fakeRules) LoadRules(context.Context, order.SKU) ([]pricing.Rule, error) {
	return f.rules, f.err
}

func get(t *testing.T, src RuleSource, url string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	New(src).Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec
}

func TestQuote(t *testing.T) {
	src := fakeRules{rules: []pricing.Rule{{SKU: "a", Kind: pricing.FlatRule, Flat: 100}}}
	rec := get(t, src, "/quote/a?qty=3&discount=50")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Body.String(); got != "{\"sku\":\"a\",\"qty\":3,\"total\":\"$0.02\"}\n" {
		t.Fatalf("body %q", got)
	}
}

func TestQuoteErrors(t *testing.T) {
	cases := []struct {
		src  RuleSource
		url  string
		want int
	}{
		{fakeRules{}, "/quote/a?qty=x", http.StatusBadRequest},
		{fakeRules{err: errors.New("down")}, "/quote/a", http.StatusBadGateway},
		{fakeRules{}, "/quote/a", http.StatusNotFound},
	}
	for _, c := range cases {
		if rec := get(t, c.src, c.url); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d", c.url, rec.Code, c.want)
		}
	}
}

func TestHealth(t *testing.T) {
	if rec := get(t, fakeRules{}, "/healthz"); rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
}
