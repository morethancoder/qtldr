# qtldr

**qtldr** ("quality, too long; didn't read") shows a Go codebase as a map you
drill into (module → package → function → source), with every box colored by
how risky it is to change (**CRAP**) and how well its tests actually check it
(**mutation score**). The same data is available to AI agents through an MCP
server and a CLI: point at a problem on the map, tell your agent "fix what I'm
looking at", and it already knows the scores, the uncovered lines and the
surviving mutants.

## Install

```
go install github.com/morethancoder/qtldr/cmd/qtldr@latest
go install github.com/go-gremlins/gremlins/cmd/gremlins@latest   # optional, for mutation testing
```

One static binary; the web UI is embedded. Go 1.26 or newer. `go install`
puts it in `$(go env GOBIN)` (default `~/go/bin`), which must be on your PATH.

Upgrade to the newest release with `qtldr upgrade` (`--check` only says whether
there is one).

## Quick start

```
cd your/module
qtldr init                  # .qtldr.toml, .gitignore rules, tool check
qtldr analyze --coverage    # structure, complexity, coverage, CRAP
qtldr mutate                # mutation testing (slow; only changed code is re-tested later)
qtldr serve --open --watch  # the map at http://127.0.0.1:<port>
```

## Commands

| Command | What it does |
|---|---|
| `qtldr analyze [--coverage] [--mutate] [--changed]` | Scan and write `.qtldr/snapshot.json` |
| `qtldr check [--changed \| --all] [--fast] [ids or files]` | Pass/fail report (≤ 60 lines Markdown, or `--json`); exit 1 on a breach |
| `qtldr show <id>` | Everything about a package, function or type |
| `qtldr worst [--metric crap\|coverage\|mutation\|cognitive\|cc]` | Ranked functions |
| `qtldr mutate [--func id] [--pkg path] [--changed] [--force]` | Mutation testing with Gremlins, cached per function |
| `qtldr serve [--open] [--port N] [--watch]` | Web UI on 127.0.0.1 |
| `qtldr mcp` | MCP server over stdio for agents |
| `qtldr note add\|list\|resolve` | Code notes shared by people and agents |
| `qtldr explain <term>` | What a metric means (same text as the UI tooltips) |
| `qtldr hook install [--claude] [--git]` | Claude Code hook + `.mcp.json`, and a git pre-push hook |
| `qtldr provider test <name>` | Check an external language provider ([contract](docs/provider-contract.md)) |
| `qtldr upgrade [--check]` | Install the latest release with `go install` |
| `qtldr doctor`, `qtldr init`, `qtldr version` | Setup helpers |

IDs accept the full form (`github.com/acme/ledger/internal/pricing.applyTiered`)
or any unique suffix (`pricing.applyTiered`, `applyTiered`). Global flags:
`-C <dir>`, `--config`, `--json`, `--quiet`, `-v`.

## With Claude Code

```
qtldr hook install --claude
```

This adds a PostToolUse hook (`qtldr check --fast --quiet --files-from-stdin`:
breaches are shown to the agent after each edit) and registers `qtldr mcp` in
`.mcp.json`, and prints a CLAUDE.md snippet. The agent gets these tools:
`get_overview`, `get_focus`, `get_node`, `get_source`, `list_worst`, `explain`,
`add_note`, `refresh`, `check`.

## What the numbers mean

- **CRAP** = CC² × (1 − coverage)³ + CC. Target ≤ 8.
- **Mutation score** = killed ÷ (killed + survived). Target ≥ 70%.
- **Coverage** per function (target ≥ 80%), **cognitive complexity** (≤ 15),
  **churn** (commits in 12 months), **purity** (λ: no I/O, clock, globals…).

Grades run 1–10; a package shows its worst function. All targets are in
`.qtldr.toml` (`qtldr init` writes the defaults with comments).

## Develop

```
make            # list targets
make setup      # modules + crap4go, gocognit, gremlins
make ci         # lint, Go + web tests, web/dist freshness, qtldr check on itself
make demo       # run qtldr on the fixture (testdata/ledger)
make serve      # the fixture's map
make smoke      # Playwright smoke test
```

The spec is [PLAN.md](PLAN.md) (behavior) and [docs/UI.md](docs/UI.md) (UI);
build-time verifications and design choices are in
[docs/decisions.md](docs/decisions.md). Working rules for contributors (and
coding agents) are in [CLAUDE.md](CLAUDE.md).
