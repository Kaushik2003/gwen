package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/kzark/gwen/internal/wire"
)

// The long poll of plan.md 3.15: Gwen's screens inside Gwen Terminal can
// fetch but cannot hold a stream open, so they ask for the events after the
// last one they saw and wait for the answer.

// pollWait is how long GET /v1/events/poll waits for an event.
const pollWait = 25 * time.Second

// pollKept is how many published events the hub keeps for the long poll. A
// client further behind than this is told to refetch.
const pollKept = 256

// record keeps a published event for the long poll and wakes every waiting
// poll; h.mu must be held, and h.nextID is the event's id.
func (h *Hub) record(name string, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	h.recent = append(h.recent, wire.Event{ID: h.nextID, Name: name, Data: b})
	if len(h.recent) > pollKept {
		h.recent = append(h.recent[:0:0], h.recent[len(h.recent)-pollKept:]...)
	}
	if h.wake != nil {
		close(h.wake)
		h.wake = nil
	}
}

// after returns the kept events with an id above seq, the id of the newest
// event published, and whether the caller missed some: seq is older than
// what is kept, or newer than anything this daemon published. When there is
// nothing to return it also hands back a channel closed by the next event,
// nil once the hub is closed.
func (h *Hub) after(seq int64) (events []wire.Event, latest int64, missed bool, wait <-chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	latest = h.nextID
	if seq > latest {
		return nil, latest, true, nil
	}
	if len(h.recent) > 0 {
		if oldest := h.recent[0].ID; seq < oldest-1 && len(h.recent) == pollKept {
			return nil, latest, true, nil
		}
		for i, ev := range h.recent {
			if ev.ID > seq {
				return append([]wire.Event(nil), h.recent[i:]...), latest, false, nil
			}
		}
	}
	if h.closed {
		return nil, latest, false, nil
	}
	if h.wake == nil {
		h.wake = make(chan struct{})
	}
	return nil, latest, false, h.wake
}

// poll serves GET /v1/events/poll?after=<seq>. Without after it answers at
// once with the current sequence number, which is where a client starts.
// With it, it answers as soon as there are events after seq, or with none
// after 25 s. Reset tells the client it missed events and must refetch.
func (s *Server) poll(w http.ResponseWriter, r *http.Request) error {
	raw := r.URL.Query().Get("after")
	if raw == "" {
		_, latest, _, _ := s.Hub.after(1 << 62)
		writeJSON(w, http.StatusOK, wire.EventPoll{Seq: latest, Events: []wire.Event{}})
		return nil
	}
	seq, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seq < 0 {
		return badRequest("after", "after must be a sequence number")
	}
	timeout := s.Hub.clk.NewTimer(pollWait)
	defer timeout.Stop()
	for {
		events, latest, missed, wait := s.Hub.after(seq)
		switch {
		case missed:
			writeJSON(w, http.StatusOK, wire.EventPoll{Seq: latest, Events: []wire.Event{}, Reset: true})
			return nil
		case len(events) > 0:
			writeJSON(w, http.StatusOK, wire.EventPoll{Seq: events[len(events)-1].ID, Events: events})
			return nil
		case wait == nil:
			writeJSON(w, http.StatusOK, wire.EventPoll{Seq: seq, Events: []wire.Event{}})
			return nil
		}
		select {
		case <-r.Context().Done():
			return nil
		case <-timeout.C():
			writeJSON(w, http.StatusOK, wire.EventPoll{Seq: seq, Events: []wire.Event{}})
			return nil
		case <-wait:
		}
	}
}
