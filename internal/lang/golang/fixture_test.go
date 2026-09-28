package golang

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/metrics"
	"github.com/morethancoder/qtldr/internal/model"
)

var update = flag.Bool("update", false, "rewrite golden files from the current output")

var (
	fixtureDir = filepath.Join("..", "..", "..", "testdata", "ledger")
	goldenPath = filepath.Join("..", "..", "..", "testdata", "golden", "snapshot.json")

	fixtureOnce  sync.Once
	fixtureGraph model.Graph
	fixtureErr   error
)

// scanFixture scans testdata/ledger once per test binary.
func scanFixture(t *testing.T) model.Graph {
	t.Helper()
	fixtureOnce.Do(func() {
		root, err := filepath.Abs(fixtureDir)
		if err != nil {
			fixtureErr = err
			return
		}
		fixtureGraph, fixtureErr = New().Scan(context.Background(), root, config.Default().Project)
	})
	if fixtureErr != nil {
		t.Fatalf("scan testdata/ledger: %v", fixtureErr)
	}
	return fixtureGraph
}

// TestGoldenSnapshot compares the scan of the fixture with
// testdata/golden/snapshot.json. Regenerate with:
//
//	go test ./internal/lang/golang -run Golden -update
func TestGoldenSnapshot(t *testing.T) {
	// Structure, complexity and purity: the scan plus metrics.Compute with no
	// coverage, mutation or churn inputs (those depend on the machine).
	got, err := json.MarshalIndent(metrics.Compute(scanFixture(t), metrics.Inputs{}), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", goldenPath)
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("%v (create it with: go test ./internal/lang/golang -run Golden -update)", err)
	}
	if line, g, w, differ := firstDiff(got, want); differ {
		t.Fatalf("snapshot differs from %s at line %d:\n got: %s\nwant: %s\nreview, then regenerate with -update", goldenPath, line, g, w)
	}
}

func firstDiff(a, b []byte) (int, string, string, bool) {
	al, bl := strings.Split(string(a), "\n"), strings.Split(string(b), "\n")
	for i := 0; i < max(len(al), len(bl)); i++ {
		x, y := at(al, i), at(bl, i)
		if x != y {
			return i + 1, x, y, true
		}
	}
	return 0, "", "", false
}

func at(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "<end of file>"
}

// TestCrap4goParity checks our CC against crap4go's report on the fixture,
// captured with `crap4go` in testdata/ledger (see docs/decisions.md #3).
func TestCrap4goParity(t *testing.T) {
	want := readCrap4go(t, filepath.Join("..", "..", "..", "testdata", "crap4go", "ledger.txt"))
	got := map[string]int{}
	g := scanFixture(t)
	for _, n := range g.Nodes {
		if n.Kind == model.KindFunc {
			got[declaredPackage(t, n.File)+" "+n.Name] = *g.Metrics[n.ID].CC
		}
	}
	if len(got) != len(want) {
		t.Errorf("we found %d functions, crap4go %d", len(got), len(want))
	}
	for key, cc := range want {
		if got[key] != cc {
			t.Errorf("%s: CC = %d, crap4go says %d", key, got[key], cc)
		}
	}
}

// readCrap4go parses the report table: Function Package CC Cov% CRAP.
func readCrap4go(t *testing.T, path string) map[string]int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cc := map[string]int{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	inTable := false
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 1 && strings.HasPrefix(f[0], "----") {
			inTable = true
			continue
		}
		if !inTable || len(f) != 5 {
			continue
		}
		n, err := strconv.Atoi(f[2])
		if err != nil {
			t.Fatalf("%s: bad CC in %q", path, sc.Text())
		}
		cc[f[1]+" "+f[0]] = n
	}
	if len(cc) == 0 {
		t.Fatalf("%s: no report rows found", path)
	}
	return cc
}

// declaredPackage returns the package clause name of a fixture file.
func declaredPackage(t *testing.T, file string) string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(fixtureDir, file), nil, parser.PackageClauseOnly)
	if err != nil {
		t.Fatal(err)
	}
	return f.Name.Name
}

// TestGocognitParity checks our cognitive complexity against
// `gocognit -json .` run in testdata/ledger (see docs/decisions.md #4).
func TestGocognitParity(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "gocognit", "ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stats []struct {
		FuncName   string
		Complexity int
		Pos        struct {
			Filename string
			Line     int
		}
	}
	if err := json.Unmarshal(b, &stats); err != nil {
		t.Fatal(err)
	}
	ours := cognitiveByPos(scanFixture(t))
	checked := 0
	for _, s := range stats {
		if strings.HasSuffix(s.Pos.Filename, "_test.go") {
			continue
		}
		key := fmt.Sprintf("%s:%d", s.Pos.Filename, s.Pos.Line)
		got, ok := ours[key]
		if !ok || got != s.Complexity {
			t.Errorf("%s %s: cognitive = %d (found %v), gocognit says %d", key, s.FuncName, got, ok, s.Complexity)
		}
		checked++
	}
	if checked != len(ours) {
		t.Errorf("gocognit reported %d functions, we found %d", checked, len(ours))
	}
}

