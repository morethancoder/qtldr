package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/editor"
	"github.com/morethancoder/qtldr/internal/focus"
	"github.com/morethancoder/qtldr/internal/glossary"
	"github.com/morethancoder/qtldr/internal/lang/golang"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/notes"
	"github.com/morethancoder/qtldr/internal/source"
	"github.com/morethancoder/qtldr/internal/theme"
)

// Handler returns the whole HTTP API and UI behind the guard.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /api/node", s.handleNode)
	mux.HandleFunc("GET /api/source", s.handleSource)
	mux.HandleFunc("GET /api/prompt", s.handlePrompt)
	mux.HandleFunc("GET /api/glossary", s.handleGlossary)
	mux.HandleFunc("GET /api/themes", s.handleThemes)
	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("GET /api/notes", s.handleListNotes)
	mux.HandleFunc("POST /api/notes", s.handleAddNote)
	mux.HandleFunc("PATCH /api/notes/{id}", s.handlePatchNote)
	mux.HandleFunc("POST /api/focus", s.handleFocus)
	mux.HandleFunc("POST /api/open", s.handleOpen)
	mux.HandleFunc("POST /api/refresh", s.handleRefresh)
	mux.HandleFunc("POST /api/theme", s.handleTheme)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		apiError(w, http.StatusNotFound, "no such API endpoint: "+r.Method+" "+r.URL.Path)
	})
	mux.Handle("/", s.static())
	return s.guard(mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func apiError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("request body is not valid JSON: %w", err)
	}
	return nil
}

// lookup resolves the id query parameter (full ID or unique suffix).
func (s *Server) lookup(snap model.Snapshot, id string) (model.Node, error) {
	if id == "" {
		return model.Node{}, errors.New("missing id")
	}
	full, err := model.Resolve(snap.IDs(), id)
	if err != nil {
		return model.Node{}, err
	}
	n, _ := snap.Node(full)
	return n, nil
}

func (s *Server) handleSnapshot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.snapshot())
}

// nodeResponse is /api/node: the same detail as `qtldr show`, plus notes.
type nodeResponse struct {
	model.Detail
	Notes []notes.Placed `json:"notes"`
}

func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	snap := s.snapshot()
	n, err := s.lookup(snap, r.URL.Query().Get("id"))
	if err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}
	d, _ := snap.Detail(n.ID)
	placed, err := s.placedNotes(n)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nodeResponse{Detail: d, Notes: placed})
}

// placedNotes anchors the open notes of n in its current source.
func (s *Server) placedNotes(n model.Node) ([]notes.Placed, error) {
	all, err := notes.Load(s.opt.Root)
	if err != nil {
		return nil, err
	}
	return notes.PlaceAll(s.opt.Root, all, n), nil
}

func (s *Server) handleSource(w http.ResponseWriter, r *http.Request) {
	src, status, err := s.annotated(r.URL.Query().Get("id"))
	if err != nil {
		apiError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, src)
}

// annotated builds the annotated source of a function (also used by MCP).
func (s *Server) annotated(id string) (source.Source, int, error) {
	snap := s.snapshot()
	n, err := s.lookup(snap, id)
	if err != nil {
		return source.Source{}, http.StatusNotFound, err
	}
	if n.Kind != model.KindFunc {
		return source.Source{}, http.StatusBadRequest, fmt.Errorf("%s is a %s; source is shown for functions", n.ID, n.Kind)
	}
	return Annotated(s.opt.Root, snap, n)
}

// Annotated reads a function's source (from its doc comment) and annotates
// it with coverage, survivors and notes.
func Annotated(root string, snap model.Snapshot, n model.Node) (source.Source, int, error) {
	from := n.Line
	if n.DocLine > 0 {
		from = n.DocLine
	}
	text, err := golang.New().Source(context.Background(), root, n.File, from, n.EndLine)
	if err != nil {
		return source.Source{}, http.StatusInternalServerError, fmt.Errorf("%v (the file changed since the last scan? it is re-scanned on save with --watch)", err)
	}
	all, err := notes.Load(root)
	if err != nil {
		return source.Source{}, http.StatusInternalServerError, err
	}
	return source.Annotate(n, snap.Metrics[n.ID], notes.PlaceAll(root, all, n), text, from), http.StatusOK, nil
}

func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	snap := s.snapshot()
	n, err := s.lookup(snap, r.URL.Query().Get("id"))
	if err != nil {
		apiError(w, http.StatusNotFound, err.Error())
		return
	}
	d, _ := snap.Detail(n.ID)
	placed, _ := s.placedNotes(n)
	writeJSON(w, http.StatusOK, map[string]string{"prompt": focus.Prompt(d, s.config().Thresholds, placed, r.URL.Query().Get("message"))})
}

func (s *Server) handleGlossary(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, glossary.Load(s.config()))
}

func (s *Server) handleThemes(w http.ResponseWriter, _ *http.Request) {
	all, warnings, err := theme.All(s.opt.Root)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"themes": all, "warnings": warnings})
}

// configResponse is what the UI needs from the configuration.
type configResponse struct {
	Theme      string            `json:"theme"`
	ThemeLight string            `json:"theme_light"`
	ThemeDark  string            `json:"theme_dark"`
	Editors    []editor.Preset   `json:"editors"`
	Thresholds config.Thresholds `json:"thresholds"`
	Tmux       bool              `json:"tmux"`
	Agent      agentStatus       `json:"agent"`
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := s.config()
	writeJSON(w, http.StatusOK, configResponse{
		Theme: cfg.UI.Theme, ThemeLight: cfg.UI.ThemeLight, ThemeDark: cfg.UI.ThemeDark,
		Editors:    editor.Available(cfg.Editor, lookPath),
		Thresholds: cfg.Thresholds, Tmux: cfg.Agent.TmuxTarget != "", Agent: s.agentState(),
	})
}

func (s *Server) handleTheme(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Theme string `json:"theme"`
	}
	if err := readJSON(r, &req); err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	all, _, err := theme.All(s.opt.Root)
	if err != nil || !theme.Find(all, req.Theme) {
		apiError(w, http.StatusBadRequest, fmt.Sprintf("unknown theme %q", req.Theme))
		return
	}
	if err := s.saveTheme(req.Theme); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"theme": req.Theme})
}

// saveTheme writes [ui].theme into .qtldr.toml, keeping the rest of the file.
func (s *Server) saveTheme(id string) error {
	path := s.opt.ConfigPath
	if path == "" {
		path = filepath.Join(s.opt.Root, config.FileName)
	}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(path, []byte(config.SetString(string(b), "ui", "theme", id)), 0o644); err != nil {
		return fmt.Errorf("save theme to %s: %w", path, err)
	}
	s.mu.Lock()
	s.cfg.UI.Theme = id
	s.mu.Unlock()
	return nil
}
