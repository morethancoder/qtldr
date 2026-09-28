#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

q() { go run ./cmd/qtldr -C testdata/ledger "$@"; }

step "qtldr analyze (fixture)"
q analyze
step "qtldr show applyTiered"
q show applyTiered
step "qtldr worst --metric cognitive"
q worst --metric cognitive -n 5
