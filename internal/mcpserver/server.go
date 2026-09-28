package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/morethancoder/qtldr/internal/analysis"
	"github.com/morethancoder/qtldr/internal/check"
	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/focus"
	"github.com/morethancoder/qtldr/internal/gitx"
	"github.com/morethancoder/qtldr/internal/glossary"
	"github.com/morethancoder/qtldr/internal/metrics"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/mutate"
	"github.com/morethancoder/qtldr/internal/notes"
	"github.com/morethancoder/qtldr/internal/source"
	"github.com/morethancoder/qtldr/internal/store"
)

// Options configure the MCP server.
type Options struct {
	Root    string
	Config  config.Config
	Version string
	// Now and Mutator are replaced in tests.
	Now     func() time.Time
	Mutator mutate.Mutator
}

// service holds what every tool needs.
type service struct {
	opt  Options
	glos glossary.Glossary
}

// New builds the MCP server with every qtldr tool.
func New(opt Options) *mcp.Server {
	if opt.Now == nil {
		opt.Now = func() time.Time { return time.Now().UTC().Truncate(time.Second) }
	}
	s := &service{opt: opt, glos: glossary.Load(opt.Config)}
	srv := mcp.NewServer(&mcp.Implementation{Name: "qtldr", Title: "qtldr: code quality map", Version: opt.Version}, &mcp.ServerOptions{
		Instructions: "qtldr knows the CRAP, coverage and surviving mutants of every function in this Go module. " +
			"When the user says \"fix what I'm looking at\", call get_focus first. Before changing a function, call get_node for it. " +
			"After editing, call check (or run `qtldr check --changed`).",
	})
	addTools(srv, s)
	return srv
}

// Run serves over stdio until the client disconnects.
func Run(ctx context.Context, opt Options) error {
	return New(opt).Run(ctx, &mcp.StdioTransport{})
}

func tool(name, desc string) *mcp.Tool {
	return &mcp.Tool{Name: name, Description: desc}
}

func addTools(srv *mcp.Server, s *service) {
	mcp.AddTool(srv, tool("get_overview", "The module at a glance: packages with grades, the 10 riskiest functions, and how fresh the data is."), wrap(s, s.overview))
	mcp.AddTool(srv, tool("get_focus", "What the human is looking at in the qtldr UI (or sent with \"Send to agent\"): the node with its scores, surviving mutants, uncovered lines and notes, plus the user's message. Call this when asked to fix what the user is looking at."), wrap(s, s.getFocus))
	mcp.AddTool(srv, tool("get_node", "Everything about a package, function or type: scores against targets, callers, callees, uncovered line ranges, surviving mutants with the change each made, and notes."), wrap(s, s.getNode))
	mcp.AddTool(srv, tool("get_source", "A function's source, each line marked covered (C), uncovered (U) or partly covered (P), with surviving mutants and notes under the lines they belong to."), wrap(s, s.getSource))
	mcp.AddTool(srv, tool("list_worst", "Functions ranked worst first by a metric: crap, coverage, mutation, cognitive or cc."), wrap(s, s.listWorst))
	mcp.AddTool(srv, tool("explain", "What a qtldr term means (crap, coverage, mutation, survived, not_covered, cognitive, cc, churn, purity, grade, stale, …) and qtldr's target."), wrap(s, s.explain))
	mcp.AddTool(srv, tool("add_note", "Leave a note on a function (optionally on one line) or package; people see it in the qtldr UI. Use it to explain an equivalent mutant or a planned refactor."), wrap(s, s.addNote))
	mcp.AddTool(srv, tool("refresh", "Re-analyze a scope and return its new scores. coverage=true runs the tests; mutation=true runs mutation testing (slow; only changed code is re-tested)."), wrap(s, s.refresh))
	mcp.AddTool(srv, tool("check", "The pass/fail report of `qtldr check --json`: breaches of the CRAP, cognitive, coverage and mutation targets for changed functions (default), all functions, or one ID or file."), wrap(s, s.check))
}

// wrap records the agent ping (so the UI shows the agent as connected) and
// passes the client name to the handler.
func wrap[In, Out any](s *service, h func(ctx context.Context, client string, in In) (Out, error)) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		client := "agent"
		if info := req.ClientInfo(); info != nil && info.Name != "" {
			client = info.Name
		}
		_ = focus.WriteAgentPing(s.opt.Root, client, s.opt.Now())
		out, err := h(ctx, client, in)
		return nil, out, err
	}
}

