package client

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func readAll(t *testing.T, input string) ([]wire.Event, error) {
	t.Helper()
	r := newSSEReader(io.NopCloser(strings.NewReader(input)))
	var events []wire.Event
	for {
		ev, err := r.Next()
		if err != nil {
			return events, err
		}
		events = append(events, ev)
	}
}

func TestSSEReader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  []wire.Event
	}{
		{
			name:  "single frame",
			input: "id: 1\nevent: state_changed\ndata: {\"state\":\"off\"}\n\n",
			want:  []wire.Event{{ID: 1, Name: "state_changed", Data: json.RawMessage(`{"state":"off"}`)}},
		},
		{
			name:  "multi-line data is joined with newlines",
			input: "id: 7\nevent: day_changed\ndata: {\"day\":\ndata:  \"2026-09-15\"}\n\n",
			want:  []wire.Event{{ID: 7, Name: "day_changed", Data: json.RawMessage("{\"day\":\n \"2026-09-15\"}")}},
		},
		{
			name:  "ping comments are ignored",
			input: ": ping\n\nid: 2\n: ping\nevent: projects_changed\ndata: {}\n\n: ping\n\n",
			want:  []wire.Event{{ID: 2, Name: "projects_changed", Data: json.RawMessage(`{}`)}},
		},
		{
			name:  "CRLF line endings and no space after colon",
			input: "id:3\r\nevent:tasks_changed\r\ndata:{\"task_ids\":[]}\r\n\r\n",
			want:  []wire.Event{{ID: 3, Name: "tasks_changed", Data: json.RawMessage(`{"task_ids":[]}`)}},
		},
		{
			name:  "frame without data is dropped",
			input: "id: 4\nevent: nothing\n\nid: 5\nevent: config_changed\ndata: {}\n\n",
			want:  []wire.Event{{ID: 5, Name: "config_changed", Data: json.RawMessage(`{}`)}},
		},
		{
			name:  "unknown fields and bad ids are ignored",
			input: "retry: 100\nid: x\nevent: e\nfoo: bar\ndata: 1\n\n",
			want:  []wire.Event{{ID: 0, Name: "e", Data: json.RawMessage(`1`)}},
		},
		{
			name:  "a truncated final frame is not delivered",
			input: "id: 1\nevent: a\ndata: {}\n\nid: 2\nevent: b\ndata: {",
			want:  []wire.Event{{ID: 1, Name: "a", Data: json.RawMessage(`{}`)}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := readAll(t, tc.input)
			require.ErrorIs(t, err, io.EOF)
			require.Equal(t, tc.want, got)
		})
	}
}
