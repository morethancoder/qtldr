# qtldr — build plan

**qtldr** shows a Go codebase as a map you drill into (module → package → function → source), with every box colored by how risky it is to change (CRAP) and how well its tests actually check it (mutation score). The same data is available to AI agents through an MCP server and a CLI. A human spots a problem on the map, points at it, and tells an agent "fix that"; the agent already knows the scores, the uncovered lines and the surviving mutants.

The name stands for "quality, too long; didn't read": the map and the `check` report are the short version of your code's quality. This plan replaces the earlier Python qtldr plan (an adapter-only report CLI). What carries over from it: the "changed code vs a base ref" scope, the short pass/fail report with exit codes (now `qtldr check`), the external-adapter idea (now the external language provider, §6.3), hook installation, and the rule that tool output is never invented in tests.

Inspiration: Uncle Bob's `uml-viewer` (drill-down diagram colored by CRAP + mutation, agent in the loop) and `crap4go` (reference CRAP implementation for Go). qtldr is written from scratch and depends on neither.

This document is the specification. Where it says **verify at build time**, run the command or read the current docs, record the answer in `docs/decisions.md`, and do not proceed on assumption.

## 0. Name

The name is **qtldr**. Use it everywhere: module path, binary, `.qtldr/`, `.qtldr.toml`, MCP server name, docs. Pick the Go module path (e.g. `github.com/<you>/qtldr`) at M0.

---

## 1. Decisions (fixed)

| Decision | Choice | Reason |
|---|---|---|
| Backend language | Go, latest stable (verify; support the two latest releases) | `go/packages`, `go/ast`, `go/types` are the most accurate way to read Go; single static binary |
| Frontend | Vite + React + TypeScript | Mature ecosystem for the diagram libraries below |
| Diagram | `@xyflow/react` (React Flow) + `elkjs` layered layout | HTML cards (badges, tooltips) + clean automatic routing |
| Code highlighting | Shiki, Go grammar only, theme taken from the active qtldr theme | Authentic editor themes (gruvbox, github, …) |
| Delivery | One binary; web UI built to `web/dist` and embedded with `go:embed` | `go install` must work without Node, so `web/dist` is committed; CI checks it matches a fresh build |
| Faces | CLI · local web UI (`serve`) · MCP server over stdio (`mcp`), all reading the same snapshot | No CLI-vs-MCP choice needed |
| MCP SDK | `github.com/modelcontextprotocol/go-sdk` (official) | Verify current API at build time |
| Mutation engine | Gremlins behind a `Mutator` interface; own AST mutator is a later option | Mature enough, JSON output, `--diff` support |
| Config | `.qtldr.toml` at module root | One file |
| State dir | `.qtldr/` at module root | §5.6 says what is committed |
| Scope v1 | Go only, one module (`go.mod` at root); `go.work` later | Keep v1 small |
| Platforms | Linux, macOS; Windows best-effort | |
| Network | Web server binds `127.0.0.1` only; mutating endpoints need a per-run token | The server can launch editors |

---

## 2. Repository layout

```
qtldr/
├── go.mod
├── cmd/qtldr/main.go              # wires CLI → internal packages; no logic
├── internal/
│   ├── cli/                       # command parsing, output formatting
│   ├── config/                    # load/validate .qtldr.toml, defaults
│   ├── model/                     # PURE: Snapshot, Node, Edge, Metrics, IDs (language-neutral)
│   ├── metrics/                   # PURE: CRAP, grades, roll-ups, thresholds, purity propagation
│   ├── lang/                      # Provider interface + registry
│   │   ├── golang/                # Go provider: scan, CC, cognitive, coverage mapping, purity facts, source
│   │   └── external/              # subprocess provider speaking the JSON contract (§6.3)
│   ├── coverage/                  # run go test -coverprofile (effectful) + profile parser (pure)
│   ├── mutate/                    # Mutator interface; gremlins adapter; attribution; body-hash cache
│   ├── gitx/                      # changed files/hunks vs ref, churn
│   ├── store/                     # atomic read/write of .qtldr/*
│   ├── notes/                     # code notes (human + agent), anchoring
│   ├── focus/                     # what the human is pointing at; agent bridge (tmux optional)
│   ├── editor/                    # "open in editor" command templates
│   ├── check/                     # thresholds → report (Markdown ≤ 60 lines / JSON) + exit code
│   ├── server/                    # HTTP API + SSE + embedded web assets + file watcher
│   ├── mcpserver/                 # MCP tools over stdio
│   └── glossary/terms.yaml        # embedded; single source for UI tooltips, CLI explain, MCP
├── web/
│   ├── src/                       # React app (docs/UI.md)
│   │   └── themes.json            # theme palettes; single source of truth for UI + map + code colors
│   └── dist/                      # built assets, committed, embedded
├── testdata/ledger/               # fixture module (§12)
├── docs/
│   ├── UI.md                      # UI spec (from the approved mockup)
│   ├── mockup/                    # the approved mockup source, for reference only
│   ├── decisions.md               # answers to every "verify at build time"
│   └── provider-contract.md       # external provider JSON contract (M5)
├── CLAUDE.md
└── PLAN.md
```