// snapshot reads the current snapshot, building it when there is none.
func (s *service) snapshot(ctx context.Context) (model.Snapshot, error) {
	snap, err := store.ReadSnapshot(s.opt.Root)
	if errors.Is(err, store.ErrNoSnapshot) {
		res, err := analysis.Run(ctx, analysis.Options{Root: s.opt.Root, Config: s.opt.Config, Now: s.opt.Now()})
		return res.Snapshot, err
	}
	return snap, err
}

func (s *service) resolve(snap model.Snapshot, id string) (model.Node, error) {
	full, err := model.Resolve(snap.IDs(), strings.TrimSpace(id))
	if err != nil {
		return model.Node{}, err
	}
	n, _ := snap.Node(full)
	return n, nil
}

func (s *service) view(snap model.Snapshot, n model.Node) (NodeView, error) {
	d, _ := snap.Detail(n.ID)
	all, err := notes.Load(s.opt.Root)
	if err != nil {
		return NodeView{}, err
	}
	return nodeView(d, s.opt.Config.Thresholds, s.glos, notes.PlaceAll(s.opt.Root, all, n)), nil
}

// ---- get_overview ----

type empty struct{}

// PackageRow is one package in the overview.
type PackageRow struct {
	ID       model.ID      `json:"id"`
	Name     string        `json:"name"`
	Pure     bool          `json:"pure"`
	Grades   *model.Grades `json:"grades,omitempty"`
	CrapMax  *float64      `json:"crap_max"`
	Coverage *float64      `json:"coverage_percent"`
	Mutation *float64      `json:"mutation_score"`
	Riskiest string        `json:"riskiest_function,omitempty"`
	Problems []string      `json:"problems,omitempty"`
}

// Overview is get_overview's result.
type Overview struct {
	Module     string            `json:"module"`
	Generated  time.Time         `json:"generated"`
	Runs       model.Runs        `json:"data_freshness"`
	Packages   []PackageRow      `json:"packages"`
	TopRisks   []NodeView        `json:"top_risks"`
	Thresholds config.Thresholds `json:"targets"`
	Glossary   map[string]Term   `json:"glossary"`
}

func (s *service) overview(ctx context.Context, _ string, _ empty) (Overview, error) {
	snap, err := s.snapshot(ctx)
	if err != nil {
		return Overview{}, err
	}
	o := Overview{Module: snap.Module, Generated: snap.Generated, Runs: snap.Runs, Packages: []PackageRow{}, TopRisks: []NodeView{},
		Thresholds: s.opt.Config.Thresholds, Glossary: terms(s.glos, "crap", "coverage", "mutation", "grade", "purity", "not_measured")}
	for _, n := range snap.Nodes {
		if n.Kind == model.KindPackage {
			o.Packages = append(o.Packages, packageRow(snap, n))
		}
	}
	rank, _ := metrics.Rank(snap.Graph, metrics.CRAPMetric, 10, nil)
	for _, it := range rank.Items {
		n, _ := snap.Node(it.ID)
		v, err := s.view(snap, n)
		if err != nil {
			return o, err
		}
		v.Glossary = nil // once, at the top
		o.TopRisks = append(o.TopRisks, v)
	}
	return o, nil
}

func packageRow(snap model.Snapshot, n model.Node) PackageRow {
	m := snap.Metrics[n.ID]
	r := PackageRow{ID: n.ID, Name: n.Name, Pure: n.Pure != nil && *n.Pure, Grades: m.Grades, CrapMax: m.CrapMax}
	if m.Coverage != nil {
		r.Coverage = m.Coverage.Percent
	}
	if m.Mutation != nil {
		r.Mutation = m.Mutation.Score
	}
	if m.Worst != "" {
		r.Riskiest = check.Short(m.Worst)
	}
	for _, e := range []string{m.CoverageError, m.MutationError} {
		if e != "" {
			r.Problems = append(r.Problems, e)
		}
	}
	return r
}

// ---- get_focus ----

// FocusResult is get_focus's result.
type FocusResult struct {
	Focus        focus.Focus `json:"focus"`
	Message      string      `json:"message,omitempty"`
	SelectedLine string      `json:"selected_line_text,omitempty"`
	Node         NodeView    `json:"node"`
	FixPrompt    string      `json:"fix_prompt"`
}

