package server

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"slices"
	"strings"

	"github.com/morethancoder/qtldr/internal/analysis"
	"github.com/morethancoder/qtldr/internal/check"
	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/editor"
	"github.com/morethancoder/qtldr/internal/focus"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/notes"
)

var lookPath = exec.LookPath

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	all, err := notes.Load(s.opt.Root)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if id := r.URL.Query().Get("id"); id != "" {
		all = notes.ForTarget(all, model.ID(id))
	}
	writeJSON(w, http.StatusOK, all)
}

func (s *Server) handleAddNote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target string `json:"target"`
		Line   int    `json:"line"`
		Text   string `json:"text"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, err := s.AddNote(req.Target, req.Line, req.Text, "user")
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

// AddNote stores a note on target (line 0 = no line) and tells browsers.
func (s *Server) AddNote(target string, line int, text, author string) (notes.Note, error) {
	n, err := notes.Add(s.opt.Root, s.snapshot(), target, line, text, author, s.opt.Now())
	if err != nil {
		return notes.Note{}, err
	}
	s.Publish("notes", map[string]model.ID{"target": n.Target})
	return n, nil
}

func (s *Server) handlePatchNote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Resolved *bool   `json:"resolved"`
		Text     *string `json:"text"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, err := notes.Update(s.opt.Root, r.PathValue("id"), func(n *notes.Note) {
		if req.Resolved != nil {
			n.Resolved = *req.Resolved
		}
		if req.Text != nil && strings.TrimSpace(*req.Text) != "" {
			n.Text = strings.TrimSpace(*req.Text)
		}
	})
	if err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}
	s.Publish("notes", map[string]model.ID{"target": n.Target})
	writeJSON(w, http.StatusOK, n)
}

// focusRequest is what the UI posts on selection and on "Send to agent".
type focusRequest struct {
	ID           string `json:"id"`
	Level        string `json:"level"`
	SelectedLine int    `json:"selected_line"`
	Message      string `json:"message"`
	Sent         bool   `json:"sent"`
}

func (s *Server) handleFocus(w http.ResponseWriter, r *http.Request) {
	var req focusRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	snap := s.snapshot()
	n, err := s.lookup(snap, req.ID)
	if err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}
	f := focus.Focus{ID: n.ID, Level: req.Level, SelectedLine: req.SelectedLine, Message: strings.TrimSpace(req.Message), Sent: req.Sent, At: s.opt.Now()}
	if err := focus.Write(s.opt.Root, f); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"focus": f}
	if req.Sent {
		resp["tmux"], resp["tmux_error"] = s.sendToTmux(r.Context(), snap, n, f)
	}
	writeJSON(w, http.StatusOK, resp)
}

// sendToTmux types the fix prompt into [agent].tmux_target when set.
func (s *Server) sendToTmux(ctx context.Context, snap model.Snapshot, n model.Node, f focus.Focus) (bool, string) {
	target := s.config().Agent.TmuxTarget
	if target == "" {
		return false, ""
	}
	d, _ := snap.Detail(n.ID)
	placed, _ := s.placedNotes(n)
	if err := focus.SendTmux(ctx, target, focus.Prompt(d, s.config().Thresholds, placed, f.Message)); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// openRequest opens a node's file in an editor.
type openRequest struct {
	ID     string `json:"id"`
	Line   int    `json:"line"`
	Editor string `json:"editor"`
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req openRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	ed, target, err := s.openTarget(req)
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	args, err := editor.Open(r.Context(), s.opt.Root, ed, target)
	if err != nil {
		apiError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"command": args})
}

// openTarget picks the editor (the configured one unless another preset is
// asked for) and the file and line to open.
func (s *Server) openTarget(req openRequest) (config.Editor, editor.Target, error) {
	n, err := s.lookup(s.snapshot(), req.ID)
	if err != nil || n.File == "" {
		return config.Editor{}, editor.Target{}, fmt.Errorf("%s has no source file to open", req.ID)
	}
	ed := s.config().Editor
	if req.Editor != "" {
		ed = pickPreset(ed, req.Editor)
	}
	line := n.Line
	if req.Line > 0 {
		line = req.Line
	}
	return ed, editor.Target{File: n.File, Line: line, Col: 1}, nil
}

// pickPreset switches to another detected editor preset for one open.
func pickPreset(ed config.Editor, id string) config.Editor {
	ed.Preset = id
	return ed
}

// refreshRequest re-runs analysis for a scope ("" = whole module).
type refreshRequest struct {
	ID       string `json:"id"`
	Coverage bool   `json:"coverage"`
	Mutation bool   `json:"mutation"`
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := readJSON(r, &req); err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Mutation {
		apiError(w, http.StatusNotImplemented, "mutation testing is not available in this build yet; run `qtldr mutate` when it is")
		return
	}
	if !s.busy.CompareAndSwap(false, true) {
		apiError(w, http.StatusConflict, "an analysis is already running")
		return
	}
	go func() {
		defer s.busy.Store(false)
		s.refresh(context.Background(), req)
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// refresh runs the analysis and publishes progress, the new snapshot and a
// toast.
func (s *Server) refresh(ctx context.Context, req refreshRequest) {
	opt := analysis.Options{Root: s.opt.Root, Config: s.config(), Coverage: req.Coverage, Now: s.opt.Now(),
		Progress: func(msg string) { s.Publish("progress", map[string]any{"message": msg, "done": false}) }}
	if req.ID != "" {
		opt.Scope = func(g model.Graph) ([]model.ID, error) { return scopeOf(g, req.ID) }
	}
	res, err := s.opt.Run(ctx, opt)
	s.Publish("progress", map[string]any{"message": "", "done": true})
	if err != nil {
		s.toast("Refresh failed: %v", err)
		return
	}
	s.setSnapshot(res.Snapshot)
	switch {
	case len(res.Failed) > 0:
		s.toast("Coverage finished; tests failed in %d packages (output in %s)", len(res.Failed), res.LogPath)
	case req.Coverage:
		s.toast("Coverage finished")
	default:
		s.toast("Refreshed")
	}
}

// scopeOf turns a node ID into the functions to (re)measure.
func scopeOf(g model.Graph, id string) ([]model.ID, error) {
	full, err := model.Resolve(g.IDs(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := g.Node(full); n.Kind == model.KindModule {
		return check.All(g), nil
	}
	return check.Expand(g, []model.ID{full}), nil
}

// setSnapshot swaps in a new snapshot and tells browsers which functions
// just became stale.
func (s *Server) setSnapshot(next model.Snapshot) {
	s.mu.Lock()
	prev := s.snap
	s.snap = next
	s.mu.Unlock()
	s.Publish("snapshot", map[string]any{"generated": next.Generated})
	if stale := newlyStale(prev, next); len(stale) > 0 {
		s.Publish("stale", map[string]any{"ids": stale})
	}
}

// newlyStale lists functions whose coverage or mutation is stale in next
// but was not in prev.
func newlyStale(prev, next model.Snapshot) []model.ID {
	var ids []model.ID
	for id, m := range next.Metrics {
		if isStale(m) && !isStale(prev.Metrics[id]) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

func isStale(m model.Metrics) bool {
	return (m.Coverage != nil && m.Coverage.Stale && m.CC != nil) || (m.Mutation != nil && m.Mutation.Stale)
}
