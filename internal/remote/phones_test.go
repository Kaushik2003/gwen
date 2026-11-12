package remote

import (
	"context"
	"testing"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

// phoneTestService is a Service with a database and two paired phones, and
// no core: enough for what phones.go does.
func phoneTestService(t *testing.T) (svc *Service, phoneA, phoneB string) {
	t.Helper()
	clk := clock.NewFake(testutil.At("09:00:00"))
	db := testutil.NewDBWithClock(t, clk)
	repos := store.NewRepos(db)
	svc, err := New(Options{DB: db, Repos: repos, Clock: clk, Config: config.Defaults, Home: t.TempDir()})
	require.NoError(t, err)
	a, err := repos.RemoteDevices.Create(t.Context(), "Pixel", tokenHash("token-a"))
	require.NoError(t, err)
	b, err := repos.RemoteDevices.Create(t.Context(), "iPad", tokenHash("token-b"))
	require.NoError(t, err)
	return svc, a.ID, b.ID
}

// phoneEnded reports whether ctx has ended, without waiting.
func phoneEnded(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func TestRevokeEndsThatPhonesOpenConnections(t *testing.T) {
	t.Parallel()
	svc, a, b := phoneTestService(t)
	view, endView := svc.PhoneContext(t.Context(), a)
	defer endView()
	events, endEvents := svc.PhoneContext(t.Context(), a)
	defer endEvents()
	other, endOther := svc.PhoneContext(t.Context(), b)
	defer endOther()
	laptop, endLaptop := svc.PhoneContext(t.Context(), "")
	defer endLaptop()

	require.NoError(t, svc.Revoke(t.Context(), a))
	require.True(t, phoneEnded(view), "its terminal view ends at once")
	require.True(t, phoneEnded(events), "and its event stream")
	require.False(t, phoneEnded(other), "another phone keeps its connections")
	require.False(t, phoneEnded(laptop), "the laptop is never cut off")

	// A request that passed the cookie check just before the revoke gets
	// no connection either.
	late, endLate := svc.PhoneContext(t.Context(), a)
	defer endLate()
	require.True(t, phoneEnded(late))
}

func TestRevokingAnUnknownPhoneEndsNothing(t *testing.T) {
	t.Parallel()
	svc, a, _ := phoneTestService(t)
	conn, end := svc.PhoneContext(t.Context(), a)
	defer end()
	require.ErrorIs(t, svc.Revoke(t.Context(), "no-such-phone"), store.ErrNotFound)
	require.False(t, phoneEnded(conn))
}

func TestEndPhonesEndsEveryPhoneButLetsThemBackIn(t *testing.T) {
	t.Parallel()
	svc, a, b := phoneTestService(t)
	connA, endA := svc.PhoneContext(t.Context(), a)
	defer endA()
	connB, endB := svc.PhoneContext(t.Context(), b)
	defer endB()
	laptop, endLaptop := svc.PhoneContext(t.Context(), "")
	defer endLaptop()

	svc.EndPhones() // the remote was turned off
	require.True(t, phoneEnded(connA))
	require.True(t, phoneEnded(connB))
	require.False(t, phoneEnded(laptop))

	// Still paired: once the remote is on again, the phone connects again.
	again, endAgain := svc.PhoneContext(t.Context(), a)
	defer endAgain()
	require.False(t, phoneEnded(again))
}

func TestPhoneConnectionsAreForgottenWhenDone(t *testing.T) {
	t.Parallel()
	svc, a, _ := phoneTestService(t)
	_, end1 := svc.PhoneContext(t.Context(), a)
	_, end2 := svc.PhoneContext(t.Context(), a)
	end1()
	end2()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	require.Empty(t, svc.phones, "a finished connection leaves nothing behind")
}
