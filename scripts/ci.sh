#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

bash scripts/lint.sh
step "go test"
go test ./...
ok "all green"