func cognitiveByPos(g model.Graph) map[string]int {
	out := map[string]int{}
	for _, n := range g.Nodes {
		if n.Kind == model.KindFunc {
			out[fmt.Sprintf("%s:%d", n.File, n.Line)] = *g.Metrics[n.ID].Cognitive
		}
	}
	return out
}

func TestFixtureShape(t *testing.T) {
	g := scanFixture(t)
	const pricing = "github.com/acme/ledger/internal/pricing"
	checks := []struct {
		name string
		ok   bool
	}{
		{"applyTiered at tier.go:20–48", hasFunc(g, pricing+".applyTiered", "internal/pricing/tier.go", 20, 48)},
		{"method ID uses the type name", hasFunc(g, "github.com/acme/ledger/internal/store.Store.LoadRules", "internal/store/store.go", 52, 68)},
		{"chi collapsed to its module", hasNode(g, "github.com/go-chi/chi/v5", model.KindExternal)},
		{"stdlib hidden", !hasNode(g, "net/http", model.KindExternal)},
		{"static call edge", hasEdge(g, pricing+".price", pricing+".applyTiered", model.EdgeCalls)},
		{"cross-package call edge", hasEdge(g, pricing+".applyTiered", "github.com/acme/ledger/internal/money.Round", model.EdgeCalls)},
		{"interface call is dynamic", hasEdge(g, "github.com/acme/ledger/internal/httpapi.Server.handleQuote",
			"github.com/acme/ledger/internal/httpapi.RuleSource.LoadRules", model.EdgeCallsDynamic)},
		{"import edge to external module", hasEdge(g, "github.com/acme/ledger/internal/httpapi", "github.com/go-chi/chi/v5", model.EdgeImports)},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("fixture: %s", c.name)
		}
	}
}

func hasFunc(g model.Graph, id model.ID, file string, line, end int) bool {
	n, ok := g.Node(id)
	return ok && n.File == file && n.Line == line && n.EndLine == end
}

func hasNode(g model.Graph, id model.ID, kind model.Kind) bool {
	n, ok := g.Node(id)
	return ok && n.Kind == kind
}

func hasEdge(g model.Graph, from, to model.ID, kind model.EdgeKind) bool {
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.Kind == kind {
			return true
		}
	}
	return false
}

func TestWorkspace(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "workspace"))
	g, err := New().Scan(context.Background(), root, config.Default().Project)
	if err != nil {
		t.Fatal(err)
	}
	modules, dirs := 0, map[model.ID]string{}
	for _, n := range g.Nodes {
		if n.Kind == model.KindModule {
			modules++
		}
		if n.Kind == model.KindPackage {
			dirs[n.ID] = n.Dir
		}
	}
	if modules != 2 || dirs["example.com/moda/app"] != "moda/app" || dirs["example.com/modb/lib"] != "modb/lib" {
		t.Fatalf("modules %d dirs %v", modules, dirs)
	}
	if !hasEdge(g, "example.com/moda/app", "example.com/modb/lib", model.EdgeImports) ||
		!hasEdge(g, "example.com/moda/app.Run", "example.com/modb/lib.Double", model.EdgeCalls) {
		t.Errorf("cross-module edges missing: %v", g.Edges)
	}
	if n, _ := g.Node("example.com/modb/lib.Double"); n.File != "modb/lib/lib.go" {
		t.Errorf("file %q", n.File)
	}
}

func TestExpandWorkPatterns(t *testing.T) {
	got := ExpandWorkPatterns([]string{"./...", ".", "example.com/x/..."}, []string{"./moda", "modb"})
	if strings.Join(got, " ") != "./moda/... ./modb/... ./moda ./modb example.com/x/..." {
		t.Fatalf("got %v", got)
	}
}

func TestVTAEdges(t *testing.T) {
	if testing.Short() {
		t.Skip("builds SSA for the fixture")
	}
	root, _ := filepath.Abs(fixtureDir)
	cfg := config.Default().Project
	cfg.Calls = "vta"
	g, err := New().Scan(context.Background(), root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// cmd/ledgerd passes a *store.Store to httpapi.New, so VTA resolves
	// handleQuote's call through RuleSource to the concrete method.
	if !hasEdge(g, "github.com/acme/ledger/internal/httpapi.Server.handleQuote",
		"github.com/acme/ledger/internal/store.Store.LoadRules", model.EdgeCallsDynamic) {
		var dyn []string
		for _, e := range g.Edges {
			if e.Kind == model.EdgeCallsDynamic {
				dyn = append(dyn, string(e.From)+" → "+string(e.To))
			}
		}
		t.Fatalf("no resolved edge; dynamic edges: %v", dyn)
	}
}
