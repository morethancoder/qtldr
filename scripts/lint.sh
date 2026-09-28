#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

require_cli gocognit "run: make setup"

step "gofmt"
unformatted=$(gofmt -l cmd internal)
[[ -z "$unformatted" ]] || die "not gofmt-ed (run: make fmt):"$'\n'"$unformatted"
ok "formatted"

step "go vet"
go vet ./...
ok "vet clean"

step "staticcheck"
# go run, not an installed binary: an old binary can't read newer Go export data.
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
ok "staticcheck clean"

step "cognitive complexity <= 15 (until qtldr check exists)"
if ! gocognit -over 15 internal cmd; then
  die "functions above cognitive 15 (listed above); split them"
fi
ok "all functions <= 15"
