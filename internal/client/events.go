package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/wire"
)

// EventStream yields the frames of one GET /v1/events connection.
type EventStream interface {
	// Next blocks until the next event. It returns io.EOF when the server ends
	// the stream, or the context's error after it is cancelled.
	Next() (wire.Event, error)
	Close() error
}

// sseReader parses text/event-stream: "id:", "event:", and "data:" fields,
// multi-line data joined with "\n", comment lines such as ": ping" ignored,
// and a blank line ending each frame.
type sseReader struct {
	r    *bufio.Reader
	body io.ReadCloser
}

func newSSEReader(body io.ReadCloser) *sseReader {
	return &sseReader{r: bufio.NewReader(body), body: body}
}

func (s *sseReader) Next() (wire.Event, error) {
	var ev wire.Event
	var data []string
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				err = io.EOF
			}
			return wire.Event{}, err
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			if data == nil {
				ev = wire.Event{}
				continue
			}
			ev.Data = json.RawMessage(strings.Join(data, "\n"))
			return ev, nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "id":
			if n, err := strconv.ParseInt(value, 10, 64); err == nil {
				ev.ID = n
			}
		case "event":
			ev.Name = value
		case "data":
			data = append(data, value)
		}
	}
}

func (s *sseReader) Close() error { return s.body.Close() }

// Reconnect backoff for Follower (docs/04-api-contract.md#events).
const (
	minBackoff = time.Second
	maxBackoff = 30 * time.Second
)

// Follower holds one event stream open for the life of a client, reconnecting
// with a backoff of 1 s doubling to 30 s. Every successful connection begins
// with a state_changed frame, and because there is no replay, a client should
// treat cached lists as stale in OnConnect and refetch them.
type Follower struct {
	API   API
	Clock clock.Clock

	// OnConnect is called after each successful connection, before its first
	// event. Optional.
	OnConnect func()
	// OnEvent is called for every event, in order.
	OnEvent func(wire.Event)
	// OnDisconnect is called when a connection attempt fails or an open stream
	// ends; err is ErrDaemonNotRunning while the daemon is down. Optional.
	OnDisconnect func(err error)
}

// Run follows events until ctx is cancelled, then returns ctx's error. The
// callbacks run on Run's goroutine.
func (f Follower) Run(ctx context.Context) error {
	backoff := minBackoff
	for {
		stream, err := f.API.Events(ctx)
		if err == nil {
			backoff = minBackoff
			// Closing the stream is what unblocks Next on cancellation, whatever
			// the stream's implementation does with ctx.
			stop := context.AfterFunc(ctx, func() { stream.Close() })
			if f.OnConnect != nil {
				f.OnConnect()
			}
			err = f.drain(stream)
			stop()
			stream.Close()
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if f.OnDisconnect != nil {
			f.OnDisconnect(err)
		}
		timer := f.Clock.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C():
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func (f Follower) drain(stream EventStream) error {
	for {
		ev, err := stream.Next()
		if err != nil {
			return err
		}
		f.OnEvent(ev)
	}
}
