#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

missing=0
check() { # check NAME REQUIRED HINT
  if has_cli "$1"; then ok "$1"; elif [[ $2 == yes ]]; then err "$1 not found — $3"; missing=1; else warn "$1 not found (optional) — $3"; fi
}

step "Tools"
check go yes "install from https://go.dev/dl"
check git yes "install git"
check crap4go yes "run: make setup"
check gocognit yes "run: make setup"
check gremlins no "needed for qtldr mutate; run: make setup"
check node no "needed to rebuild the web UI (make web); install Node LTS"

step "qtldr doctor"
go run ./cmd/qtldr doctor || missing=1

exit "$missing"
