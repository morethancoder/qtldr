#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

q() { go run ./cmd/qtldr -C testdata/ledger "$@"; }

step "qtldr analyze --coverage (fixture)"
q analyze --coverage
step "qtldr show applyTiered"
q show applyTiered
step "qtldr worst"
q worst -n 5
step "qtldr check --all --fast (exit 1 is expected: the fixture has breaches)"
q check --all --fast || true
