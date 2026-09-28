#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

bash scripts/lint.sh
step "go test"
go test ./...
ok "Go tests pass"

if has_cli npm; then
  bash scripts/web.sh
  step "web/dist matches a fresh build"
  if ! git diff --quiet -- web/dist || [[ -n "$(git ls-files --others --exclude-standard web/dist)" ]]; then
    git --no-pager diff --stat -- web/dist
    die "web/dist is stale; run make web and commit it"
  fi
  ok "web/dist is fresh"
else
  warn "npm not found: skipping web tests and the dist check"
fi

step "qtldr check --all on qtldr (CRAP <= 8, cognitive <= 15; see .qtldr.toml)"
go run ./cmd/qtldr check --all --quiet
ok "all green"