func (s *service) getFocus(ctx context.Context, _ string, _ empty) (FocusResult, error) {
	f, err := focus.Read(s.opt.Root)
	if err != nil {
		return FocusResult{}, err
	}
	snap, err := s.snapshot(ctx)
	if err != nil {
		return FocusResult{}, err
	}
	n, err := s.resolve(snap, string(f.ID))
	if err != nil {
		return FocusResult{}, fmt.Errorf("the focused node %s is gone (the code changed); ask the user to select it again: %w", f.ID, err)
	}
	v, err := s.view(snap, n)
	if err != nil {
		return FocusResult{}, err
	}
	d, _ := snap.Detail(n.ID)
	all, _ := notes.Load(s.opt.Root)
	res := FocusResult{Focus: f, Message: f.Message, Node: v,
		FixPrompt: focus.Prompt(d, s.opt.Config.Thresholds, notes.PlaceAll(s.opt.Root, all, n), f.Message)}
	if f.SelectedLine > 0 {
		res.SelectedLine = lineText(notes.FuncLines(s.opt.Root, n), n.Line, f.SelectedLine)
	}
	return res, nil
}

func lineText(lines []string, start, line int) string {
	i := line - start
	if i < 0 || i >= len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[i])
}

// ---- get_node / get_source ----

// IDInput names a node.
type IDInput struct {
	ID string `json:"id" jsonschema:"package, function or type ID; a unique suffix such as pricing.applyTiered or applyTiered works"`
}

func (s *service) getNode(ctx context.Context, _ string, in IDInput) (NodeView, error) {
	snap, err := s.snapshot(ctx)
	if err != nil {
		return NodeView{}, err
	}
	n, err := s.resolve(snap, in.ID)
	if err != nil {
		return NodeView{}, err
	}
	return s.view(snap, n)
}

// SourceResult is get_source's result.
type SourceResult struct {
	ID            model.ID `json:"id"`
	File          string   `json:"file"`
	Lines         string   `json:"lines"`
	CoverageStale bool     `json:"coverage_stale"`
	MutationStale bool     `json:"mutation_stale"`
	Source        string   `json:"source"`
	Legend        string   `json:"legend"`
}

func (s *service) getSource(ctx context.Context, _ string, in IDInput) (SourceResult, error) {
	snap, err := s.snapshot(ctx)
	if err != nil {
		return SourceResult{}, err
	}
	n, err := s.resolve(snap, in.ID)
	if err != nil {
		return SourceResult{}, err
	}
	src, err := analysis.Source(s.opt.Root, snap, n)
	if err != nil {
		return SourceResult{}, err
	}
	return sourceResult(src), nil
}

func sourceResult(src source.Source) SourceResult {
	return SourceResult{ID: src.ID, File: src.File, Lines: fmt.Sprintf("%d-%d", src.From, src.To),
		CoverageStale: src.CoverageStale, MutationStale: src.MutationStale, Source: sourceText(src),
		Legend: "C covered · U not covered · P partly covered · blank: no statement; ^ lines annotate the line above"}
}

// ---- list_worst / explain ----

// WorstInput selects a ranking.
type WorstInput struct {
	Metric string `json:"metric,omitempty" jsonschema:"crap (default), coverage, mutation, cognitive or cc"`
	N      int    `json:"n,omitempty" jsonschema:"how many functions to list (default 10)"`
	Scope  string `json:"scope,omitempty" jsonschema:"optional package ID to limit the ranking to its functions"`
}

func (s *service) listWorst(ctx context.Context, _ string, in WorstInput) (metrics.Ranking, error) {
	snap, err := s.snapshot(ctx)
	if err != nil {
		return metrics.Ranking{}, err
	}
	var scope []model.ID
	if in.Scope != "" {
		n, err := s.resolve(snap, in.Scope)
		if err != nil {
			return metrics.Ranking{}, err
		}
		scope = check.Expand(snap.Graph, []model.ID{n.ID})
	}
	return metrics.Rank(snap.Graph, cmpOr(in.Metric, metrics.CRAPMetric), cmpOr(in.N, 10), scope)
}

func cmpOr[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}

// TermInput names a glossary term.
type TermInput struct {
	Term string `json:"term" jsonschema:"a term such as crap, coverage, mutation, survived, not_covered, cognitive, cc, churn, purity, grade, stale"`
}

func (s *service) explain(_ context.Context, _ string, in TermInput) (glossary.Term, error) {
	return s.glos.Lookup(in.Term)
}

// ---- add_note ----

// NoteInput is a note from an agent.
type NoteInput struct {
	ID   string `json:"id" jsonschema:"the function or package the note is about"`
	Line int    `json:"line,omitempty" jsonschema:"optional source line inside the function"`
	Text string `json:"text" jsonschema:"the note, one or two sentences"`
}

