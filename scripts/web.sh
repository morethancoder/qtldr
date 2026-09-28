#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

require_cli npm "install Node LTS from https://nodejs.org"

cd web
if [[ ! -d node_modules || package-lock.json -nt node_modules ]]; then
  step "npm ci"
  npm ci --no-audit --no-fund
fi
step "type check + unit tests"
npx tsc --noEmit -p .
npx vitest run
step "vite build → web/dist"
npm run build --silent
ok "web/dist is up to date; rebuild the Go binary to embed it"
