#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

step "Regenerating testdata/golden/snapshot.json"
go test ./internal/lang/golang -run Golden -update
ok "written"
if git diff --quiet -- testdata/golden; then
  ok "no changes"
else
  warn "golden changed — review before committing:"
  git --no-pager diff --stat -- testdata/golden
fi
