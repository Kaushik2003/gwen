package wire_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestMillisRoundTrip(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 15, 10, 3, 0, 500_000_000, time.UTC)
	ms := wire.Millis(at)
	require.Equal(t, int64(1789466580500), ms)
	require.True(t, wire.Time(ms).Equal(at))

	require.Nil(t, wire.MillisPtr(nil))
	require.Nil(t, wire.TimePtr(nil))
	require.Equal(t, ms, *wire.MillisPtr(&at))
	require.True(t, wire.TimePtr(&ms).Equal(at))
}

func decodeStrict(t *testing.T, body string, v any) error {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(body)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func TestOptionalDecode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		body     string
		wantSet  bool
		wantNull bool
		wantVal  string
	}{
		{name: "absent", body: `{"rev": 3}`},
		{name: "null", body: `{"project_id": null}`, wantSet: true, wantNull: true},
		{name: "null with spaces", body: `{"project_id" :  null }`, wantSet: true, wantNull: true},
		{name: "value", body: `{"project_id": "abc"}`, wantSet: true, wantVal: "abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var req wire.PatchSegmentRequest
			require.NoError(t, decodeStrict(t, tc.body, &req))
			require.Equal(t, tc.wantSet, req.ProjectID.Set)
			require.Equal(t, tc.wantNull, req.ProjectID.Null)
			require.Equal(t, tc.wantVal, req.ProjectID.Value)
			require.False(t, req.TaskID.Set)
		})
	}
}

func TestOptionalDecodeRejectsWrongType(t *testing.T) {
	t.Parallel()
	var req wire.PatchTaskRequest
	require.Error(t, decodeStrict(t, `{"estimate_minutes": "ten"}`, &req))
}

func TestOptionalEncode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		req  wire.PatchTaskRequest
		want string
	}{
		{name: "absent fields are omitted", req: wire.PatchTaskRequest{}, want: `{}`},
		{
			name: "null and values",
			req: wire.PatchTaskRequest{
				ProjectID:       wire.Null[string](),
				DueDay:          wire.Some("2026-09-20"),
				EstimateMinutes: wire.Some(90),
			},
			want: `{"project_id":null,"due_day":"2026-09-20","estimate_minutes":90}`,
		},
		{
			name: "from pointer",
			req:  wire.PatchTaskRequest{ProjectID: wire.FromPtr[string](nil), DueDay: wire.FromPtr(ptr("2026-09-21"))},
			want: `{"project_id":null,"due_day":"2026-09-21"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tc.req)
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(b))

			var back wire.PatchTaskRequest
			require.NoError(t, decodeStrict(t, string(b), &back))
			require.Equal(t, tc.req.ProjectID.Ptr(), back.ProjectID.Ptr())
			require.Equal(t, tc.req.DueDay.Ptr(), back.DueDay.Ptr())
			require.Equal(t, tc.req.EstimateMinutes.Ptr(), back.EstimateMinutes.Ptr())
		})
	}
}

func TestOptionalPtr(t *testing.T) {
	t.Parallel()
	require.Nil(t, wire.Optional[int]{}.Ptr())
	require.Nil(t, wire.Null[int]().Ptr())
	require.Equal(t, 7, *wire.Some(7).Ptr())
}

func TestNullableFieldsAreAlwaysPresent(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(wire.Status{State: wire.StateOff, Warnings: []string{}})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	for _, key := range []string{"work_day", "open_segment", "project_id", "task_id", "idle_since_at", "snoozed_until_at", "today"} {
		v, ok := got[key]
		require.True(t, ok, "%s missing", key)
		require.Nil(t, v, key)
	}
	require.Equal(t, []any{}, got["warnings"])

	b, err = json.Marshal(wire.Task{})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &got))
	for _, key := range []string{"goal_id", "quantity", "quantity_done", "rrule", "template_id", "occurrence_day"} {
		_, ok := got[key]
		require.True(t, ok, "v2 field %s must be present in v1 responses", key)
	}
}

func TestErrorEnvelopeShape(t *testing.T) {
	t.Parallel()
	body := `{"error": {"code": "invalid_state", "message": "cannot start a break while off", "details": {"state": "off"}}}`
	var resp wire.ErrorResponse
	require.NoError(t, decodeStrict(t, body, &resp))
	require.Equal(t, wire.CodeInvalidState, resp.Error.Code)
	require.Equal(t, "off", resp.Error.Details["state"])
}

func ptr[T any](v T) *T { return &v }
