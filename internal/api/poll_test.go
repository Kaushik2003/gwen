package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestPollSnapshot(t *testing.T) {
	t.Parallel()
	for _, count := range []int{0, 3} {
		t.Run(fmt.Sprintf("published=%d", count), func(t *testing.T) {
			t.Parallel()
			s, clk := newPollServer(t)
			for i := 0; i < count; i++ {
				s.Hub.Publish("changed", i)
			}
			w := httptest.NewRecorder()
			require.NoError(t, s.poll(w, httptest.NewRequest(http.MethodGet, "/v1/events/poll", nil)))
			require.Equal(t, wire.EventPoll{Seq: int64(count), Events: []wire.Event{}}, readPollResponse(t, w))
			require.Zero(t, clk.Armed())
		})
	}
}

func TestPollEvents(t *testing.T) {
	t.Parallel()
	s, clk := newPollServer(t)
	publishThreePollEvents(s.Hub)
	w := httptest.NewRecorder()
	require.NoError(t, s.poll(w, httptest.NewRequest(http.MethodGet, "/v1/events/poll?after=0", nil)))
	require.Equal(t, wire.EventPoll{Seq: 3, Events: []wire.Event{
		{ID: 1, Name: "first", Data: json.RawMessage(`{"n":1}`)},
		{ID: 2, Name: "second", Data: json.RawMessage(`{"n":2}`)},
		{ID: 3, Name: "third", Data: json.RawMessage(`{"n":3}`)},
	}}, readPollResponse(t, w))
	require.Zero(t, clk.Armed())
}

func TestPollWait(t *testing.T) {
	t.Parallel()
	for _, wake := range []string{"event", "timeout", "hub closed"} {
		t.Run(wake, func(t *testing.T) {
			t.Parallel()
			s, clk := newPollServer(t)
			publishThreePollEvents(s.Hub)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/events/poll?after=3", nil)
			w := httptest.NewRecorder()
			done := make(chan error, 1)
			go func() { done <- s.poll(w, r) }()
			clk.BlockUntil(1)
			select {
			case err := <-done:
				t.Fatalf("poll returned before an event or timeout: %v", err)
			default:
			}

			want := wire.EventPoll{Seq: 3, Events: []wire.Event{}}
			switch wake {
			case "event":
				s.Hub.Publish("fourth", map[string]int{"n": 4})
				want = wire.EventPoll{Seq: 4, Events: []wire.Event{
					{ID: 4, Name: "fourth", Data: json.RawMessage(`{"n":4}`)},
				}}
			case "timeout":
				clk.Advance(25 * time.Second)
			case "hub closed":
				s.Hub.Close()
			}
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("poll did not return after " + wake)
			}
			require.Equal(t, want, readPollResponse(t, w))
			require.Zero(t, clk.Armed())
		})
	}
}

func TestPollReset(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		count int
		after string
	}{
		{"ahead of hub", 3, "99"},
		{"expired history", pollKept + 1, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, _ := newPollServer(t)
			for i := 0; i < tc.count; i++ {
				s.Hub.Publish("changed", i)
			}
			w := httptest.NewRecorder()
			require.NoError(t, s.poll(w, httptest.NewRequest(http.MethodGet, "/v1/events/poll?after="+tc.after, nil)))
			require.Equal(t, wire.EventPoll{Seq: int64(tc.count), Events: []wire.Event{}, Reset: true}, readPollResponse(t, w))
		})
	}
}

func TestPollRetentionBoundary(t *testing.T) {
	t.Parallel()
	s, _ := newPollServer(t)
	for i := 1; i <= pollKept+1; i++ {
		s.Hub.Publish("changed", i)
	}
	w := httptest.NewRecorder()
	require.NoError(t, s.poll(w, httptest.NewRequest(http.MethodGet, "/v1/events/poll?after=1", nil)))
	got := readPollResponse(t, w)
	require.False(t, got.Reset)
	require.Equal(t, int64(257), got.Seq)
	require.Len(t, got.Events, 256)
	for i, event := range got.Events {
		require.Equal(t, wire.Event{ID: int64(i + 2), Name: "changed", Data: json.RawMessage(fmt.Sprint(i + 2))}, event)
	}
}

func TestPollInvalidAfter(t *testing.T) {
	t.Parallel()
	for _, after := range []string{"abc", "-1", "9223372036854775808"} {
		t.Run(after, func(t *testing.T) {
			t.Parallel()
			s, clk := newPollServer(t)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/events/poll?after="+after, nil))
			require.Equal(t, http.StatusBadRequest, w.Code)
			var failure wire.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &failure))
			require.Equal(t, "invalid_request", failure.Error.Code)
			require.Equal(t, "after", failure.Error.Details["field"])
			require.Zero(t, clk.Armed())
		})
	}
}

func TestPollRoute(t *testing.T) {
	t.Parallel()
	s, _ := newPollServer(t)
	s.Hub.Publish("changed", map[string]bool{"ready": true})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/events/poll?after=0", nil))
	require.Equal(t, wire.EventPoll{Seq: 1, Events: []wire.Event{
		{ID: 1, Name: "changed", Data: json.RawMessage(`{"ready":true}`)},
	}}, readPollResponse(t, w))
}

func newPollServer(t *testing.T) (*Server, *clock.Fake) {
	t.Helper()
	clk := clock.NewFake(time.Unix(0, 0))
	hub := NewHub(clk)
	t.Cleanup(hub.Close)
	return &Server{Hub: hub}, clk
}

func publishThreePollEvents(hub *Hub) {
	for i, name := range []string{"first", "second", "third"} {
		hub.Publish(name, map[string]int{"n": i + 1})
	}
}

func readPollResponse(t *testing.T, w *httptest.ResponseRecorder) wire.EventPoll {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	var response wire.EventPoll
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	return response
}
