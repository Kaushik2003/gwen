package store_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/stretchr/testify/require"
)

func TestSelfVersions(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	self := f.r.Self
	_, err := self.Current(f.ctx)
	require.ErrorIs(t, err, store.ErrNotFound, "before the assistant chose anything")
	history, err := self.History(f.ctx, 10)
	require.NoError(t, err)
	require.Empty(t, history)

	_, err = self.Add(f.ctx, model.SelfVersion{Attitude: "smug", Reason: "mood"})
	userErr(t, err, store.ErrInvalid, "reason")
	_, err = self.Add(f.ctx, model.SelfVersion{Attitude: strings.Repeat("x", 41), Reason: model.SelfChat})
	require.ErrorIs(t, err, store.ErrInvalid, "the schema bounds the attitude")

	th, _ := f.thread("Stop being so nice")
	since := at("09:00")
	chat, err := self.Add(f.ctx, model.SelfVersion{Attitude: "smug", Note: "They like it blunt", Since: &since,
		Reason: model.SelfChat, ThreadID: &th.ID})
	require.NoError(t, err)
	require.Len(t, chat.ID, 36)
	require.Equal(t, model.Envelope{CreatedAt: fxNow, UpdatedAt: fxNow, DeviceID: f.db.DeviceID(), Rev: 1}, chat.Envelope)
	got, err := self.Current(f.ctx)
	require.NoError(t, err)
	require.Equal(t, chat, got, "returned as stored")

	f.clk.Advance(time.Minute)
	reset, err := self.Add(f.ctx, model.SelfVersion{Reason: model.SelfReset})
	require.NoError(t, err)
	got, err = self.Current(f.ctx)
	require.NoError(t, err)
	require.Equal(t, reset, got, "a reset is a version with neither")
	require.Nil(t, got.Since)
	require.Nil(t, got.ThreadID)

	again, err := self.Add(f.ctx, model.SelfVersion{Attitude: "soft", Reason: model.SelfChat})
	require.NoError(t, err)
	got, err = self.Current(f.ctx)
	require.NoError(t, err)
	require.Equal(t, again.ID, got.ID, "the newest, even on the same instant")

	history, err = self.History(f.ctx, 0)
	require.NoError(t, err)
	require.Equal(t, []string{again.ID, reset.ID, chat.ID},
		ids(history, func(v model.SelfVersion) string { return v.ID }), "newest first")
	history, err = self.History(f.ctx, 2)
	require.NoError(t, err)
	require.Equal(t, []string{again.ID, reset.ID}, ids(history, func(v model.SelfVersion) string { return v.ID }))
}
