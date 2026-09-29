package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/morethancoder/qtldr/internal/analysis"
	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/focus"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/mutate"
	"github.com/morethancoder/qtldr/internal/notes"
)

var testNow = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

type runFunc = func(context.Context, analysis.Options) (analysis.Result, error)

// bare is a server in an empty directory that does not listen; run replaces
// the analysis (nil: an empty snapshot).
func bare(t *testing.T, run runFunc) *Server {
	t.Helper()
	if run == nil {
		run = func(context.Context, analysis.Options) (analysis.Result, error) { return analysis.Result{}, nil }
	}
	s, err := New(context.Background(), Options{Root: t.TempDir(), Config: config.Default(), Run: run, Now: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// published drains the events published so far and returns their names.
func published(ch chan Event) []string {
	var names []string
	for {
		select {
		case e := <-ch:
			names = append(names, e.Name)
		default:
			return names
		}
	}
}

func TestNewKeepsTheGivenLog(t *testing.T) {
	var logs strings.Builder
	s, err := New(context.Background(), Options{Root: t.TempDir(), Log: &logs,
		Run: func(context.Context, analysis.Options) (analysis.Result, error) { return analysis.Result{}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	s.logf("watch %s: %v", "a", 1)
	if logs.String() != "watch a: 1\n" {
		t.Errorf("log = %q", logs.String())
	}
	bare(t, nil).logf("no log given: discarded, not a panic")
}

func TestWatchLogsDirectoriesItCannotWatch(t *testing.T) {
	ts := start(t, nil)
	var logs strings.Builder
	ts.opt.Log = &logs
	gone := filepath.Join(ts.root, "internal", "money")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- ts.Watch(ctx) }()
	<-ts.Watching()
	got := logs.String()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "watch "+gone+": ") || strings.Contains(got, "watch "+ts.root+": ") {
		t.Errorf("only the missing directory is logged: %q", got)
	}
}

func TestAgentConnectedForOneMinute(t *testing.T) {
	s := bare(t, nil)
	if st := s.agentState(); st.Connected || st.Client != "" {
		t.Errorf("no ping yet: %+v", st)
	}
	cases := []struct {
		ago  time.Duration
		want bool
	}{{0, true}, {30 * time.Second, true}, {60 * time.Second, true}, {61 * time.Second, false}}
	for _, c := range cases {
		if err := focus.WriteAgentPing(s.opt.Root, "claude-code", testNow.Add(-c.ago)); err != nil {
			t.Fatal(err)
		}
		if st := s.agentState(); st.Connected != c.want || st.Client != "claude-code" {
			t.Errorf("ping %v ago: %+v, want connected=%v", c.ago, st, c.want)
		}
	}
}

func TestCheckAgentPublishesOnlyChanges(t *testing.T) {
	s := bare(t, nil)
	ch := s.events.subscribe()
	ping := func(client string) {
		if err := focus.WriteAgentPing(s.opt.Root, client, testNow); err != nil {
			t.Fatal(err)
		}
	}
	steps := []struct {
		name, client string
		want         int
	}{
		{"connects", "claude-code", 1},
		{"unchanged", "claude-code", 0},
		{"same state, other client", "codex", 1},
	}
	for _, st := range steps {
		ping(st.client)
		s.checkAgent()
		if got := len(published(ch)); got != st.want {
			t.Errorf("%s: %d events, want %d", st.name, got, st.want)
		}
	}
	if s.agent.Client != "codex" {
		t.Errorf("agent %+v", s.agent)
	}
}

func TestConfigSaysWhetherTmuxIsSet(t *testing.T) {
	s := bare(t, nil)
	for _, target := range []string{"", "main:1.0"} {
		s.cfg.Agent.TmuxTarget = target
		rec := httptest.NewRecorder()
		s.handleConfig(rec, httptest.NewRequest("GET", "/api/config", nil))
		if want := fmt.Sprintf(`"tmux":%v`, target != ""); !strings.Contains(rec.Body.String(), want) {
			t.Errorf("target %q: missing %s in %s", target, want, rec.Body.String())
		}
	}
}

func TestPatchNoteFields(t *testing.T) {
	ts := start(t, nil)
	n, err := ts.AddNote("applyTiered", 0, "first", "user")
	if err != nil {
		t.Fatal(err)
	}
	patch := func(body string) notes.Note {
		t.Helper()
		code, out := ts.do(t, "PATCH", "/api/notes/"+n.ID, body, ts.auth())
		var got notes.Note
		if code != 200 || json.Unmarshal([]byte(out), &got) != nil {
			t.Fatalf("patch %s: %d %s", body, code, out)
		}
		return got
	}
	if got := patch(`{"text":"  second  "}`); got.Text != "second" || got.Resolved {
		t.Errorf("text: %+v", got)
	}
	if got := patch(`{"text":"   "}`); got.Text != "second" {
		t.Errorf("blank text must keep the old one: %+v", got)
	}
	if got := patch(`{"resolved":true}`); !got.Resolved || got.Text != "second" {
		t.Errorf("resolve: %+v", got)
	}
	if got := patch(`{"resolved":false}`); got.Resolved {
		t.Errorf("reopen: %+v", got)
	}
}

func TestAnnotatedAndPlacedNotes(t *testing.T) {
	ts := start(t, nil)
	src, status, err := ts.annotated("applyTiered")
	if err != nil || status != http.StatusOK || src.From != 18 {
		t.Fatalf("annotated: %d %v from %d", status, err, src.From)
	}
	if _, err := ts.AddNote("applyTiered", 0, "whole function", "user"); err != nil {
		t.Fatal(err)
	}
	snap := ts.snapshot()
	n, _ := snap.Node("github.com/acme/ledger/internal/pricing.applyTiered")
	placed, err := ts.placedNotes(n)
	if err != nil || len(placed) != 1 || placed[0].Text != "whole function" {
		t.Errorf("placed notes: %+v %v", placed, err)
	}
}

func TestRefreshScopesOnlyWithAnID(t *testing.T) {
	var runs []analysis.Options
	s := bare(t, func(_ context.Context, o analysis.Options) (analysis.Result, error) {
		runs = append(runs, o)
		return analysis.Result{}, nil
	})
	runs = nil // New ran it once
	ch := s.events.subscribe()
	s.refresh(context.Background(), refreshRequest{})
	s.refresh(context.Background(), refreshRequest{ID: "applyTiered"})
	if len(runs) != 2 || runs[0].Scope != nil || runs[1].Scope == nil {
		t.Fatalf("scopes: %+v", runs)
	}
	if got := published(ch); strings.Join(got, ",") != "progress,snapshot,toast,progress,snapshot,toast" {
		t.Errorf("events %v", got)
	}
}

func TestRescanReplacesTheSnapshot(t *testing.T) {
	n := 0
	var fail error
	s := bare(t, func(context.Context, analysis.Options) (analysis.Result, error) {
		n++
		return analysis.Result{Snapshot: model.Snapshot{Generated: testNow.Add(time.Duration(n) * time.Minute)}}, fail
	})
	ch := s.events.subscribe()
	if !s.rescan(context.Background()) || !s.snapshot().Generated.Equal(testNow.Add(2*time.Minute)) {
		t.Fatalf("rescan did not swap in the new snapshot: %v", s.snapshot().Generated)
	}
	fail = errors.New("broken module")
	if !s.rescan(context.Background()) || !s.snapshot().Generated.Equal(testNow.Add(2*time.Minute)) {
		t.Errorf("a failed rescan must keep the snapshot: %v", s.snapshot().Generated)
	}
	if got := published(ch); strings.Join(got, ",") != "snapshot,toast" {
		t.Errorf("events %v", got)
	}
}

func TestSaveThemeKeepsTheRestOfTheFile(t *testing.T) {
	s := bare(t, nil)
	path := filepath.Join(s.opt.Root, config.FileName)
	if err := os.WriteFile(path, []byte("[ui]\ntheme = \"nord\"\n\n[thresholds]\ncrap_max = 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.saveTheme("dracula"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `theme = "dracula"`) || !strings.Contains(string(b), "crap_max = 5") || s.config().UI.Theme != "dracula" {
		t.Errorf("file %q, theme %q", b, s.config().UI.Theme)
	}
	s.opt.ConfigPath = filepath.Join(s.opt.Root, "missing", "dir", config.FileName)
	if err := s.saveTheme("nord"); err == nil || !strings.Contains(err.Error(), "save theme to ") {
		t.Errorf("unwritable path: %v", err)
	}
}

func TestSendToTmux(t *testing.T) {
	s := bare(t, nil)
	if sent, msg := s.sendToTmux(context.Background(), model.Snapshot{}, model.Node{}, focus.Focus{}); sent || msg != "" {
		t.Errorf("no target: %v %q", sent, msg)
	}
	s.cfg.Agent.TmuxTarget = "qtldr-test-no-such-session:0.0"
	if sent, msg := s.sendToTmux(context.Background(), model.Snapshot{}, model.Node{}, focus.Focus{}); sent || !strings.Contains(msg, "qtldr-test-no-such-session") {
		t.Errorf("unreachable target must say why: %v %q", sent, msg)
	}
}

func TestSetSnapshotPublishesStaleOnlyWhenSome(t *testing.T) {
	s := bare(t, nil)
	ch := s.events.subscribe()
	snap := func(stale bool) model.Snapshot {
		return model.Snapshot{Graph: model.Graph{Metrics: map[model.ID]model.Metrics{
			"m/p.f": {CC: model.Ptr(1), Coverage: &model.Coverage{Stale: stale}},
		}}}
	}
	s.setSnapshot(snap(false))
	s.setSnapshot(snap(true))
	if got := published(ch); strings.Join(got, ",") != "snapshot,snapshot,stale" {
		t.Errorf("events %v", got)
	}
}

func TestStaticRoutes(t *testing.T) {
	s := bare(t, nil)
	s.opt.Assets = fstest.MapFS{
		"index.html":    {Data: []byte("<html><head></head></html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	h := s.static()
	index := `<meta name="qtldr-token" content="` + s.token + `">`
	cases := []struct {
		method, path string
		code         int
		body, cache  string
	}{
		{"GET", "/", 200, index, "no-store"},
		{"GET", "/index.html", 200, index, "no-store"},
		{"HEAD", "/", 200, "", "no-store"},
		{"POST", "/", 405, "method not allowed", ""},
		{"GET", "/assets/app.js", 200, "console.log(1)", "public, max-age=31536000, immutable"},
		{"GET", "/pkg/internal/pricing", 200, index, "no-store"}, // client-side route
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.body) || rec.Header().Get("Cache-Control") != c.cache {
			t.Errorf("%s %s: %d %q cache %q", c.method, c.path, rec.Code, rec.Body.String(), rec.Header().Get("Cache-Control"))
		}
	}
}

func TestScopeOf(t *testing.T) {
	g := model.Graph{Nodes: []model.Node{
		{ID: "m", Kind: model.KindModule},
		{ID: "m/p", Kind: model.KindPackage, Parent: "m"},
		{ID: "m/p.a", Kind: model.KindFunc, Parent: "m/p"},
		{ID: "m/p.b", Kind: model.KindFunc, Parent: "m/p"},
	}}
	cases := []struct{ id, want string }{{"m", "[m/p.a m/p.b]"}, {"m/p.a", "[m/p.a]"}, {"m/p", "[m/p.a m/p.b]"}}
	for _, c := range cases {
		if got, err := scopeOf(g, c.id); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("scopeOf(%s) = %v %v, want %s", c.id, got, err, c.want)
		}
	}
	if _, err := scopeOf(g, "nope"); err == nil {
		t.Error("unknown id")
	}
}

func TestRefreshMessage(t *testing.T) {
	rep := &mutate.Report{Packages: []mutate.PackageReport{{Killed: 3, Lived: 1}, {Killed: 2}}}
	cases := []struct {
		req  refreshRequest
		res  analysis.Result
		want string
	}{
		{refreshRequest{Coverage: true}, analysis.Result{Failed: []model.ID{"m/a", "m/b"}, LogPath: ".qtldr/logs/c.log"},
			"Coverage finished; tests failed in 2 packages (output in .qtldr/logs/c.log)"},
		{refreshRequest{Mutation: true}, analysis.Result{Mutation: rep}, "Mutation finished: 5 killed, 1 survived"},
		{refreshRequest{Mutation: true}, analysis.Result{}, "Refreshed"},
		{refreshRequest{Coverage: true}, analysis.Result{}, "Coverage finished"},
		{refreshRequest{}, analysis.Result{Mutation: rep}, "Refreshed"},
	}
	for _, c := range cases {
		if got := refreshMessage(c.req, c.res); got != c.want {
			t.Errorf("%+v: %q, want %q", c.req, got, c.want)
		}
	}
}
