#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

bash scripts/lint.sh
step "go test"
go test ./...
ok "tests pass"

step "qtldr check --all on qtldr (CRAP <= 8, cognitive <= 15; see .qtldr.toml)"
go run ./cmd/qtldr check --all --quiet
ok "all green"
