package vec

import (
	"testing"

	"github.com/kzark/gwen/internal/memory"
	"github.com/stretchr/testify/require"
)

// TestIndexRefs lists what the index holds, so a reconcile can drop what is
// gone.
func TestIndexRefs(t *testing.T) {
	t.Parallel()
	f := newFx(t, false)
	ix := f.open("nomic-embed-text")
	got, err := ix.Refs(f.ctx)
	require.NoError(t, err)
	require.Empty(t, got)

	require.NoError(t, ix.Upsert(f.ctx, []memory.Record{rec(lisbon, "Lives in Lisbon"), rec(acme, "Works at Acme"),
		rec(gym, "Gym on Tuesdays")}))
	got, err = ix.Refs(f.ctx)
	require.NoError(t, err)
	require.Equal(t, []string{acme, lisbon, gym}, got)

	require.NoError(t, ix.Delete(f.ctx, []string{lisbon}))
	got, err = ix.Refs(f.ctx)
	require.NoError(t, err)
	require.Equal(t, []string{acme, gym}, got)

	require.NoError(t, ix.Clear(f.ctx))
	got, err = ix.Refs(f.ctx)
	require.NoError(t, err)
	require.Empty(t, got)
}
