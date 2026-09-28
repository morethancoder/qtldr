package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/morethancoder/qtldr/internal/argv"
	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/model"
)

// Input is what a provider reads on stdin.
type Input struct {
	Root  string   `json:"root"`
	Files []string `json:"files"`
}

// Timeout bounds one provider run.
const Timeout = 5 * time.Minute

var skipDirs = []string{".git", ".qtldr", "node_modules", "vendor", "testdata"}

// Files lists files under root with one of exts, relative to root, skipping
// hidden directories, vendor, node_modules, testdata and excluded globs.
func Files(root string, exts, exclude []string, excluded func(patterns []string, file string) bool) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return skipDir(root, p, d.Name())
		}
		rel, _ := filepath.Rel(root, p)
		if rel = filepath.ToSlash(rel); slices.Contains(exts, filepath.Ext(p)) && !excluded(exclude, rel) {
			out = append(out, rel)
		}
		return nil
	})
	return out, err
}

// skipDir skips hidden directories and the ones in skipDirs (not the root).
func skipDir(root, p, name string) error {
	if p != root && (strings.HasPrefix(name, ".") || slices.Contains(skipDirs, name)) {
		return filepath.SkipDir
	}
	return nil
}

// Run executes the provider (argv, or its command split without a shell) in
// root with the input on stdin and returns its stdout.
func Run(ctx context.Context, root string, p config.Provider, in Input) ([]byte, error) {
	args, err := p.Args, error(nil)
	if len(args) == 0 {
		args, err = argv.Split(p.Command)
	}
	if err != nil {
		return nil, err
	}
	command := strings.Join(args, " ")
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	payload, _ := json.Marshal(in)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("`%s` failed: %v: %s", command, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Scan runs a configured provider over its files and validates the result.
func Scan(ctx context.Context, root string, p config.Provider, exclude []string, excluded func([]string, string) bool) (model.Graph, []Problem, error) {
	files, err := Files(root, p.Extensions, exclude, excluded)
	if err != nil {
		return model.Graph{}, nil, err
	}
	out, err := Run(ctx, root, p, Input{Root: root, Files: files})
	if err != nil {
		return model.Graph{}, nil, err
	}
	g, problems := Validate(out)
	return g, problems, nil
}

// Merge adds a provider's graph to g. Provider packages without a parent are
// put under parent (the Go module). Functions whose purity the provider did
// not report as pure are marked effectful with a reason, so qtldr never claims
// λ on its behalf. IDs that collide with existing nodes are problems.
func Merge(g *model.Graph, add model.Graph, provider string, parent model.ID) []Problem {
	have := map[model.ID]bool{}
	for _, n := range g.Nodes {
		have[n.ID] = true
	}
	var problems []Problem
	for i, n := range add.Nodes {
		if have[n.ID] {
			problems = append(problems, Problem{fmt.Sprintf("$.nodes[%d].id", i), fmt.Sprintf("%q collides with an existing node", n.ID)})
			continue
		}
		g.Nodes = append(g.Nodes, adopt(n, provider, parent))
	}
	g.Edges = append(g.Edges, add.Edges...)
	for id, m := range add.Metrics {
		g.Metrics[id] = m
	}
	g.Sort()
	return problems
}

func adopt(n model.Node, provider string, parent model.ID) model.Node {
	if n.Parent == "" && n.Kind == model.KindPackage {
		n.Parent = parent
	}
	if n.Kind == model.KindFunc && (n.Pure == nil || !*n.Pure) && len(n.Effects) == 0 {
		n.Effects = []string{"purity not known (from provider " + provider + ")"}
	}
	return n
}

// NotMeasured is the placeholder package for a provider that failed.
func NotMeasured(provider string, parent model.ID, reasons []string) model.Node {
	return model.Node{ID: model.ID("provider:" + provider), Kind: model.KindPackage, Name: provider + " (not measured)",
		Parent: parent, Dir: ".", Errors: reasons}
}
