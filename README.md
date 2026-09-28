# qtldr — starter kit

Drop these files into an empty folder, `git init`, and start Claude Code there.

| File | What it is |
|---|---|
| `PLAN.md` | The build spec: architecture, data formats, metrics, commands, MCP tools, tests, milestones M0–M5 |
| `CLAUDE.md` | Working rules Claude Code follows every session |
| `docs/UI.md` | The web UI spec, written from the approved mockup |
| `docs/mockup/` | The approved mockup source (visual reference only) |
| `docs/decisions.md` | Where Claude records every "verify at build time" answer |
| `web/src/themes.json` | The 9 themes (Gruvbox, GitHub, Tokyo Night, Catppuccin, Nord, Dracula, Solarized) — UI, map and code colors |
| `internal/glossary/terms.yaml` | Every `?` tooltip text, also used by the CLI and MCP |

## First message to Claude Code

```
Read CLAUDE.md, PLAN.md and docs/UI.md fully. Then:
1. List anything in the plan that is unclear or contradictory, and ask me before coding.
2. Do the M0 "verify at build time" checks that M0 needs (Go version, go/packages mode,
   crap4go CC rules, gocognit) and record them in docs/decisions.md.
3. Build M0 only. Stop when its acceptance checks pass and show me the results.
```

Go one milestone at a time. After M2 you can run `qtldr serve --open` on the fixture and compare it with the mockup.

## Before you start
- Pick the Go module path (PLAN.md §0), e.g. `github.com/<you>/qtldr`.
- Install: Go (latest), Node LTS, git. Gremlins is needed from M3; crap4go only for the CC parity test.
