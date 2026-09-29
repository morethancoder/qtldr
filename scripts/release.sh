#!/usr/bin/env bash
# Tag a release and publish it on GitHub. Releases are `go install` only
# (docs/decisions.md): the tag is the release; users get it with qtldr upgrade.
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/_lib.sh
load_env

module=github.com/morethancoder/qtldr
latest=$(git describe --tags --abbrev=0 2>/dev/null || echo none)

require_cli git "install git"
require_cli gh "install from https://cli.github.com, then: gh auth login"
require_cli go "install from https://go.dev/dl"

v=${1:-}
[[ -n "$v" ]] || die "no version given — run: make release v=X.Y.Z (latest tag: $latest)"
v=v${v#v}
[[ "$v" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || die "$v is not a version like v1.2.3"

step "Checks for $v"
[[ "$(git branch --show-current)" == main ]] || die "not on main — run: git switch main"
[[ -z "$(git status --porcelain)" ]] || die "uncommitted changes — commit or stash them first"
git fetch -q --tags origin
[[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] || die "main is not the same as origin/main — run: git pull, or git push"
git rev-parse -q --verify "refs/tags/$v" >/dev/null && die "tag $v already exists (latest: $latest)"
tool=$(sed -n 's/^const ToolVersion = "\(.*\)"$/\1/p' internal/analysis/analysis.go)
[[ "$tool" == "${v#v}" ]] || die "internal/analysis/analysis.go says ToolVersion = \"$tool\"; set it to \"${v#v}\", commit, push, and wait for CI"
ok "clean main at origin/main, ToolVersion $tool"

sha=$(git rev-parse HEAD)
run=$(gh run list --commit "$sha" --workflow ci.yml --json status,conclusion -q '.[0] | "\(.status) \(.conclusion)"')
[[ "$run" == "completed success" ]] || die "CI on ${sha:0:7} is '${run:-not started}', not green — check: gh run list --commit $sha"
ok "CI green on ${sha:0:7}"

confirm "Tag $v and publish the GitHub release?" || die "stopped; nothing was tagged"

step "Tag and publish"
git tag -a "$v" -m "qtldr $v"
git push -q origin "$v"
url=$(gh release create "$v" --verify-tag --title "qtldr $v" --generate-notes)
ok "released: $url"

step "Warm the Go module proxy"
if GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod go list -m "$module@$v" >/dev/null 2>&1; then
  ok "proxy.golang.org has $v; qtldr upgrade will find it"
else
  warn "proxy.golang.org does not list $v yet; qtldr upgrade may take a few minutes to see it"
fi
say "Install or upgrade: go install $module/cmd/qtldr@$v  (or: qtldr upgrade)"
