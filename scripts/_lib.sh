# Shared helpers for scripts/*.sh. Source it; don't run it.

if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'; C_RED=$'\033[31m'; C_GREEN=$'\033[32m'
  C_YELLOW=$'\033[33m'; C_CYAN=$'\033[36m'
else
  C_RESET=""; C_BOLD=""; C_RED=""; C_GREEN=""; C_YELLOW=""; C_CYAN=""
fi

say()  { printf '%s\n' "$*"; }
step() { printf '%s==>%s %s%s%s\n' "$C_CYAN" "$C_RESET" "$C_BOLD" "$*" "$C_RESET"; }
ok()   { printf '%s✓%s %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf '%s!%s %s\n' "$C_YELLOW" "$C_RESET" "$*" >&2; }
err()  { printf '%s✗%s %s\n' "$C_RED" "$C_RESET" "$*" >&2; }
die()  { err "$*"; exit 1; }

# load_env sources .env when present (qtldr needs no env vars today).
load_env() {
  if [[ -f .env ]]; then
    set -a; source .env; set +a
  fi
}

# require_env NAME "where to get it"
require_env() {
  local name=$1 hint=${2:-}
  [[ -n "${!name:-}" ]] && return 0
  err "$name is not set${hint:+ — $hint}"
  exit 1
}

has_cli() { command -v "$1" >/dev/null 2>&1; }

# require_cli NAME "how to install"
require_cli() {
  has_cli "$1" || die "$1 not found${2:+ — $2}"
}

# confirm "question" — auto-yes when CI=1.
confirm() {
  [[ -n "${CI:-}" ]] && return 0
  local reply
  read -r -p "$1 [y/N] " reply
  [[ "$reply" =~ ^[Yy]$ ]]
}
