package server

import (
	"context"
	"time"

	"github.com/morethancoder/qtldr/internal/focus"
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
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.checkAgent()
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
