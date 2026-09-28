package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/focus"
)

type testServer struct {
	*Server
	url  string
	root string
}

// copyFixture copies testdata/ledger (without state) into a temp dir.
func copyFixture(t *testing.T) string {
	t.Helper()
	src, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "ledger"))
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() && (d.Name() == ".qtldr" || d.Name() == "target") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
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
	return dst
}

func start(t *testing.T, edit func(*config.Config)) testServer {
	t.Helper()
	root := copyFixture(t)
	cfg := config.Default()
	if edit != nil {
		edit(&cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	assets := fstest.MapFS{
		"index.html":    {Data: []byte("<html><head><title>qtldr</title></head><body></body></html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	s, err := New(ctx, Options{Root: root, Config: cfg, Assets: assets})
	if err != nil {
		t.Fatal(err)
	}
	ln, url, err := s.Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve(ctx, ln) }()
	return testServer{Server: s, url: strings.TrimSuffix(url, "/"), root: root}
}

func (ts testServer) do(t *testing.T, method, path, body string, header map[string]string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, ts.url+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (ts testServer) auth() map[string]string { return map[string]string{TokenHeader: ts.Token()} }

func TestGuard(t *testing.T) {
	ts := start(t, nil)
	cases := []struct {
		name   string
		method string
		path   string
		header map[string]string
		want   int
	}{
		{"index", "GET", "/", nil, 200},
		{"asset", "GET", "/assets/app.js", nil, 200},
		{"api read", "GET", "/api/snapshot", nil, 200},
		{"localhost host", "GET", "/api/snapshot", map[string]string{"Host": strings.Replace(strings.TrimPrefix(ts.url, "http://"), "127.0.0.1", "localhost", 1)}, 200},
		{"rebinding host", "GET", "/api/snapshot", map[string]string{"Host": "evil.example:80"}, 403},
		{"foreign origin", "GET", "/api/snapshot", map[string]string{"Origin": "http://evil.example"}, 403},
		{"same origin", "GET", "/api/snapshot", map[string]string{"Origin": ts.url}, 200},
		{"post without token", "POST", "/api/focus", nil, 403},
		{"post with wrong token", "POST", "/api/focus", map[string]string{TokenHeader: "nope"}, 403},
		{"unknown api", "GET", "/api/nope", nil, 404},
	}
	for _, c := range cases {
		body := ""
		if c.method == "POST" {
			body = `{"id":"applyTiered","level":"function"}`
		}
		if got, b := ts.do(t, c.method, c.path, body, c.header); got != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, got, c.want, b)
		}
	}
	_, index := ts.do(t, "GET", "/", "", nil)
	if !strings.Contains(index, `<meta name="qtldr-token" content="`+ts.Token()+`"></head>`) {
		t.Errorf("token not injected: %s", index)
	}
}

func TestReadEndpoints(t *testing.T) {
	ts := start(t, nil)
	code, body := ts.do(t, "GET", "/api/node?id=applyTiered", "", nil)
	if code != 200 || !strings.Contains(body, `"signature":"func applyTiered(`) || !strings.Contains(body, `"callers":["github.com/acme/ledger/internal/pricing.price"]`) {
		t.Fatalf("node: %d %s", code, body)
	}
	code, body = ts.do(t, "GET", "/api/source?id=applyTiered", "", nil)
	var src struct {
		From, To int
		Lines    []struct{ N int }
	}
	if err := json.Unmarshal([]byte(body), &src); code != 200 || err != nil || src.From != 18 || src.To != 48 || len(src.Lines) != 31 {
		t.Fatalf("source: %d %s", code, body)
	}
	if code, _ := ts.do(t, "GET", "/api/source?id=internal/pricing", "", nil); code != 400 {
		t.Errorf("source of a package: %d", code)
	}
	if code, body := ts.do(t, "GET", "/api/prompt?id=applyTiered&message=hi", "", nil); code != 200 || !strings.Contains(body, "Verify with: qtldr check pricing.applyTiered") {
		t.Errorf("prompt: %d %s", code, body)
	}
	if code, body := ts.do(t, "GET", "/api/glossary", "", nil); code != 200 || !strings.Contains(body, `"crap_avg"`) {
		t.Errorf("glossary: %d", code)
	}
	if code, body := ts.do(t, "GET", "/api/themes", "", nil); code != 200 || strings.Count(body, `"shiki"`) != 9 {
		t.Errorf("themes: %d", code)
	}
	if code, body := ts.do(t, "GET", "/api/config", "", nil); code != 200 || !strings.Contains(body, `"theme":"gruvbox-dark"`) {
		t.Errorf("config: %d %s", code, body)
	}
	if code, _ := ts.do(t, "GET", "/api/node?id=nope", "", nil); code != 404 {
		t.Errorf("unknown node: %d", code)
	}
}

func TestNotesFocusThemeOpen(t *testing.T) {
	ts := start(t, func(c *config.Config) { c.Editor = config.Editor{Preset: "custom", Command: "true {file}:{line}"} })
	events := ts.subscribe(t)

	code, body := ts.do(t, "POST", "/api/notes", `{"target":"applyTiered","line":34,"text":"Split by Kind."}`, ts.auth())
	if code != 201 || !strings.Contains(body, `"line_text":"switch best.Kind {"`) {
		t.Fatalf("add note: %d %s", code, body)
	}
	waitEvent(t, events, "notes")
	var note struct{ ID string }
	_ = json.Unmarshal([]byte(body), &note)
	_, src := ts.do(t, "GET", "/api/source?id=applyTiered", "", nil)
	if !strings.Contains(src, `"title":"Note from you","why":"Split by Kind."`) {
		t.Errorf("note not in source: %s", src)
	}
	if code, _ := ts.do(t, "PATCH", "/api/notes/"+note.ID, `{"resolved":true}`, ts.auth()); code != 200 {
		t.Errorf("resolve: %d", code)
	}
	if code, _ := ts.do(t, "POST", "/api/notes", `{"target":"applyTiered","line":99,"text":"x"}`, ts.auth()); code != 400 {
		t.Errorf("line outside function: %d", code)
	}

	if code, body := ts.do(t, "POST", "/api/focus", `{"id":"applyTiered","level":"function","selected_line":29,"message":"fix","sent":true}`, ts.auth()); code != 200 {
		t.Fatalf("focus: %d %s", code, body)
	}
	f, err := focus.Read(ts.root)
	if err != nil || f.ID != "github.com/acme/ledger/internal/pricing.applyTiered" || !f.Sent || f.SelectedLine != 29 {
		t.Errorf("focus.json: %+v %v", f, err)
	}

	if code, body := ts.do(t, "POST", "/api/theme", `{"theme":"nord"}`, ts.auth()); code != 200 {
		t.Fatalf("theme: %d %s", code, body)
	}
	toml, _ := os.ReadFile(filepath.Join(ts.root, config.FileName))
	if !strings.Contains(string(toml), `theme = "nord"`) {
		t.Errorf(".qtldr.toml: %s", toml)
	}
	if code, _ := ts.do(t, "POST", "/api/theme", `{"theme":"nope"}`, ts.auth()); code != 400 {
		t.Errorf("unknown theme: %d", code)
	}

	code, body = ts.do(t, "POST", "/api/open", `{"id":"applyTiered","line":29}`, ts.auth())
	if code != 200 || !strings.Contains(body, `internal/pricing/tier.go:29"]`) {
		t.Errorf("open: %d %s", code, body)
	}
	ts.mu.Lock()
	ts.cfg.Editor.Command = "false {file}"
	ts.mu.Unlock()
	if code, body := ts.do(t, "POST", "/api/open", `{"id":"applyTiered"}`, ts.auth()); code != 502 || !strings.Contains(body, "`false ") {
		t.Errorf("failing editor must name the command: %d %s", code, body)
	}
}

func TestRefreshAndWatch(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	ts := start(t, nil)
	events := ts.subscribe(t)
	if code, body := ts.do(t, "POST", "/api/refresh", `{"id":"internal/pricing","coverage":true}`, ts.auth()); code != 202 {
		t.Fatalf("refresh: %d %s", code, body)
	}
	waitEvent(t, events, "snapshot")
	if !strings.Contains(waitEvent(t, events, "toast"), "Coverage finished") {
		t.Error("toast")
	}
	if m := ts.snapshot().Metrics["github.com/acme/ledger/internal/pricing.BestRule"]; m.CRAP == nil || *m.CRAP != 30 {
		t.Errorf("BestRule after refresh: %+v", m.CRAP)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ts.Watch(ctx) }()
	time.Sleep(300 * time.Millisecond) // let the watcher register
	tier := filepath.Join(ts.root, "internal", "pricing", "tier.go")
	b, _ := os.ReadFile(tier)
	if err := os.WriteFile(tier, []byte(strings.Replace(string(b), "qty >= t.Floor", "qty > t.Floor", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := waitEvent(t, events, "stale"); !strings.Contains(got, "pricing.applyTiered") {
		t.Errorf("stale event: %s", got)
	}
}

// subscribe opens /api/events and returns a channel of "name data" lines.
func (ts testServer) subscribe(t *testing.T) <-chan string {
	t.Helper()
	resp, err := http.Get(ts.url + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	out := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		name := ""
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				out <- name + " " + strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	time.Sleep(50 * time.Millisecond)
	return out
}

func waitEvent(t *testing.T, events <-chan string, name string) string {
	t.Helper()
	deadline := time.After(60 * time.Second)
	for {
		select {
		case e := <-events:
			if strings.HasPrefix(e, name+" ") {
				return e
			}
		case <-deadline:
			t.Fatalf("no %q event", name)
			return ""
		}
	}
}

func TestListNotesAndAgent(t *testing.T) {
	ts := start(t, nil)
	events := ts.subscribe(t)
	if _, err := ts.AddNote("applyTiered", 0, "whole function", "agent:test"); err != nil {
		t.Fatal(err)
	}
	if code, body := ts.do(t, "GET", "/api/notes?id=github.com/acme/ledger/internal/pricing.applyTiered", "", nil); code != 200 || !strings.Contains(body, "whole function") {
		t.Errorf("list for id: %d %s", code, body)
	}
	if code, body := ts.do(t, "GET", "/api/notes", "", nil); code != 200 || !strings.Contains(body, `"author":"agent:test"`) {
		t.Errorf("list all: %d %s", code, body)
	}
	if err := focus.WriteAgentPing(ts.root, "claude-code", ts.opt.Now()); err != nil {
		t.Fatal(err)
	}
	ts.checkAgent()
	if got := waitEvent(t, events, "agent"); !strings.Contains(got, `"connected":true`) || !strings.Contains(got, "claude-code") {
		t.Errorf("agent event: %s", got)
	}
	ts.checkAgent() // unchanged: no second event
	if code, body := ts.do(t, "GET", "/api/config", "", nil); !strings.Contains(body, `"connected":true`) {
		t.Errorf("config agent: %d %s", code, body)
	}
}

func TestOpenTarget(t *testing.T) {
	ts := start(t, nil)
	ed, target, err := ts.openTarget(openRequest{ID: "applyTiered", Editor: "zed"})
	if err != nil || ed.Preset != "zed" || target.Line != 20 || target.File != "internal/pricing/tier.go" {
		t.Fatalf("%+v %+v %v", ed, target, err)
	}
	if _, _, err := ts.openTarget(openRequest{ID: "internal/pricing"}); err == nil {
		t.Error("a package has no file")
	}
}

func TestRelevant(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "newpkg")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	var added []string
	add := func(p string) error { added = append(added, p); return nil }
	cases := []struct {
		ev   fsnotify.Event
		want bool
	}{
		{fsnotify.Event{Name: "a/tier.go", Op: fsnotify.Write}, true},
		{fsnotify.Event{Name: "a/tier_test.go", Op: fsnotify.Write}, false},
		{fsnotify.Event{Name: "a/tier.go", Op: fsnotify.Chmod}, false},
		{fsnotify.Event{Name: "a/notes.md", Op: fsnotify.Write}, false},
		{fsnotify.Event{Name: sub, Op: fsnotify.Create}, false},
	}
	for _, c := range cases {
		if got := relevant(c.ev, add); got != c.want {
			t.Errorf("%v: %v", c.ev, got)
		}
	}
	if len(added) != 1 || added[0] != sub {
		t.Errorf("new directory not watched: %v", added)
	}
}
