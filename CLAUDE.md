# CLAUDE.md — working rules for building qtldr

You are building **qtldr**, specified in `PLAN.md` (what to build, in what order) and `docs/UI.md` (how the web UI looks and behaves). Read both fully before writing code. When they disagree, PLAN.md wins for behavior and data, UI.md wins for visuals; note the conflict in `docs/decisions.md`.

## Source files that are already final
- `web/src/themes.json` — theme palettes. Do not change colors without being asked; add themes by appending.
- `internal/glossary/terms.yaml` — every tooltip / `explain` / MCP glossary text. Add terms here; never hard-code explanations elsewhere.
- `docs/mockup/` — the approved visual reference. Look, don't port.

## How to work
1. **Milestones in order** (PLAN.md §13): M0 → M5. Finish a milestone's acceptance checks before starting the next. At the end of each milestone, update the checklist at the bottom of this file and commit.
2. **Verify, don't assume.** Every "verify at build time" item (PLAN.md §16) gets checked against the real tool or current docs, and the answer goes in `docs/decisions.md` (question · answer · how you checked · date). This includes Go/Node versions, Gremlins flags and JSON, go-sdk API, Shiki theme IDs, React Flow + ELK APIs, editor CLI flags, Claude Code hook format.
3. **Never invent tool output in tests.** Gremlins JSON, coverage profiles from `go test`, crap4go output and Claude Code hook payloads used as fixtures must be captured from real runs and saved under `testdata/`. Hand-written inputs are fine only for pure parsers (e.g., a coverage profile with specific blocks), and must be labeled as such.
4. **Keep the core pure.** `internal/model`, `internal/metrics`, the coverage-profile parser and mutation attribution take values and return values: no file, network, clock, env or globals. Tests for them are table-driven.
5. **Small functions.** qtldr is checked by qtldr: CRAP ≤ 8 and cognitive ≤ 15 per function in `internal/`. Run `make check` (or `make ci`) before each commit.
6. **Tests first for metrics.** Grades, roll-ups, CRAP, coverage mapping and check scoping have exact rules in PLAN.md §7–8; write the table tests from those rules before the code.
7. **Errors are specific.** Messages name the file, package or function, what went wrong, and what to run next. No silent fallbacks: missing data is shown as "not measured", never as zero.
8. **The web build is committed.** After changing `web/src`, run the web build so `web/dist` matches; CI fails otherwise.
9. **Ask before** adding a dependency not named in PLAN.md, changing a file format in PLAN.md §5, or dropping a feature from a milestone. Otherwise decide, and record non-obvious choices in `docs/decisions.md`.

## Commands
Workflows run through `make` (type `make` for the list). Rule: any workflow longer than one short command gets a one-word `make` target; recipes over ~3 lines live in `scripts/<verb>.sh` (sourcing `scripts/_lib.sh`). Add new workflows the same way.
```
make setup      # download modules, install crap4go + gocognit
make doctor     # check tools
make test       # all Go tests
make lint       # gofmt, vet, staticcheck, cognitive <= 15
make check      # qtldr check --changed on qtldr itself (run before each commit)
make ci         # lint + test + qtldr check --all on itself
make golden     # regenerate testdata/golden (review the diff!)
make capture    # re-capture crap4go/gocognit output after changing testdata/ledger
make demo       # analyze the fixture with coverage, show applyTiered, worst, check
make web        # type check, unit test and build web/ into web/dist (commit it)
make mutate     # mutation testing on the fixture (needs gremlins; results committed)
make serve      # the fixture's map at http://127.0.0.1:7777 (--watch)
make smoke      # Playwright smoke test on the fixture (artifacts in .playwright/)
make build      # bin/qtldr
make clean
```
Direct CLI use: `go run ./cmd/qtldr -C testdata/ledger show <id>` (the fixture has its own go.mod, hence `-C`).

## Style
- Go: standard library first; `gofmt`, `go vet`, `staticcheck` clean. Package names short and lower-case. Exported identifiers have doc comments.
- TypeScript: strict mode; no `any` in app code; components small; colors only through theme tokens (never literal hex in components).
- User-facing text (UI, CLI, errors): short, plain English, sentence case, no jargon without a `?` tooltip.

## Progress checklist
- [x] §0 module path chosen (`github.com/morethancoder/qtldr`)
- [x] M0 skeleton & structure
- [x] M1 coverage, CRAP, check, hooks
- [x] M2 web UI
- [x] M3 mutation
- [x] M4 agents (MCP)
- [ ] M5 breadth
