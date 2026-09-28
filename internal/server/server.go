// Package server is `qtldr serve`: the embedded web UI, a JSON API over the
// current snapshot, server-sent events, and the file watcher. It binds
// 127.0.0.1 only; every mutating request needs the per-run token.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/morethancoder/qtldr/internal/analysis"
	"github.com/morethancoder/qtldr/internal/config"
	"github.com/morethancoder/qtldr/internal/model"
	"github.com/morethancoder/qtldr/internal/mutate"
)

// TokenHeader carries the per-run token on mutating requests.
const TokenHeader = "X-Qtldr-Token"

// Options configure a server.
type Options struct {
	Root       string
	Config     config.Config
	ConfigPath string
	// Assets is the built web UI (web.Dist()).
	Assets fs.FS
	// Log receives one line per notable event; may be nil.
	Log io.Writer
	// Now, Run and Mutator are replaced in tests.
	Now     func() time.Time
	Run     func(ctx context.Context, opt analysis.Options) (analysis.Result, error)
	Mutator mutate.Mutator
}

// Server holds the latest snapshot and the connected event streams.
type Server struct {
	opt     Options
	token   string
	mu      sync.RWMutex
	snap    model.Snapshot
	cfg     config.Config
	events  *broker
	busy    atomic.Bool
	addr    atomic.Value // "127.0.0.1:port" once listening
	agentMu sync.Mutex
	agent   agentStatus
	// notesSeen is the notes.json modification time last seen (poll loop only).
	notesSeen    time.Time
	notesChecked bool
	// watching is closed once Watch has added its directories.
	watching  chan struct{}
	watchOnce sync.Once
}

// New analyzes the module (structure plus cached coverage) and returns a
// server ready to listen.
func New(ctx context.Context, opt Options) (*Server, error) {
	if opt.Now == nil {
		opt.Now = func() time.Time { return time.Now().UTC().Truncate(time.Second) }
	}
	if opt.Run == nil {
		opt.Run = analysis.Run
	}
	if opt.Log == nil {
		opt.Log = io.Discard
	}
	s := &Server{opt: opt, token: newToken(), cfg: opt.Config, events: newBroker(), watching: make(chan struct{})}
	res, err := opt.Run(ctx, analysis.Options{Root: opt.Root, Config: opt.Config, Now: opt.Now()})
	if err != nil {
		return nil, err
	}
	s.snap = res.Snapshot
	return s, nil
}

func newToken() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Watching is closed once Watch is watching every package directory.
func (s *Server) Watching() <-chan struct{} { return s.watching }

// Token is the per-run token (tests use it).
func (s *Server) Token() string { return s.token }

// Listen binds 127.0.0.1:port (0 = a free port) and returns the listener
// and the URL to open.
func (s *Server) Listen(port int) (net.Listener, string, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, "", fmt.Errorf("listen on 127.0.0.1:%d: %w (pick another with --port or [ui].port)", port, err)
	}
	s.addr.Store(ln.Addr().String())
	return ln, "http://" + ln.Addr().String() + "/", nil
}

// Serve serves on ln until ctx is done.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go s.pollAgent(ctx)
	go func() {
		<-ctx.Done()
		s.events.close()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	err := srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) snapshot() model.Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

func (s *Server) config() config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *Server) logf(format string, args ...any) {
	fmt.Fprintf(s.opt.Log, format+"\n", args...)
}
