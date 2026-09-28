# Decisions and verifications

Record every "verify at build time" answer (PLAN.md §16) and every non-obvious choice here. Newest first.

## Verify-at-build-time (PLAN.md §16)

| # | Question | Answer | How checked | Date |
|---|---|---|---|---|
| — | Project name (PLAN.md §0) | qtldr ("quality, too long; didn't read") | web search found no project using it | 2026-09-28 |
| — | Go module path | `github.com/morethancoder/qtldr` | chosen by the owner at M0 | 2026-09-28 |
| 1 | Current stable Go; minimum supported | Stable: go1.27.1 and go1.26.8. `go.mod` says `go 1.26.0`, so the two latest releases are supported. Dev machine runs go1.27.0. golang.org/x/tools v0.50.0 itself requires go 1.26.0, so 1.26 is also the floor from dependencies. | `curl https://go.dev/dl/?mode=json` (stable list); `go version`; `go list -m -json golang.org/x/tools@latest` | 2026-09-28 |
| 2 | Minimal `go/packages` load mode | `NeedName \| NeedFiles \| NeedSyntax \| NeedTypes \| NeedTypesInfo \| NeedImports \| NeedModule`, `Tests: false`, **no `NeedDeps`**. Without NeedDeps, `Imports` holds stub packages (ID only), so a second cheap load (`NeedName \| NeedModule`) of the external import paths maps each to its module (nil module = stdlib). | Benchmark (warm cache, M-series Mac): fixture: 183 ms vs 584 ms with NeedDeps (7 vs 239 packages type-checked from source); golang.org/x/tools v0.50.0 (215 packages): 466 ms vs 876 ms (216 vs 491 type-checked). | 2026-09-28 |
| 3 | crap4go CC rules vs ours | **crap4go counts every `case` clause and every `select` comm clause, including `default`.** PLAN.md §6.2 says "each non-default case", which contradicts "matching crap4go" and the M0 acceptance ("CC matches crap4go on every fixture function"). We follow crap4go (default counts); no intentional difference remains. Function literals count toward the enclosing function in both. Parity: 38/38 fixture functions equal (`TestCrap4goParity`; the fixture's `order.Status.Label` has a `default` case). | Read `internal/complexity/complexity.go` of crap4go `v0.0.0-20260521190544-bee16dbdadb4`; installed with `go install github.com/unclebob/crap4go/cmd/crap4go@latest` (the module root has no main package); ran `crap4go` in `testdata/ledger`, output saved verbatim in `testdata/crap4go/ledger.txt`. | 2026-09-28 |
| 4 | gocognit as a library, or own implementation | Library: `github.com/uudashr/gocognit` v1.2.1, `gocognit.Complexity(*ast.FuncDecl) int`. Cross-checked against the CLI on every fixture function (`TestGocognitParity`). Note: the CLI's default `-over 0` omits functions with complexity 0; the capture uses `gocognit -over -1 -json .` in `testdata/ledger` → `testdata/gocognit/ledger.json`. | pkg source in module cache; CLI `-h`; real run on fixture | 2026-09-28 |
| 5 | Gremlins: install, `unleash` flags, JSON schema, mutation types, exit codes | _open (M3)_ | `gremlins unleash --help`, a real run on the fixture | — |
| 6 | go-sdk: server, tool registration, stdio | _open (M4)_ | pkg.go.dev + example | — |
| 7 | Shiki version and theme IDs in themes.json | _open (M2)_ | `shiki` bundled themes list | — |
| 8 | React Flow + ELK: node measuring, group/sub-flow API | _open (M2)_ | docs + spike | — |
| 9 | Editor CLI flags per preset | _open (M2)_ | each editor's `--help` | — |
| 10 | Claude Code hook payload and settings.json shape | _open (M1)_ | captured payload in testdata | — |
| 11 | fsnotify recursion and limits | _open (M2)_ | docs + test on macOS/Linux | — |

## Choices made while building

| Milestone | Choice | Why |
|---|---|---|
| M0 | Dependencies added: `golang.org/x/tools` v0.50.0 (named in PLAN), `github.com/uudashr/gocognit` v1.2.1 (named), `github.com/BurntSushi/toml` v1.6.0 (approved by owner). `go.yaml.in/yaml/v3` is approved for `terms.yaml` and will be added when the glossary is first read (M1 `explain`). | CLAUDE.md rule 9 |
| M0 | CLI uses the standard `flag` package, with flags allowed after positional arguments (`qtldr show applyTiered --json`). Global flags work before or after the command. | Standard library first |
| M0 | New global flag `-C <dir>` ("run as if started in dir"), like `go -C` / `git -C`. The module root is the nearest `go.mod` at or above that dir. | The fixture has its own `go.mod`; `analyze ./testdata/ledger/...` cannot cross module boundaries. |
| M0 | The snapshot has a `kind: "module"` node (ID = module path, name = last path element without `/vN`, e.g. `ledger`). Packages' `parent` points at it. | §5.2 references the module as `parent` but never defines it. |
| M0 | Package node `name`/`dir` = path relative to the module (`internal/pricing`); the root package gets `dir: "."` and the module's short name. | Matches §5.2 example. |
| M0 | Method nodes: ID `<pkg>.<Type>.<Method>`, `name: "Type.Method"`, plus `recv` and `ptr_recv` fields. | §5.1 says the pointer receiver is a field; crap4go also names methods `Type.Method`. |
| M0 | `calls_dynamic` edges point at the interface method ID (`…/httpapi.RuleSource.LoadRules`), which is not itself a node; the edge is kept when the interface type node exists. | §6.2 wording. The UI can draw it to the interface type. Revisit in M5 with VTA. |
| M0 | Edges whose endpoints are not nodes (calls into excluded or generated files) are dropped. | Avoid dangling edges. |
| M0 | Duplicate function IDs (several `init` funcs) get `#<file>` then `#<file>:<line>`. | §15; `init` is the case a single go/packages load can produce. |
| M0 | External module display name: path without the host element and `/vN` (`github.com/go-chi/chi/v5` → `go-chi/chi`). With `show_stdlib = true`, stdlib packages appear as external nodes keyed by import path. | Matches the mockup's `go-chi/chi`, `jackc/pgx`. |
| M0 | Body hash = SHA-256 of `go/printer` output of the FuncDecl without doc comment, **with blank lines removed**. go/printer drops comments for non-File nodes but keeps line breaks, so removing a comment line would otherwise leave a blank line and change the hash. | §6.2 intent: formatting and doc edits don't invalidate results. Tested in `TestBodyHash`. |
| M0 | `loc` = `end_line − line + 1` (the `func` line to the closing brace; doc comment excluded). | §5.2 example: applyTiered lines 20–48 → loc 29. |
| M0 | The golden file stores the whole scanned graph (nodes incl. signatures and body hashes, edges, cc/cognitive/loc). **Purity is not in the M0 golden**: purity is an M1 feature; the golden will be regenerated when it lands. `implements` edges are M5 and absent. | §12 lists purity in the golden, §13 schedules purity for M1. |
| M0 | `worst` accepts `cognitive` and `cc` in M0; `crap`, `coverage`, `mutation` exit 2 with "not measured yet". `show` prints CRAP/Coverage/Mutation as "not measured". | No silent zeros (CLAUDE.md rule 7). |
| M0 | `init` never overwrites an existing `.qtldr.toml`; it appends only missing `.gitignore` rules under a `# qtldr` comment. qtldr's own `.gitignore` uses `**/.qtldr/...` so the fixture's state is ignored too. | Idempotent. |
| M0 | The fixture was written so the M1 targets fall out of real runs: crap4go already reports `BestRule` CRAP 30.0 (CC 5, 0%) and `applyTiered` CRAP 14.1 (CC 11, 70.6% = 12/17 statements). Its deps: chi v5.3.2, pgx v5.11.0. | PLAN §12/§13 M1 acceptance |
| M0 | staticcheck: the installed binary was built with an older Go and cannot read Go 1.27 export data; run `go run honnef.co/go/tools/cmd/staticcheck@latest ./...` (v0.8.1 is clean). | — |

## Open questions for later milestones

- **M1 purity vs sentinel errors.** §6.2 rule 2 marks a function effectful if it reads a package-level `var`. Sentinel errors (`var ErrNoTiers = errors.New(...)`) are package-level vars, so `applyTiered`, `validate` and `Apply` would all be effectful and `internal/pricing` would not be "λ pure core" as the mockup shows. Proposal: treat a package-level var of type `error` that is never assigned after initialization as a constant. To confirm with the owner before M1.
