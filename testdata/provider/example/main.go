// Command example is a tiny qtldr language provider used by tests and
// docs/provider-contract.md: one package per directory, one function per
// file, with made-up metrics. It shows the contract, not a real analyzer.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
)

type input struct {
	Root  string   `json:"root"`
	Files []string `json:"files"`
}

func main() {
	var in input
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		fmt.Fprintln(os.Stderr, "example provider: bad input:", err)
		os.Exit(1)
	}
	nodes := []map[string]any{}
	metrics := map[string]any{}
	pkgs := map[string]bool{}
	for _, f := range in.Files {
		dir := path.Dir(f)
		pkg := "example:" + dir
		if !pkgs[pkg] {
			pkgs[pkg] = true
			nodes = append(nodes, map[string]any{"id": pkg, "kind": "package", "name": dir + " (txt)", "dir": dir})
		}
		fn := pkg + "." + strings.TrimSuffix(path.Base(f), path.Ext(f))
		nodes = append(nodes, map[string]any{"id": fn, "kind": "func", "name": path.Base(f), "parent": pkg,
			"file": f, "line": 1, "end_line": 3, "pure": true})
		metrics[fn] = map[string]any{"cc": 2, "coverage": map[string]any{"stmts": 4, "covered": 3, "percent": 75, "stale": false}}
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"nodes": nodes, "edges": []any{}, "metrics": metrics})
}
