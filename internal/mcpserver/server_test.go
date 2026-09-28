package mcpserver

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/focus"
	"github.com/morethancoder/qtldr/internal/notes"
)

var now = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

// fixtureRepo copies testdata/ledger (with its committed mutation results)
// into a git repository.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	src, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "ledger"))
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() && (d.Name() == "cache" || d.Name() == "logs" || d.Name() == "target") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if strings.HasSuffix(p, "snapshot.json") || strings.HasSuffix(p, "focus.json") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"},
		{"-c", "user.name=q", "-c", "user.email=q@example.com", "commit", "-q", "-m", "fixture"}} {
		if out, err := exec.Command("git", append([]string{"-C", dst}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dst
}

func connect(t *testing.T, root string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	srv := New(Options{Root: root, Config: config.Default(), Version: "test", Now: func() time.Time { return now }})
	st, ct := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// call runs a tool and returns its JSON text; isErr reports a tool error.
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	// The SDK escapes <, > and & in JSON text; undo that for readable asserts.
	text := strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\u0026`, "&").Replace(b.String())
	return text, res.IsError
}

func TestTools(t *testing.T) {
	if testing.Short() {
		t.Skip("scans the fixture and runs go test")
	}
	root := fixtureRepo(t)
	cs := connect(t, root)

	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range tools.Tools {
		names = append(names, tl.Name)
	}
	if strings.Join(names, ",") != "add_note,check,explain,get_focus,get_node,get_overview,get_source,list_worst,refresh" {
		t.Errorf("tools: %v", names)
	}

	// Coverage first, so CRAP and uncovered lines exist.
	if out, isErr := call(t, cs, "refresh", map[string]any{"scope": "internal/pricing", "coverage": true}); isErr || !strings.Contains(out, `"crap":30`) {
		t.Fatalf("refresh: %s", out)
	}

	cases := []struct {
		tool string
		args map[string]any
		want []string
	}{
		{"get_overview", nil, []string{`"module":"github.com/acme/ledger"`, `"riskiest_function":"pricing.BestRule"`, `"top_risks"`, `"glossary"`}},
		{"get_node", map[string]any{"id": "applyTiered"}, []string{
			`"crap_target":8`, `"coverage_percent":70.6`, `"mutation_score":63.6`, `"survived":4`,
			`"uncovered_lines":["tier.go:25–26","tier.go:37–38","tier.go:41","tier.go:43–45"]`,
			`"at":"tier.go:29","change":">= → >"`,
			`"verify":"qtldr check pricing.applyTiered"`, `"crap":{"good":`}},
		{"get_source", map[string]any{"id": "applyTiered"}, []string{`"lines":"18-48"`, `  29 C | \t\tif qty >= t.Floor`, `^ SURVIVED:`, `^ NOT COVERED: Never run by tests if qty <= 0`}},
		{"list_worst", map[string]any{"metric": "crap", "n": 2}, []string{`"metric":"crap"`, `pricing.BestRule`}},
		{"list_worst", map[string]any{"metric": "mutation", "scope": "internal/pricing"}, []string{`pricing.applyTiered`}},
		{"explain", map[string]any{"term": "not covered"}, []string{`"title":"Not-covered mutants"`}},
		{"add_note", map[string]any{"id": "applyTiered", "line": 28, "text": "tiers[1:] → tiers[0:] is equivalent"}, []string{`"author":"agent:test-agent"`, `"line_text":"for _, t := range tiers[1:] {"`}},
		{"check", map[string]any{"scope": "all", "fast": true}, []string{`"scope":"all"`, `"fast":true`}},
		{"check", map[string]any{"fast": true}, []string{`"scope":"changed"`, `"base":"main"`, `"pass":true`}},
		{"check", map[string]any{"scope": "internal/pricing/tier.go", "fast": true}, []string{`"name":"pricing.applyTiered"`}},
		{"check", map[string]any{"scope": "applyTiered"}, []string{`"kind":"crap"`, `"kind":"coverage"`, `"kind":"mutation"`}},
	}
	for _, c := range cases {
		out, isErr := call(t, cs, c.tool, c.args)
		if isErr {
			t.Errorf("%s: tool error %s", c.tool, out)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: missing %s in\n%.1500s", c.tool, w, out)
			}
		}
	}
	if out, isErr := call(t, cs, "get_node", map[string]any{"id": "nope"}); !isErr || !strings.Contains(out, "no node matches") {
		t.Errorf("unknown id: %v %s", isErr, out)
	}
	if ping, err := focus.ReadAgentPing(root); err != nil || ping.Client != "test-agent" {
		t.Errorf("agent ping %+v %v", ping, err)
	}
	all, _ := notes.Load(root)
	if len(all) != 1 || all[0].Author != "agent:test-agent" {
		t.Errorf("notes %+v", all)
	}
}

func TestGetFocus(t *testing.T) {
	if testing.Short() {
		t.Skip("scans the fixture")
	}
	root := fixtureRepo(t)
	cs := connect(t, root)
	if out, isErr := call(t, cs, "get_focus", nil); !isErr || !strings.Contains(out, "nothing is selected") {
		t.Fatalf("no focus yet: %s", out)
	}
	f := focus.Focus{ID: "github.com/acme/ledger/internal/pricing.applyTiered", Level: "function", SelectedLine: 29, Message: "kill the survivors", Sent: true, At: now}
	if err := focus.Write(root, f); err != nil {
		t.Fatal(err)
	}
	out, isErr := call(t, cs, "get_focus", nil)
	var got FocusResult
	if isErr || json.Unmarshal([]byte(out), &got) != nil {
		t.Fatalf("get_focus: %s", out)
	}
	if got.Message != "kill the survivors" || got.SelectedLine != "if qty >= t.Floor && t.Floor > best.Floor {" ||
		got.Node.Name != "applyTiered" || len(got.Node.Survivors) != 4 || !strings.Contains(got.FixPrompt, "Surviving mutants: tier.go:24") {
		t.Errorf("focus result: %+v", got)
	}
}