Architecture rule (dogfooding the idea): `model`, `metrics`, the coverage-profile parser and the mutation attribution logic are **pure**: no I/O, no clock, no globals. Effects live in `cli`, `server`, `mcpserver`, the runners and `store`.

---

## 3. Configuration: `.qtldr.toml`

`qtldr init` writes this with defaults. All keys are optional.

```toml
[project]
include = ["./..."]                        # package patterns to analyze
exclude = ["**/*_mock.go", "**/mocks/**"]  # globs on file paths; generated files are always excluded
base_ref = "main"                          # default ref for the "changed" scope
external_modules = "collapsed"             # collapsed | hidden
show_stdlib = false

[coverage]
command = "go test -covermode=count -coverprofile={profile} {packages}"  # {profile}, {packages} are filled in
coverpkg = "own"                           # own (package's own tests) | module (adds -coverpkg=./...)
timeout = "10m"

[mutation]
engine = "gremlins"
args = []                                  # extra args passed through
timeout = "30m"
max_functions = 40                         # per run; highest-CRAP first; the rest are reported as skipped

[thresholds]                               # used by `qtldr check` and for "problem" chips in the UI
crap_max = 8
cognitive_max = 15
coverage_min = 80                          # percent, per changed function
mutation_min = 70                          # percent, per changed function with killed+survived ≥ 1
critical_paths = []                        # globs; coverage_min becomes 100 there

[purity]
allow = []                                 # function IDs to treat as pure even if the heuristic says effectful

[churn]
window_months = 12

[ui]
theme = "gruvbox-dark"                     # id from web/src/themes.json or a custom theme
theme_light = ""                           # optional: follow the OS light/dark setting with this pair
theme_dark = ""
port = 0                                   # 0 = pick a free port

[editor]
preset = "vscode"                          # vscode | cursor | zed | goland | nvim-remote | vim-tmux | custom
command = ""                               # used when preset = "custom", e.g. "nvim +{line} {file}"
nvim_socket = ""                           # for nvim-remote; default $NVIM
terminal = ""                              # for vim-tmux, e.g. "tmux new-window"

[agent]
tmux_target = ""                           # optional: "Send to agent" also types the prompt into this tmux pane
```

Validation: unknown top-level tables → error (exit 2). Unknown keys inside known tables → warning. `exclude` globs support `**`. Custom themes: any `.qtldr/themes/*.json` matching the theme schema (§11.3) is added to the theme list.

---

## 4. Commands

Global flags: `--config <path>`, `--json`, `--quiet`, `-v`.

| Command | What it does |
|---|---|
| `qtldr init` | Write `.qtldr.toml`; add the ignore rules from §5.6 to `.gitignore`; run `doctor`. |
| `qtldr doctor` | Check `go`, `git`, `gremlins` (optional), the editor command; print versions and hints. Exit 2 only if `go` or `git` is missing. |
| `qtldr analyze [pkgs...] [--coverage] [--mutate] [--changed]` | Scan structure + complexity (always, fast). `--coverage` runs tests. `--mutate` runs mutation (slow). Writes the snapshot. |
| `qtldr serve [--open] [--port N] [--watch]` | Web UI on 127.0.0.1. `--watch` re-scans on file changes and pushes updates. |
| `qtldr mcp` | MCP server over stdio (§9). |
| `qtldr check [--changed \| --all] [--fast] [--files-from-stdin] [ids or files...]` | Evaluate thresholds (§8). `--fast` = no test run. Prints ≤ 60-line Markdown or `--json`. Exit 0 pass · 1 breach · 2 error. |
| `qtldr show <id> [--json]` | Everything about one node: metrics, callers, callees, uncovered lines, survivors, notes. |
| `qtldr worst [--metric crap\|coverage\|mutation\|cognitive] [-n 10]` | Ranked list. |
| `qtldr mutate [--func <id>] [--pkg <path>] [--changed]` | Run mutation for a scope; reuse cached results for unchanged functions. |
| `qtldr explain <term>` | Glossary entry (the same text as UI tooltips). |
| `qtldr note add <id> [--line N] <text>` · `note list [id]` · `note resolve <note-id>` | Code notes (§5.4). |
| `qtldr hook install [--claude] [--git]` | Install hooks (§10). |

