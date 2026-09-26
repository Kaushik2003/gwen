package clienttest_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

var _ client.API = (*clienttest.Fake)(nil)

func TestFakeRecordsAndAnswers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	project := "p1"
	f := clienttest.New().
		Returns("ClockIn", &wire.Status{State: wire.StateWorking, ProjectID: &project}, nil).
		On("GetDay", func(args ...any) (any, error) {
			return &wire.DayDetail{WorkDay: wire.WorkDay{Day: args[0].(string)}}, nil
		}).
		Returns("DeleteTask", nil, errors.New("gone"))

	st, err := f.ClockIn(ctx, wire.ClockInRequest{ProjectID: &project})
	require.NoError(t, err)
	require.Equal(t, wire.StateWorking, st.State)

	day, err := f.GetDay(ctx, "2026-09-15")
	require.NoError(t, err)
	require.Equal(t, "2026-09-15", day.WorkDay.Day)

	require.EqualError(t, f.DeleteTask(ctx, "t1"), "gone")

	_, err = f.Status(ctx)
	require.ErrorIs(t, err, clienttest.ErrNotScripted)

	require.Equal(t, []clienttest.Call{
		{Method: "ClockIn", Args: []any{wire.ClockInRequest{ProjectID: &project}}},
		{Method: "GetDay", Args: []any{"2026-09-15"}},
		{Method: "DeleteTask", Args: []any{"t1"}},
		{Method: "Status", Args: nil},
	}, f.Calls())
	require.Len(t, f.CallsTo("GetDay"), 1)
	f.Reset()
	require.Empty(t, f.Calls())
}

func TestFakeRejectsMisuse(t *testing.T) {
	t.Parallel()
	f := clienttest.New()
	require.Panics(t, func() { f.Returns("ClockInn", nil, nil) }, "unknown method")
	require.Panics(t, func() { f.On("Nope", nil) }, "unknown method")
	require.Panics(t, func() { f.Returns("Status", &wire.Project{}, nil) }, "wrong result type")
	require.Panics(t, func() { f.Returns("DeleteTask", &wire.Task{}, nil) }, "method returns only an error")

	f.On("Health", func(...any) (any, error) { return &wire.Status{}, nil })
	require.Panics(t, func() { f.Health(context.Background()) }, "handler returned the wrong type")
}

func TestStream(t *testing.T) {
	t.Parallel()
	s := clienttest.NewStream()
	f := clienttest.New().Returns("Events", s, nil)
	got, err := f.Events(context.Background())
	require.NoError(t, err)

	s.Send(wire.Event{ID: 1, Name: wire.EventStateChanged})
	s.Send(wire.Event{ID: 2, Name: wire.EventDayChanged})
	s.End(nil)

	ev, err := got.Next()
	require.NoError(t, err)
	require.Equal(t, int64(1), ev.ID)
	ev, err = got.Next()
	require.NoError(t, err)
	require.Equal(t, int64(2), ev.ID)
	_, err = got.Next()
	require.ErrorIs(t, err, io.EOF)
	require.NoError(t, got.Close())

	s2 := clienttest.NewStream()
	boom := errors.New("reset")
	s2.End(boom)
	_, err = s2.Next()
	require.ErrorIs(t, err, boom)
}
