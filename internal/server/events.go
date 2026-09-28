package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Event is one server-sent event: snapshot, stale, progress, toast, notes,
// agent, config.
type Event struct {
	Name string
	Data any
}

// broker fans events out to every connected stream.
type broker struct {
	mu      sync.Mutex
	clients map[chan Event]bool
	closed  bool
}

func newBroker() *broker { return &broker{clients: map[chan Event]bool{}} }

func (b *broker) subscribe() chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Event, 16)
	if b.closed {
		close(ch)
		return ch
	}
	b.clients[ch] = true
	return ch
}

func (b *broker) unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.clients[ch] {
		delete(b.clients, ch)
		close(ch)
	}
}

// publish never blocks: a slow client misses events and re-syncs on the
// next snapshot.
func (b *broker) publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- e:
		default:
		}
	}
}

func (b *broker) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for ch := range b.clients {
		delete(b.clients, ch)
		close(ch)
	}
}

// Publish sends an event to every connected browser.
func (s *Server) Publish(name string, data any) { s.events.publish(Event{Name: name, Data: data}) }

func (s *Server) toast(format string, args ...any) {
	s.Publish("toast", map[string]string{"message": fmt.Sprintf(format, args...)})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := s.events.subscribe()
	defer s.events.unsubscribe(ch)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
		case e, open := <-ch:
			if !open {
				return
			}
			writeEvent(w, e)
		}
		flusher.Flush()
	}
}

func writeEvent(w http.ResponseWriter, e Event) {
	b, err := json.Marshal(e.Data)
	if err != nil {
		b = []byte(`{}`)
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Name, b)
}
