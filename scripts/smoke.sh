#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

require_cli npm "install Node LTS from https://nodejs.org"
step "coverage for the fixture (the smoke test expects annotations)"
go run ./cmd/qtldr -C testdata/ledger analyze --coverage --quiet
cd web
[[ -d node_modules ]] || npm ci --no-audit --no-fund
npx playwright install chromium >/dev/null
step "playwright (artifacts in .playwright/)"
npx playwright test
ok "smoke test passed"
