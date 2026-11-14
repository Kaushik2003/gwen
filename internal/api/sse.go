package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/wire"
)

// pingInterval is how often an idle stream carries a ": ping" comment.
const pingInterval = 25 * time.Second

// subscriberBuffer is how many frames a slow client may fall behind before it
// is disconnected; it then reconnects and refetches, as after any drop.
const subscriberBuffer = 64

// Hub fans events out to every GET /v1/events stream. Frame ids increase
// from 1 for the life of the daemon.
type Hub struct {
	clk clock.Clock

	mu        sync.Mutex
	nextID    int64
	subs      map[chan []byte]struct{}
	closed    bool
	observers []func(name string)
}

// OnPublish makes fn see the name of every event published from now on, in
// the publishing goroutine; fn must not block. The daemon's background
// integrations use it to react to plan changes.
func (h *Hub) OnPublish(fn func(name string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.observers = append(h.observers, fn)
}

// NewHub returns a hub whose pings follow clk.
func NewHub(clk clock.Clock) *Hub {
	return &Hub{clk: clk, subs: map[chan []byte]struct{}{}}
}

// Publish sends an event to every stream.
func (h *Hub) Publish(name string, data any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, fn := range h.observers {
		fn(name)
	}
	frame, err := h.frame(name, data)
	if err != nil {
		slog.Error("encode event", "event", name, "err", err)
		return
	}
	for ch := range h.subs {
		select {
		case ch <- frame:
		default:
			delete(h.subs, ch) // too slow: drop it so it reconnects
			close(ch)
		}
	}
}

// frame encodes one event with the next id; h.mu must be held.
func (h *Hub) frame(name string, data any) ([]byte, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	h.nextID++
	return fmt.Appendf(nil, "id: %d\nevent: %s\ndata: %s\n\n", h.nextID, name, b), nil
}

// subscribe registers a stream whose first frame is first.
func (h *Hub) subscribe(firstName string, first any) (chan []byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, fmt.Errorf("event hub closed")
	}
	frame, err := h.frame(firstName, first)
	if err != nil {
		return nil, err
	}
	ch := make(chan []byte, subscriberBuffer)
	ch <- frame
	h.subs[ch] = struct{}{}
	return ch, nil
}

func (h *Hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}

// Close ends every stream, as at shutdown.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
}

// Subscribers is the number of open streams.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// events serves GET /v1/events. The first frame is state_changed with the
// current status, so a client never needs a separate GET /v1/status.
func (s *Server) events(w http.ResponseWriter, r *http.Request) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("events: response cannot stream")
	}
	st, err := s.Tracker.Status(r.Context())
	if err != nil {
		return err
	}
	ch, err := s.Hub.subscribe(wire.EventStateChanged, st)
	if err != nil {
		return err
	}
	defer s.Hub.unsubscribe(ch)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ping := s.Hub.clk.NewTimer(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case frame, ok := <-ch:
			if !ok {
				return nil
			}
			if _, err := w.Write(frame); err != nil {
				return nil
			}
		case <-ping.C():
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return nil
			}
			ping.Reset(pingInterval)
		}
		flusher.Flush()
	}
}