IDs accept the full form (`github.com/acme/ledger/internal/pricing.applyTiered`) or any unambiguous suffix (`pricing.applyTiered`, `applyTiered`). Ambiguous → exit 2 listing the candidates.

---

## 5. Data model and files

### 5.1 IDs
- Package: import path, e.g. `github.com/acme/ledger/internal/pricing`
- Function: `<pkg>.<Func>`, e.g. `…/internal/pricing.applyTiered`
- Method: `<pkg>.<Type>.<Method>` (pointer receiver is a field, not part of the ID)
- Type: `<pkg>.<Type>`
- External module: module path, e.g. `github.com/go-chi/chi/v5`
- Function literals (closures) belong to their enclosing function; their complexity counts there.

### 5.2 Snapshot: `.qtldr/snapshot.json` (gitignored; cheap to rebuild)

```json
{
  "schema": 1,
  "tool_version": "0.1.0",
  "module": "github.com/acme/ledger",
  "generated": "2026-09-28T10:00:00Z",
  "git": {"head": "3f2a91c…", "dirty": false},
  "runs": {"structure": "2026-09-28T10:00:00Z", "coverage": null, "mutation": null},
  "nodes": [
    {"id": "…/internal/pricing", "kind": "package", "name": "internal/pricing", "parent": "github.com/acme/ledger",
     "dir": "internal/pricing", "pure": true},
    {"id": "…/internal/pricing.applyTiered", "kind": "func", "name": "applyTiered", "parent": "…/internal/pricing",
     "file": "internal/pricing/tier.go", "line": 20, "end_line": 48, "exported": false,
     "signature": "func applyTiered(tiers []Tier, qty int, unit money.Amount) (money.Amount, error)",
     "body_hash": "sha256:…", "pure": true, "effects": []},
    {"id": "…/internal/pricing.Tier", "kind": "type", "type_kind": "struct", "name": "Tier",
     "parent": "…/internal/pricing", "file": "internal/pricing/types.go", "line": 14,
     "fields": [{"name": "Floor", "type": "int"}], "implements": []},
    {"id": "github.com/go-chi/chi/v5", "kind": "external", "name": "go-chi/chi"}
  ],
  "edges": [
    {"from": "…/internal/httpapi", "to": "…/internal/pricing", "kind": "imports"},
    {"from": "…pricing.Apply", "to": "…pricing.applyTiered", "kind": "calls"},
    {"from": "…pricing.Rule", "to": "…pricing.Pricer", "kind": "implements"}
  ],
  "metrics": {
    "…/internal/pricing.applyTiered": {
      "cc": 11, "cognitive": 14, "loc": 29, "churn": 17, "churn_scope": "file",
      "coverage": {"stmts": 17, "covered": 12, "percent": 70.6, "stale": false,
                   "lines": {"covered": [21, 22, 24], "uncovered": [25, 37, 41, 43, 44], "partial": []}},
      "crap": 14.0,
      "mutation": {"killed": 9, "survived": 7, "not_covered": 3, "timed_out": 0,
                   "score": 56, "stale": false,
                   "mutants": [{"line": 29, "col": 13, "type": "CONDITIONALS_BOUNDARY", "status": "LIVED",
                                "description": ">= → >"}]},
      "grades": {"crap": 4, "mutation": 4, "coverage": 6, "combined": 4}
    },
    "…/internal/pricing": {
      "crap_max": 30.0, "crap_avg": 9.9, "coverage": {"percent": 72.0},
      "mutation": {"killed": 37, "survived": 12, "not_covered": 11, "score": 76},
      "grades": {"crap": 2, "mutation": 1, "coverage": 6, "combined": 1}, "churn": 24,
      "worst": "…pricing.BestRule"
    }
  }
}
```

### 5.3 Mutation cache: `.qtldr/mutation/<pkg path, "/" → "__">.json` (committed)
Per function: `{"body_hash", "engine", "engine_version", "ran_at", "counts", "mutants"}`. A function whose current `body_hash` matches keeps its results; otherwise its results are `stale` until re-run. Committing this means a fresh clone (and any agent) sees mutation results without re-running hours of tests, which is the same reason uml-viewer commits its `.metrics/`.

