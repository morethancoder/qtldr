# External language providers

qtldr reads Go itself. Any other language can join the map through an
**external provider**: an executable that reads a small JSON request on stdin
and prints a graph in qtldr's snapshot shapes on stdout. qtldr validates the
output, merges it into the snapshot, and grades it like Go code. Until a
provider passes validation, its language is shown as **not measured**.

## Configure

In `.qtldr.toml`, one `[[providers]]` table per provider:

```toml
[[providers]]
name = "python"                    # shown in the UI and in messages
command = "qtldr-py --strict"      # split into argv without a shell; run from the module root
extensions = [".py"]               # files sent to the provider
```

qtldr sends every file under the root with one of the extensions, skipping
hidden directories, `vendor/`, `node_modules/`, `testdata/`, and
`[project].exclude` globs. A run times out after 5 minutes.

## Input (stdin)

```json
{"root": "/abs/path/to/module", "files": ["app/run.py", "app/util/text.py"]}
```

`files` are relative to `root`, with forward slashes.

## Output (stdout, exit 0)

A JSON object with the same `nodes`, `edges` and `metrics` shapes as
`.qtldr/snapshot.json` (PLAN.md §5.2):

```json
{
  "nodes": [
    {"id": "py:app", "kind": "package", "name": "app", "dir": "app"},
    {"id": "py:app.run", "kind": "func", "name": "run", "parent": "py:app",
     "file": "app/run.py", "line": 3, "end_line": 21, "pure": false,
     "effects": ["reads os.environ"]}
  ],
  "edges": [
    {"from": "py:app.run", "to": "py:app.util.clean", "kind": "calls"}
  ],
  "metrics": {
    "py:app.run": {
      "cc": 7, "cognitive": 9, "loc": 19,
      "coverage": {"stmts": 12, "covered": 9, "percent": 75, "stale": false,
                   "lines": {"covered": [4, 5], "uncovered": [9, 10], "partial": []}},
      "crap": 7.9,
      "mutation": {"killed": 6, "survived": 2, "not_covered": 1, "timed_out": 0,
                   "score": 75, "stale": false,
                   "mutants": [{"line": 9, "col": 12, "type": "CONDITIONALS_BOUNDARY",
                                "status": "LIVED", "description": "< → <="}]}
    }
  }
}
```

Anything written to stderr is shown when the provider exits non-zero.

### Rules the validator checks

| Path | Rule |
|---|---|
| `$` | a JSON object; `nodes` is required (`edges`, `metrics` may be omitted) |
| `$.nodes[i]` | no unknown fields (typos are errors) |
| `$.nodes[i].id` | required, unique, and must not collide with a Go node |
| `$.nodes[i].kind` | `module`, `package`, `func`, `type` or `external` |
| `$.nodes[i].name` | required |
| `$.nodes[i].parent` | when set, an existing node id |
| `$.nodes[i].file`, `.line`, `.end_line` | functions and types: `file` relative to root (no `/`, no `..`), `line ≥ 1`; functions also `end_line ≥ line` |
| `$.edges[i].kind` | `imports`, `calls`, `calls_dynamic` or `implements` |
| `$.edges[i].from`, `.to` | existing node ids |
| `$.metrics["id"]` | an existing node id; no unknown fields |
| `$.metrics["id"].cc` | ≥ 1 |
| `$.metrics["id"].coverage` | `covered ≤ stmts`, `percent` within 0–100 |
| `$.metrics["id"].mutation.score` | within 0–100 |

Prefix ids with the language (`py:`, `ts:`) so they never collide with Go
import paths.

### How qtldr uses the output

- Packages without a `parent` are placed under the Go module, so they show at
  the module level next to the Go packages.
- Provider metrics are used as given (qtldr does not recompute CRAP for them);
  grades, package roll-ups, `check`, `worst`, the web UI and the MCP tools
  treat them exactly like Go functions.
- Purity: a function is shown λ pure only when the provider says
  `"pure": true`. Otherwise it is effectful with the reason "purity not known
  (from provider …)", unless the provider lists `effects`.
- Coverage and mutation are the provider's job; `qtldr analyze --coverage` and
  `qtldr mutate` only run Go tests.

## Test a provider

```
qtldr provider test python                        # a provider from .qtldr.toml
qtldr provider test --ext .py -- qtldr-py --strict  # any command
```

It runs the provider once on the current module and prints either
`Provider python follows the contract: N nodes, M edges, K metrics.` or every
problem with its JSON path, for example:

```
Provider python broke the contract (2 problems):
  $.nodes[3].kind: must be one of module, package, func, type, external, got "class"
  $.edges[0].to: unknown node "py:app.missing"
```

Exit code 0 when it passes, 1 on contract problems, 2 when it cannot run.
During `qtldr analyze` a failing provider does not stop the run: its language
appears as a package named `<name> (not measured)` listing the problems.

## Example

`testdata/provider/example/main.go` is a complete provider in 40 lines (one
package per directory, one function per file, fixed metrics). It is used by
qtldr's tests:

```
go build -o /tmp/example-provider ./testdata/provider/example
qtldr provider test --ext .txt -- /tmp/example-provider
```
