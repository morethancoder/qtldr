#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

require_cli crap4go "run: make setup"
require_cli gocognit "run: make setup"

# Real tool output only (CLAUDE.md rule 3). Run after changing testdata/ledger.
step "crap4go on testdata/ledger"
(cd testdata/ledger && crap4go > ../crap4go/ledger.txt && rm -rf target)
ok "testdata/crap4go/ledger.txt"

step "gocognit on testdata/ledger (-over -1 keeps complexity-0 functions)"
(cd testdata/ledger && gocognit -over -1 -json . > ../gocognit/ledger.json)
ok "testdata/gocognit/ledger.json"

require_cli gremlins "run: make setup"
step "gremlins on testdata/ledger (whole module, and internal/pricing as qtldr runs it)"
(cd testdata/ledger && GOFLAGS=-count=1 gremlins unleash -o ../gremlins/ledger-all.json > ../gremlins/ledger-all.stdout.txt 2>&1)
(cd testdata/ledger && GOFLAGS=-count=1 gremlins unleash ./internal/pricing --timeout-coefficient 12 -o ../gremlins/ledger-pricing.json > ../gremlins/ledger-pricing.stdout.txt 2>&1)
ok "testdata/gremlins/*.json"

step "Parity tests"
go test ./internal/lang/golang ./internal/mutate -run "Parity|ParseReport|Attribute"
ok "parity holds"