func (s *service) addNote(ctx context.Context, client string, in NoteInput) (notes.Note, error) {
	snap, err := s.snapshot(ctx)
	if err != nil {
		return notes.Note{}, err
	}
	return notes.Add(s.opt.Root, snap, in.ID, in.Line, in.Text, "agent:"+client, s.opt.Now())
}

// ---- refresh ----

// RefreshInput re-analyzes a scope.
type RefreshInput struct {
	Scope    string `json:"scope,omitempty" jsonschema:"a package or function ID; empty for the whole module"`
	Coverage bool   `json:"coverage,omitempty" jsonschema:"run the tests of the scope's packages"`
	Mutation bool   `json:"mutation,omitempty" jsonschema:"run mutation testing for the scope's packages (slow)"`
}

// RefreshResult is the scope after the refresh.
type RefreshResult struct {
	Scope    []NodeView     `json:"scope,omitempty"`
	More     int            `json:"more,omitempty"`
	Mutation *mutate.Report `json:"mutation,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

func (s *service) refresh(ctx context.Context, _ string, in RefreshInput) (RefreshResult, error) {
	opt := analysis.Options{Root: s.opt.Root, Config: s.opt.Config, Now: s.opt.Now(), Coverage: in.Coverage,
		Mutate: in.Mutation, Mutator: s.opt.Mutator}
	if in.Scope != "" {
		opt.Scope = func(g model.Graph) ([]model.ID, error) { return scopeIDs(g, in.Scope) }
	}
	res, err := analysis.Run(ctx, opt)
	if err != nil {
		return RefreshResult{}, err
	}
	out := RefreshResult{Mutation: res.Mutation, Warnings: res.Warnings}
	ids := res.Scope
	if ids == nil {
		ids = check.All(res.Snapshot.Graph)
	}
	metrics.SortByRisk(res.Snapshot.Graph, ids)
	return s.fillScope(res.Snapshot, ids, out)
}

func (s *service) fillScope(snap model.Snapshot, ids []model.ID, out RefreshResult) (RefreshResult, error) {
	for i, id := range ids {
		if i == 15 {
			out.More = len(ids) - i
			break
		}
		n, _ := snap.Node(id)
		v, err := s.view(snap, n)
		if err != nil {
			return out, err
		}
		v.Glossary = nil
		out.Scope = append(out.Scope, v)
	}
	return out, nil
}

func scopeIDs(g model.Graph, id string) ([]model.ID, error) {
	full, err := model.Resolve(g.IDs(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := g.Node(full); n.Kind == model.KindModule {
		return check.All(g), nil
	}
	return check.Expand(g, []model.ID{full}), nil
}

// ---- check ----

// CheckInput selects what to check.
type CheckInput struct {
	Scope string `json:"scope,omitempty" jsonschema:"changed (default: functions changed since the base ref), all, or a package/function ID or .go file"`
	Fast  bool   `json:"fast,omitempty" jsonschema:"skip running tests: check cognitive complexity, and CRAP where coverage is current"`
}

func (s *service) check(ctx context.Context, _ string, in CheckInput) (check.Report, error) {
	scopeName := cmpOr(in.Scope, "changed")
	opt := analysis.Options{Root: s.opt.Root, Config: s.opt.Config, Now: s.opt.Now(), Coverage: !in.Fast}
	opt.Scope = s.checkScope(ctx, scopeName)
	res, err := analysis.Run(ctx, opt)
	if err != nil {
		return check.Report{}, err
	}
	r := check.Evaluate(res.Snapshot.Graph, res.Scope, s.opt.Config.Thresholds, in.Fast)
	r.Scope = scopeName
	if scopeName == "changed" {
		r.Base = s.opt.Config.Project.BaseRef
	}
	return r, nil
}

func (s *service) checkScope(ctx context.Context, scope string) func(model.Graph) ([]model.ID, error) {
	return func(g model.Graph) ([]model.ID, error) {
		switch {
		case scope == "all":
			return check.All(g), nil
		case scope == "changed":
			ch, err := gitx.ReadChanges(ctx, s.opt.Root, s.opt.Config.Project.BaseRef)
			if err != nil {
				return nil, fmt.Errorf("scope changed: %w (try scope \"all\")", err)
			}
			return check.Changed(g, ch), nil
		case strings.HasSuffix(scope, ".go"):
			return check.InFiles(g, []string{strings.TrimPrefix(scope, "./")}), nil
		}
		return scopeIDs(g, scope)
	}
}
