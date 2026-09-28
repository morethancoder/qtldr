#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

require_cli go "install from https://go.dev/dl"

step "Downloading modules"
go mod download
(cd testdata/ledger && go mod download)
ok "modules ready"

step "Installing crap4go, gocognit and gremlins into $(go env GOPATH)/bin"
go install github.com/unclebob/crap4go/cmd/crap4go@latest
go install github.com/uudashr/gocognit/cmd/gocognit@latest
go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
ok "crap4go, gocognit and gremlins installed"

bash scripts/doctor.sh || warn "doctor found problems (see above)"
