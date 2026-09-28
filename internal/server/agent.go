package server

import (
	"context"
	"os"
	"time"

	"github.com/morethancoder/qtldr/internal/focus"
	"github.com/morethancoder/qtldr/internal/notes"
	"github.com/morethancoder/qtldr/internal/store"
)

// agentStatus is shown in the top bar: an MCP client used qtldr in the
// last 60 seconds.
type agentStatus struct {
	Connected bool      `json:"connected"`
	Client    string    `json:"client,omitempty"`
	LastSeen  time.Time `json:"last_seen,omitzero"`
}

func (s *Server) agentState() agentStatus {
	ping, err := focus.ReadAgentPing(s.opt.Root)
	if err != nil {
		return agentStatus{}
	}
	return agentStatus{Connected: s.opt.Now().Sub(ping.At) <= 60*time.Second, Client: ping.Client, LastSeen: ping.At}
}

// pollAgent publishes an "agent" event when the connection state changes.
func (s *Server) pollAgent(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.checkAgent()
			s.checkFiles()
		}
	}
}

// checkAgent publishes the agent status when it changed since the last check.
func (s *Server) checkAgent() {
	st := s.agentState()
	s.agentMu.Lock()
	changed := st.Connected != s.agent.Connected || st.Client != s.agent.Client
	s.agent = st
	s.agentMu.Unlock()
	if changed {
		s.Publish("agent", st)
	}
}

// checkFiles notices notes and snapshots written by other qtldr processes
// (`qtldr mcp` add_note/refresh, `qtldr analyze`) and tells browsers.
func (s *Server) checkFiles() {
	mt := modTime(notes.Path(s.opt.Root))
	if s.notesChecked && !mt.Equal(s.notesSeen) {
		s.Publish("notes", map[string]string{"target": ""})
	}
	s.notesSeen, s.notesChecked = mt, true
	snap, err := store.ReadSnapshot(s.opt.Root)
	if err == nil && snap.Generated.After(s.snapshot().Generated) {
		s.setSnapshot(snap)
	}
}

func modTime(path string) time.Time {
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}
