package recall

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/stretchr/testify/require"
)

// TestDeadlineScale stretches every deadline for a slow run, and leaves
// them as designed by default.
func TestDeadlineScale(t *testing.T) {
	t.Parallel()
	r := New(Options{Scale: 10})
	require.Equal(t, 1500*time.Millisecond, r.deadline(memory.LaneClaims))
	require.Equal(t, 4*time.Second, r.deadline(memory.LaneVec))
	require.Equal(t, 5*time.Second, r.deadline(memory.LaneSupermemory))
	require.Equal(t, 6*time.Second, r.settle(memory.Query{Text: "x"}).Budget.Deadline)
	r = New(Options{Scale: 0.5})
	require.Equal(t, 150*time.Millisecond, r.deadline(memory.LaneClaims), "never shorter than designed")
}
