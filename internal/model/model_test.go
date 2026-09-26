package model_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kzark/gwen/internal/model"
	"github.com/stretchr/testify/require"
)

func TestNewIDIsCanonicalV7(t *testing.T) {
	t.Parallel()
	a, b := model.NewID(), model.NewID()
	require.Len(t, a, 36)
	require.NotEqual(t, a, b)
	id, err := uuid.Parse(a)
	require.NoError(t, err)
	require.Equal(t, uuid.Version(7), id.Version())
	require.Equal(t, id.String(), a, "must be the lowercase canonical form")
}

func TestKindsAndSources(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		kind   string
		valid  bool
		isBrk  bool
		source string
		srcOK  bool
	}{
		{name: "work", kind: model.KindWork, valid: true, source: model.SourceUser, srcOK: true},
		{name: "auto break", kind: model.KindBreakAuto, valid: true, isBrk: true, source: model.SourceIdle, srcOK: true},
		{name: "manual break", kind: model.KindBreakManual, valid: true, isBrk: true, source: model.SourceEdit, srcOK: true},
		{name: "unknown", kind: "nap", source: "cron"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.valid, model.ValidKind(tc.kind))
			require.Equal(t, tc.isBrk, model.IsBreakKind(tc.kind))
			require.Equal(t, tc.srcOK, model.ValidSource(tc.source))
		})
	}
}
