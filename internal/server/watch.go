package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/morethancoder/qtldr/internal/analysis"
	"github.com/morethancoder/qtldr/internal/model"
)

// debounce is how long the watcher waits for more saves before re-scanning.
const debounce = 300 * time.Millisecond

// Watch re-scans structure and complexity when Go files change and pushes
// the new snapshot; changed functions' coverage and mutation show as stale.
// fsnotify is not recursive, so every package directory is added (see
// docs/decisions.md #11).
func (s *Server) Watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	for _, dir := range s.watchDirs() {
		if err := w.Add(dir); err != nil {
			s.logf("watch %s: %v", dir, err)
		}
	}
	s.watchOnce.Do(func() { close(s.watching) })
	return s.watchLoop(ctx, w.Events, w.Errors, w.Add)
}

// watchLoop debounces relevant events into re-scans until ctx is done.
func (s *Server) watchLoop(ctx context.Context, events <-chan fsnotify.Event, errs <-chan error, add func(string) error) error {
	var timer <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-events:
			timer = onEvent(ev, add, timer)
		case err := <-errs:
			s.logf("watch: %v", err)
		case <-timer:
			timer = s.onTimer(ctx)
		}
	}
}

// onEvent (re)starts the debounce timer for a relevant event.
func onEvent(ev fsnotify.Event, add func(string) error, timer <-chan time.Time) <-chan time.Time {
	if relevant(ev, add) {
		return time.After(debounce)
	}
	return timer
}

// onTimer re-scans, or re-arms the timer while another analysis runs.
func (s *Server) onTimer(ctx context.Context) <-chan time.Time {
	if s.rescan(ctx) {
		return nil
	}
	return time.After(debounce)
}

func (s *Server) watchDirs() []string {
	dirs := []string{s.opt.Root}
	for _, n := range s.snapshot().Nodes {
		if n.Kind == model.KindPackage && n.Dir != "." {
			dirs = append(dirs, filepath.Join(s.opt.Root, filepath.FromSlash(n.Dir)))
		}
	}
	return dirs
}

// relevant reports whether ev should trigger a re-scan; new directories are
// added to the watch instead. Only a pure Chmod is ignored: kqueue (macOS)
// reports a truncate+write as one Write|Chmod event.
func relevant(ev fsnotify.Event, add func(string) error) bool {
	if ev.Has(fsnotify.Create) && isNewDir(ev.Name) {
		_ = add(ev.Name)
		return false
	}
	name := filepath.Base(ev.Name)
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") && ev.Op != fsnotify.Chmod
}

func isNewDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir() && !strings.HasPrefix(filepath.Base(path), ".")
}

// rescan runs a structure-only analysis. It reports false, doing nothing,
// while another analysis runs, so the caller retries and no change is lost.
func (s *Server) rescan(ctx context.Context) bool {
	if !s.busy.CompareAndSwap(false, true) {
		return false
	}
	defer s.busy.Store(false)
	res, err := s.opt.Run(ctx, analysis.Options{Root: s.opt.Root, Config: s.config(), Now: s.opt.Now()})
	if err != nil {
		s.toast("Re-scan failed: %v", err)
		return true
	}
	s.setSnapshot(res.Snapshot)
	return true
}
