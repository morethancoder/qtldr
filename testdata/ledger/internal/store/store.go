// Package store loads pricing rules from Postgres.
package store

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/acme/ledger/internal/money"
	"github.com/acme/ledger/internal/order"
	"github.com/acme/ledger/internal/pricing"
)

// defaultDSN is used when LEDGER_DSN is not set.
var defaultDSN = "postgres://localhost:5432/ledger"

// Config says where the database is.
type Config struct {
	DSN string
}

// Store reads rules from one connection.
type Store struct {
	conn *pgx.Conn
}

// ConfigFromEnv reads LEDGER_DSN.
func ConfigFromEnv() Config {
	if dsn := os.Getenv("LEDGER_DSN"); dsn != "" {
		return Config{DSN: dsn}
	}
	return Config{DSN: defaultDSN}
}

// Open connects to the database.
func Open(ctx context.Context, cfg Config) (*Store, error) {
	conn, err := pgx.Connect(ctx, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("store: connect %s: %w", redact(cfg.DSN), err)
	}
	return &Store{conn: conn}, nil
}

// Close ends the connection.
func (s *Store) Close(ctx context.Context) error { return s.conn.Close(ctx) }

// LoadRules reads every rule for a SKU.
func (s *Store) LoadRules(ctx context.Context, sku order.SKU) ([]pricing.Rule, error) {
	rows, err := s.conn.Query(ctx, "SELECT kind, priority, flat FROM rules WHERE sku = $1", string(sku))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []pricing.Rule
	for rows.Next() {
		r, err := scanRule(rows, sku)
		if err != nil {
			log.Printf("store: skipping rule for %s: %v", sku, err)
			continue
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func scanRule(rows pgx.Rows, sku order.SKU) (pricing.Rule, error) {
	var kind, priority int
	var flat int64
	if err := rows.Scan(&kind, &priority, &flat); err != nil {
		return pricing.Rule{}, err
	}
	return pricing.Rule{SKU: sku, Kind: pricing.RuleKind(kind), Priority: priority, Flat: money.Amount(flat)}, nil
}

// redact hides the password in a DSN.
func redact(dsn string) string {
	at := strings.LastIndex(dsn, "@")
	scheme := strings.Index(dsn, "://")
	if at < 0 || scheme < 0 || at < scheme {
		return dsn
	}
	return dsn[:scheme+3] + "***" + dsn[at:]
}
