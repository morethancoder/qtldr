// Package httpapi serves quotes over HTTP.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/acme/ledger/internal/money"
	"github.com/acme/ledger/internal/order"
	"github.com/acme/ledger/internal/pricing"
)

// RuleSource loads pricing rules; *store.Store implements it.
type RuleSource interface {
	LoadRules(ctx context.Context, sku order.SKU) ([]pricing.Rule, error)
}

// Server holds the HTTP handlers.
type Server struct {
	rules RuleSource
}

// quoteResponse is the JSON body of a quote.
type quoteResponse struct {
	SKU   string `json:"sku"`
	Qty   int    `json:"qty"`
	Total string `json:"total"`
}

// New builds a server.
func New(rules RuleSource) *Server { return &Server{rules: rules} }

// Routes returns the router.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", handleHealth)
	r.Get("/quote/{sku}", s.handleQuote)
	return r
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleQuote(w http.ResponseWriter, req *http.Request) {
	line, err := parseLine(chi.URLParam(req, "sku"), req.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rules, err := s.rules.LoadRules(req.Context(), line.SKU)
	if err != nil {
		http.Error(w, "could not load rules", http.StatusBadGateway)
		return
	}
	q, err := pricing.Apply(rules, line)
	if err == pricing.ErrNoRule {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if pct, ok := discountParam(req.URL.Query()); ok {
		q = pricing.Discount(q, pct)
	}
	writeJSON(w, quoteResponse{SKU: string(line.SKU), Qty: line.Qty, Total: q.Total.String()})
}

func parseLine(sku string, q map[string][]string) (order.Line, error) {
	qty, err := strconv.Atoi(first(q["qty"], "1"))
	if err != nil {
		return order.Line{}, err
	}
	unit, err := strconv.ParseInt(first(q["unit"], "0"), 10, 64)
	if err != nil {
		return order.Line{}, err
	}
	return order.NewLine(sku, qty, money.Amount(unit)), nil
}

func discountParam(q map[string][]string) (int, bool) {
	pct, err := strconv.Atoi(first(q["discount"], ""))
	return pct, err == nil
}

func first(vals []string, def string) string {
	if len(vals) == 0 {
		return def
	}
	return vals[0]
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
