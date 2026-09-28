#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

step "Removing build output and local snapshots"
rm -rf bin .qtldr/snapshot.json testdata/ledger/.qtldr/snapshot.json testdata/ledger/target
ok "clean"