### 5.4 Notes: `.qtldr/notes.json` (committed)
```json
[{"id": "n_01J…", "target": "…pricing.applyTiered", "line_offset": 14, "line_text": "switch best.Kind {",
  "author": "user", "text": "Split by Kind into applyPercent / applyFixed / applyCapped.",
  "created": "2026-09-26T09:12:00Z", "resolved": false}]
```
`author` is `user` or `agent:<client name>`. Anchoring: function ID + offset from the function's first line + the line's trimmed text. If the text at that offset no longer matches, search the function for the same text; if it is not found, mark the note `outdated` (still shown, greyed). Notes on a package or type have no line.

### 5.5 Focus: `.qtldr/focus.json` (gitignored)
Written by the UI on selection (debounced 300 ms) and on "Send to agent":
```json
{"id": "…pricing.applyTiered", "level": "function", "selected_line": 29,
 "message": "optional text the user typed", "sent": true, "at": "2026-09-28T10:04:00Z"}
```

### 5.6 What goes in git
`init` adds to `.gitignore`: `.qtldr/snapshot.json`, `.qtldr/focus.json`, `.qtldr/cache/`, `.qtldr/logs/`. Committed: `.qtldr.toml`, `.qtldr/mutation/`, `.qtldr/notes.json`, `.qtldr/themes/`.

---

## 6. Language providers

### 6.1 Interface (`internal/lang`)
```go
type Provider interface {
    Name() string
    Detect(root string) bool
    // Nodes, edges, cc, cognitive, loc, body_hash, purity facts.
    Scan(ctx context.Context, root string, cfg config.Project) (model.Graph, error)
    CoverageRun(ctx context.Context, root string, pkgs []string, cfg config.Coverage) (Profile, error)
    MapCoverage(g model.Graph, p Profile) map[model.ID]model.Coverage // pure
    Source(ctx context.Context, root, file string, from, to int) (string, error)
}
```
Mutation is separate (`internal/mutate.Mutator`) because engines are per language and optional.

### 6.2 Go provider (`internal/lang/golang`)

**Loading.** `golang.org/x/tools/go/packages` with the minimal mode set that gives syntax, types, type info, imports and module info; `Tests: false`. Skip files whose header matches `^// Code generated .* DO NOT EDIT\.$`.

**Nodes.** Packages in the module; top-level funcs and methods; named types (struct, interface, other). External modules collapse to their module path (from `Package.Module`). Stdlib hidden unless `show_stdlib`.

**Edges.**
- `imports`: package → package or external module.
- `calls`: for each `*ast.CallExpr`, resolve the callee through `types.Info.Uses` to a `*types.Func`. Static calls to module functions give `calls` edges. Calls through an interface give an edge to the interface method with kind `calls_dynamic`. (M5: resolve dynamic calls with `golang.org/x/tools/go/callgraph/vta`, after measuring its cost on a large repo.)
- `implements`: for each named type T and interface I in the module, `types.Implements(T, I) || types.Implements(types.NewPointer(T), I)`.

**Cyclomatic complexity (CC)**, matching crap4go: 1 + one for each `if`, `for`, `range`, each non-default `case` of a `switch` or type switch, each non-default `select` case, each `&&`, each `||`. Function literals count toward the enclosing function. **Verify at build time:** run crap4go on `testdata/ledger` and add a test that our CC equals crap4go's for every function; record any intentional difference in `docs/decisions.md`.

**Cognitive complexity.** Use `github.com/uudashr/gocognit` as a library if its API fits (verify), else implement the standard rules (nesting increments, sequences of `&&`/`||`, `goto` and labeled `break`/`continue`, recursion). Cross-check against the gocognit CLI on the fixture.

**Body hash.** SHA-256 of the `go/printer` output of the `*ast.FuncDecl` with comments removed, so formatting and doc edits do not invalidate coverage or mutation results.

**Purity (λ).** A function is *effectful* if any of these hold:
1. a parameter or result type is or contains `context.Context`, a type from `io`, `os`, `net`, `net/http`, `database/sql`, `*log.Logger`, or a channel;
2. it reads or writes a package-level `var` (constants are fine);
3. it contains a `go` statement, a channel send or receive, or `select`;
4. it calls a known-effectful stdlib function. Keep this as a table: `os.*`, `io` readers/writers, `net/*`, `time.Now`, `time.Sleep`, `time.Since`, `math/rand.*`, `crypto/rand.*`, `log.*`, `fmt.Print*`/`Fprint*`/`Scan*` (not `Sprint*`/`Errorf`), `os/exec.*`, `syscall.*`;
5. it calls an effectful module function (propagate to a fixed point over `calls` edges; `calls_dynamic` counts as effectful unless every implementation in the module is pure).

Record the reasons (`effects: ["calls os.ReadFile", "reads var defaultRules"]`). A package is pure iff all its functions are. `[purity].allow` overrides. Tooltips call this "pure (heuristic)".

**Churn.** v1: per file, one `git log --since="<window> months ago" --format= --name-only -- <paths>` pass, counted. Functions show their file's churn with `churn_scope: "file"`, labeled "file churn" in the UI. M5: per function, from one `git log -p -U0` pass intersecting hunks with each function's range (approximate; labeled as such).

### 6.3 External provider (M5)
Any executable that reads `{"root": "...", "files": [...]}` on stdin and prints a `model.Graph` JSON (the snapshot's `nodes`, `edges` and `metrics` shapes) on stdout, exit 0. Documented in `docs/provider-contract.md` with an example and a validator (`qtldr provider test <cmd>`) that reports contract errors with JSON paths, as the Python plan's `adapter test` did. Until a provider passes, that language is shown as "not measured".

---

## 7. Metrics

### 7.1 Coverage per function
Run `[coverage].command`. With `coverpkg = "module"` add `-coverpkg=./...`. Parse the profile (`file:startLine.startCol,endLine.endCol numStmt count`), merging duplicate blocks by summing counts. For each function, take the blocks inside `[line, end_line]`: `percent = covered_stmts / total_stmts × 100`. A function with zero statements has coverage `null` (not 0).

Line states for the source view: `covered` if every block overlapping the line has count > 0, `uncovered` if every overlapping block has count 0, `partial` if mixed, none if no block overlaps.

Coverage is `stale` for a function whose `body_hash` differs from the hash recorded at the last coverage run.

If the tests of a package fail, coverage for that package is "not measured" and the failure output is written to `.qtldr/logs/`; other packages continue.

### 7.2 CRAP
`CRAP = CC² × (1 − cov)³ + CC`, with cov in [0, 1]. Coverage `null` because the function has no statements → cov = 1. Coverage never run → CRAP is `null` (shown "—", graded as missing).

### 7.3 Mutation (Gremlins adapter)
- **Run:** `gremlins unleash -o <json> [--diff <ref>] <package pattern>`. **Verify at build time** the install command, flags, JSON schema and exit codes of the installed version. From the docs: the JSON has `files[].file_name` and `files[].mutations[]` with `line`, `column`, `type`, `status`; statuses include `KILLED`, `LIVED`, `NOT COVERED`, `TIMED OUT`, `NOT VIABLE`, `SKIPPED`, `RUNNABLE`.
- **Pre-flight (mandatory):** run `go test` on each target package first. If it fails, skip mutation for that package and report "tests fail before mutation; results would be meaningless". Mutation tools can report every mutant as killed when the test command cannot run at all; the pre-flight prevents a false 100%.
- **Attribution:** each mutant belongs to the function whose `[line, end_line]` contains it.
- **Score** = `killed / (killed + lived) × 100`; `null` if the denominator is 0. `NOT COVERED` is counted separately. `TIMED OUT` is shown but not counted as survived. `NOT VIABLE` and `SKIPPED` are ignored.
- **Description:** Gremlins reports a mutation *type*, not text. Build `"orig → repl"` from the type plus the token at `line:column`, using a table (e.g. `CONDITIONALS_BOUNDARY`: `<`→`<=`, `>=`→`>`; `CONDITIONALS_NEGATION`: `==`→`!=`; `ARITHMETIC_BASE`: `+`→`-`; `INCREMENT_DECREMENT`; `INVERT_NEGATIVES`; `INVERT_LOGICAL`: `&&`→`||`). Verify the full type list at build time; unknown types show the raw type name.
- **Incremental:** only re-run packages that contain functions whose `body_hash` changed since their cached result (or the `--func`/`--pkg` scope). Unchanged functions keep cached results.
- **`max_functions`:** when a run would cover more, run the packages of the highest-CRAP functions first and list the rest as skipped.
- **Alternative engine** to evaluate at M3: `go-mutesting` v2 (jonbaldie fork), which advertises per-test filtering and an agent-oriented JSON logger. Keep `Mutator` engine-neutral.

### 7.4 Grades and roll-up (exact)
```
gradeCRAP(c):   null→1; ≤5→10; ≤8→8; ≤12→6; ≤20→4; ≤30→2; else 1
gradeMut(m%):   null→1; ≥90→10; ≥80→8; ≥70→6; ≥55→4; ≥40→2; else 1
gradeCov(c%):   null→1; ≥90→10; ≥80→8; ≥70→6; ≥50→4; ≥30→2; else 1
combined = round((gradeCRAP + gradeMut) / 2)       // half rounds up
```
- A function with **zero mutation sites** is excluded from mutation grading; its combined grade = gradeCRAP. A function **with sites but killed + lived = 0** grades 1 on mutation: missing data counts as worst.
- A **package's** grade per metric is the **worst** grade among its functions: a parent shows its worst child. Also store `crap_max`, `crap_avg`, summed mutant counts, statement-weighted coverage and the `worst` function for display.
- The module level uses the same rule over packages.
- Color: the theme's 5-step scale `[red, orange, yellow, mix(yellow, green, 0.55), green]` for grades `1–2, 3–4, 5–6, 7–8, 9–10`.

---

## 8. `qtldr check` (the pass/fail report)

**Scope:** `--changed` (default) = functions that are new or whose line range intersects a `git diff -U0 <base_ref>` hunk, plus untracked files. `--all` = everything. Explicit IDs or files. `--files-from-stdin` = the Claude Code hook JSON (`tool_input.file_path`, plus each `tool_input.edits[].file_path` if present).

**Breaches** (in-scope functions only; missing data never breaches, it is listed under "Not measured"):

| Kind | Breach when | Hint |
|---|---|---|
| crap | `crap > crap_max` | "add tests for the uncovered branches, or split the function" |
| cognitive | `cognitive > cognitive_max` | "extract the nested block or invert conditions to flatten" |
| coverage | `percent < coverage_min` (100 in `critical_paths`) | "add tests for lines a–b" (up to 10 ranges) |
| mutation | `score < mutation_min` | "each survivor is one missing assertion" (list up to 5) |

`--fast`: no test run. Checks `cognitive`, and CRAP only for unchanged functions with cached coverage.

**Output:** Markdown ≤ 60 lines (header, breaches, changed functions table, not measured, notes; truncate tables, never overflow) or `--json`. Exit 0 / 1 / 2. With `--files-from-stdin`, runtime errors exit 0 (logged) so a broken tool never blocks the agent; breaches still exit 1.

```
## qtldr check · 3 changed functions · base main
### Breaches (2)
- [crap] pricing.applyTiered — 14.0 (limit 8) — add tests for the uncovered branches, or split the function
- [mutation] pricing.applyTiered — 56% (limit 70) — each survivor is one missing assertion
  - tier.go:29 >= → >   · tier.go:22 ErrNoTiers → nil   · tier.go:47 money.Round(price) → price
### Changed
| function | CRAP | cov | mut | cognitive |
### Not measured
- pricing.Discount — mutation not run since last change (run: qtldr mutate --func pricing.Discount)
```

---

## 9. Agent integration

### 9.1 MCP tools (`qtldr mcp`, stdio)
All return JSON. Every metric object includes a `glossary` map of the terms it uses (short definition + target), so an agent reading `crap: 14` also reads what CRAP means and the limit.

| Tool | Input | Returns |
|---|---|---|
| `get_overview` | — | module, packages with grades, top 10 risks, data freshness |
| `get_focus` | — | what the human is looking at or sent (focus.json), resolved to full node detail + message |
| `get_node` | `id` | node, metrics, callers, callees, uncovered line ranges, survivors with descriptions, notes |
| `get_source` | `id` | annotated source: each line with coverage state, mutants, notes |
| `list_worst` | `metric`, `n`, `scope?` | ranked nodes |
| `explain` | `term` | glossary entry |
| `add_note` | `id`, `line?`, `text` | the note (author `agent:<client name>`) |
| `refresh` | `scope`, `coverage?`, `mutation?` | new metrics for the scope (runs analysis) |
| `check` | `scope?` | same as `qtldr check --json` |

Verify the go-sdk tool registration API at build time (typed handlers with JSON-schema struct tags).

### 9.2 "Send to agent"
1. Always: write `focus.json` with `sent: true` and the optional message; the UI shows "Sent. Ask your agent to fix what you're looking at."
2. Always: "Copy fix prompt" puts a self-contained prompt on the clipboard: ID, file:line, scores vs targets, survivors, uncovered ranges, and "verify with `qtldr check <id>`".
3. Optional: if `[agent].tmux_target` is set, also `tmux send-keys -t <target> -l "<prompt>"` then `Enter` (uml-viewer style). Off by default.

Do not rely on MCP server-initiated notifications reaching the agent; the agent pulls with `get_focus`.

### 9.3 CLAUDE.md snippet printed by `hook install`
```
- This repo uses qtldr. Before changing a function, run `qtldr show <id>` (or MCP get_node) for its CRAP, coverage and surviving mutants.
- When the user says "fix what I'm looking at", call get_focus first.
- After editing, run `qtldr check --changed`. Targets: CRAP ≤ 8, coverage ≥ 80%, mutation ≥ 70%.
```

---

## 10. Hooks

- `--claude`: merge into `.claude/settings.json` (never duplicate an identical entry, never remove others): `PostToolUse`, matcher `Write|Edit|MultiEdit`, command `qtldr check --fast --quiet --files-from-stdin`, timeout 30. **Verify** the current Claude Code hook JSON shape at build time. Print the §9.3 snippet.
- `--git`: `.git/hooks/pre-push` with marker `# qtldr-hook`, running `qtldr check --changed`. Refuse to overwrite a hook without the marker.

---

## 11. Web UI and server

The UI is specified in **docs/UI.md**, derived from the approved mockup in `docs/mockup/`.

### 11.1 Endpoints

| Endpoint | Purpose |
|---|---|
| `GET /` | embedded app |
| `GET /api/snapshot` | full snapshot |
| `GET /api/node?id=` | node detail (same as `show`) |
| `GET /api/source?id=` | annotated source lines (same as MCP `get_source`) |
| `GET /api/glossary`, `GET /api/themes` | terms; built-in + custom themes |
| `GET /api/events` | SSE: `snapshot`, `stale` (ids), `progress` (running coverage/mutation), `toast` |
| `POST /api/focus` | write focus (token) |
| `POST /api/open` | open id or file:line in the editor (token) |
| `POST /api/notes`, `PATCH /api/notes/:id` | notes (token) |
| `POST /api/refresh` | run analyze for a scope; progress over SSE (token) |

### 11.2 Security and watching
- A random token per server start is injected into the served HTML; POSTs must send it in a header. Reject requests whose `Host` or `Origin` is not the bound address.
- `--watch`: `fsnotify` is not recursive, so add every package directory; debounce 300 ms; re-scan the changed packages' structure and complexity; mark coverage and mutation of changed functions `stale`; push over SSE. Tests are not re-run automatically.

### 11.3 Themes
`web/src/themes.json` (shipped in this starter) defines 9 built-in themes, each with UI colors, the grade colors (`red`, `orange`, `yellow`, `green`), syntax token colors, and the matching Shiki theme ID. Custom themes use the same schema. One theme drives the UI, the map colors and the code highlighting. **Verify** the Shiki theme IDs at build time; if a Shiki theme is missing, build a Shiki theme object from the token colors in themes.json.

### 11.4 Editor presets (verify every flag at build time)
`vscode` → `code -g {file}:{line}:{col}` · `cursor` → `cursor -g {file}:{line}` · `zed` → `zed {file}:{line}` · `goland` → `goland --line {line} {file}` · `nvim-remote` → `nvim --server {socket} --remote-send "<C-\\><C-N>:e +{line} {file}<CR>"` · `vim-tmux` → `{terminal} vim +{line} {file}` · `custom` → `[editor].command`. Commands run from the module root with arguments passed as an argv list, never through a shell with user-controlled strings.

---

## 12. Tests

- **Fixture module `testdata/ledger`**, mirroring the mockup: `cmd/ledgerd`, `internal/httpapi` (effectful, uses chi), `internal/store` (effectful), `internal/order` (pure), `internal/pricing` (pure: `Apply`, `BestRule` with no tests, `Discount`, `validate`, `applyTiered` with CC 11 and deliberately weak tests, `applyFlat`), `internal/money` (pure, well tested). It must build and its tests must pass. It has its own `go.mod` so it does not pull chi into qtldr's module.
- Golden test: snapshot of the fixture (IDs, edges, CC, cognitive, purity) against `testdata/golden/snapshot.json` (`-update` flag to regenerate).
- CC parity test against crap4go output captured at build time.
- Coverage parser unit tests with hand-written profiles (merging, partial lines, zero-statement functions).
- Mutation adapter tests against a **real** Gremlins JSON captured by running Gremlins on the fixture (saved in `testdata/gremlins/`), never invented. Pre-flight test with a package whose tests fail.
- Grades and roll-up: table tests for every threshold boundary, "missing counts as worst", "zero sites excluded", "parent = worst child".
- Check: each breach kind, scope from hunks, the 60-line cap, hook stdin parsing (with a captured payload).
- MCP: in-process client calls every tool against the fixture.
- Server: token enforcement, Host/Origin check, SSE smoke test.
- Web: Vitest for layout helpers and theme resolution; one Playwright smoke test (open → drill to `applyTiered` → a survivor is shown), optional in CI.
- **Dogfood:** CI runs `qtldr check --all` on qtldr itself, with CRAP ≤ 8 enforced for `internal/model` and `internal/metrics`.
- CI: ubuntu + macOS, the two latest Go versions, Node LTS for `web/`, and a check that `web/dist` matches a fresh build.

---

## 13. Milestones

**M0: skeleton and structure.** Name decided (§0). Repo layout, config, `init`, `doctor`, Go provider scan (packages, funcs, types, imports, static calls), CC, cognitive, body hash, snapshot write, `show`, `worst --metric cognitive`. Fixture module, golden test, crap4go parity test.
*Accept:* `qtldr analyze` on the fixture reproduces the golden snapshot; CC matches crap4go on every fixture function.

**M1: coverage, CRAP, check.** Coverage run and mapping, CRAP, grades and roll-up, purity, file churn, `check` (all modes), hooks.
*Accept:* on the fixture, `BestRule` has CRAP 30.0 and `applyTiered` about 14; `check --changed` after editing `applyTiered` exits 1 naming the breach; `--files-from-stdin` works with a captured Claude Code hook payload.

**M2: web UI.** `serve`, the three levels and the source view, inspector, glossary tooltips, all 9 themes plus custom themes, color-by switch, open in editor, notes (read and add), focus and copy prompt, `--watch` with stale markers.
*Accept:* the UI matches docs/UI.md on the fixture; switching themes recolors UI, map and code; "Open in" launches the configured editor at the right line.

**M3: mutation.** Gremlins adapter, pre-flight, attribution, descriptions, cache and incremental runs, `mutate`, survivors in the UI and `show`.
*Accept:* a second `mutate` with no code change re-runs nothing; changing `applyTiered` re-runs only its package; failing tests block mutation with a clear message.

**M4: agents.** MCP server with all tools, "Send to agent" (plus optional tmux), `add_note` from agents shown in the UI.
*Accept:* in Claude Code: select `applyTiered` in the UI → "fix what I'm looking at" → the agent calls `get_focus` and receives scores, survivors and uncovered lines.

**M5: breadth.** `implements` edges and type views, per-function churn, VTA dynamic calls (if affordable), external provider contract and validator, directory grouping for large modules, `go.work` support, releases (decide goreleaser vs `go install` only).

---

## 14. Non-goals (v1)
No hosted service, accounts or uploads. No auto-fix by qtldr (agents do the fixing). Metrics are limited to CC, cognitive, coverage, CRAP, mutation, churn and purity: no maintainability index, no letter grades, no debt minutes. No languages other than Go until M5's external provider.

---

## 15. Edge cases

| Situation | Behavior |
|---|---|
| Not a git repo | Everything works except `--changed`, churn and base-ref scope (a note says so) |
| A package fails to type-check | Shown with an error badge; other packages are still analyzed |
| Tests fail during the coverage run | That package's coverage is "not measured", output in the log; others continue |
| Tests fail before mutation | Mutation skipped for that package with the reason |
| Function changed since last coverage/mutation | Values shown with a "stale" marker; grades still computed but flagged |
| Generated files | Always excluded |
| Package with > 40 functions | Package view shows the 40 riskiest plus a "N more" card that expands |
| > 25 packages at one level | Group by directory (`cmd/`, `internal/…`) as an extra drill level |
| Same function name in two files (build tags) | ID gets a `#<file>` suffix; warning |
| Editor command missing | Toast with the exact command that failed and a pointer to `[editor]` |
| Windows | CLI and server work; `vim-tmux` and `nvim-remote` may not; `doctor` says so |

---

## 16. Verify-at-build-time checklist (answers go in docs/decisions.md)
1. Current stable Go version; minimum supported.
2. Minimal `go/packages` load mode.
3. crap4go CC rules vs ours (parity test).
4. gocognit library API (or own implementation).
5. Gremlins: install command, `unleash` flags (`-o`, `--diff`, package patterns), JSON schema, full mutation type list, exit codes.
6. go-sdk: server creation, tool registration, stdio transport.
7. Shiki: version, bundled theme IDs for the 9 themes in themes.json, lazy Go grammar loading.
8. `@xyflow/react` + `elkjs`: measuring node sizes before layout; the group/sub-flow API for the level box.
9. Editor CLI flags for every preset.
10. Claude Code hook JSON (PostToolUse payload, settings.json shape).
11. fsnotify recursion and platform limits.

## 17. References
- CRAP formula and Go CC rules: github.com/unclebob/crap4go
- Drill-down and grading inspiration: github.com/unclebob/uml-viewer
- Gremlins JSON output and statuses: gremlins.dev (unleash command docs) and github.com/go-gremlins/gremlins releases
- Official MCP Go SDK: github.com/modelcontextprotocol/go-sdk
- Related Go CRAP scanner that can ingest a Gremlins report: github.com/padiazg/go-crap
