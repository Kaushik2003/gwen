package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

// w27bPairService is a Service with a database and no core, on a fake
// clock: pairing needs nothing more.
func w27bPairService(t *testing.T) (*Service, *clock.Fake, *store.DB) {
	t.Helper()
	clk := clock.NewFake(testutil.At("09:00:00"))
	db := testutil.NewDBWithClock(t, clk)
	svc, err := New(Options{DB: db, Repos: store.NewRepos(db), Clock: clk, Config: config.Defaults, Home: t.TempDir()})
	require.NoError(t, err)
	return svc, clk, db
}

// w27bPhones counts the paired phones.
func w27bPhones(t *testing.T, svc *Service) int {
	t.Helper()
	devices, err := svc.Devices(t.Context())
	require.NoError(t, err)
	return len(devices)
}

// The QR code carries 128 random bits in base32 and lives ten minutes
// (plan.md 3.14.4).
func TestPairingCodeIs128BitsAndLivesTenMinutes(t *testing.T) {
	t.Parallel()
	svc, clk, _ := w27bPairService(t)
	code, expires, err := svc.NewPairCode()
	require.NoError(t, err)
	require.Len(t, code, 26, "128 bits are 26 base32 characters without padding")
	require.Empty(t, strings.Trim(code, "abcdefghijklmnopqrstuvwxyz234567"), "lower-case base32 only")
	require.Equal(t, clk.Now().Add(10*time.Minute), expires)

	other, _, err := svc.NewPairCode()
	require.NoError(t, err)
	require.NotEqual(t, code, other, "every code is fresh")
}

// A code is single use: the photo of a QR code someone took over your
// shoulder is worth nothing once your phone has paired with it.
func TestPairingCodeUsedTwiceIsRefused(t *testing.T) {
	t.Parallel()
	svc, _, _ := w27bPairService(t)
	code, _, err := svc.NewPairCode()
	require.NoError(t, err)

	token, err := svc.Pair(t.Context(), code, "Pixel")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	again, err := svc.Pair(t.Context(), code, "Someone else")
	require.ErrorIs(t, err, ErrBadPairCode)
	require.Empty(t, again)
	require.Equal(t, 1, w27bPhones(t, svc), "only the first use pairs a phone")

	// The spent code leaves no empty code behind that an empty guess would
	// match.
	_, err = svc.Pair(t.Context(), "", "Empty")
	require.ErrorIs(t, err, ErrBadPairCode)
}

// Ten minutes after it was shown, a code is refused, even if nobody used
// it. A minute before that it still works.
func TestPairingCodeUsedAfterTenMinutesIsRefused(t *testing.T) {
	t.Parallel()
	svc, clk, _ := w27bPairService(t)
	code, _, err := svc.NewPairCode()
	require.NoError(t, err)
	clk.Advance(10 * time.Minute)
	_, err = svc.Pair(t.Context(), code, "Pixel")
	require.ErrorIs(t, err, ErrBadPairCode)
	clk.Advance(time.Hour)
	_, err = svc.Pair(t.Context(), code, "Pixel")
	require.ErrorIs(t, err, ErrBadPairCode, "an old code never comes back")
	require.Zero(t, w27bPhones(t, svc))

	fresh, _, err := svc.NewPairCode()
	require.NoError(t, err)
	clk.Advance(9*time.Minute + 59*time.Second)
	_, err = svc.Pair(t.Context(), fresh, "Pixel")
	require.NoError(t, err, "a code is good for its whole ten minutes")
	require.Equal(t, 1, w27bPhones(t, svc))
}

// One live code at a time: showing a new QR code kills the one before.
func TestNewPairingCodeReplacesTheLiveOne(t *testing.T) {
	t.Parallel()
	svc, _, _ := w27bPairService(t)
	first, _, err := svc.NewPairCode()
	require.NoError(t, err)
	second, _, err := svc.NewPairCode()
	require.NoError(t, err)

	_, err = svc.Pair(t.Context(), first, "Pixel")
	require.ErrorIs(t, err, ErrBadPairCode)
	_, err = svc.Pair(t.Context(), second, "Pixel")
	require.NoError(t, err)
}

// A wrong guess is refused and does not spend the live code, so a typo on
// the phone does not force a new QR code.
func TestWrongPairingCodeLeavesTheLiveOne(t *testing.T) {
	t.Parallel()
	svc, _, _ := w27bPairService(t)
	_, err := svc.Pair(t.Context(), "", "Phone")
	require.ErrorIs(t, err, ErrBadPairCode, "with no code shown, nothing pairs")

	code, _, err := svc.NewPairCode()
	require.NoError(t, err)
	_, err = svc.Pair(t.Context(), strings.Repeat("a", 26), "Phone")
	require.ErrorIs(t, err, ErrBadPairCode)
	_, err = svc.Pair(t.Context(), "", "Phone")
	require.ErrorIs(t, err, ErrBadPairCode)

	// What a phone types back may differ in case and spaces.
	_, err = svc.Pair(t.Context(), "  "+strings.ToUpper(code)+"\n", "Phone")
	require.NoError(t, err)
}

// The phone's cookie is a fresh 256-bit token; gwend keeps only its SHA-256,
// so a copy of the database pairs no phone. Revoking the phone makes the
// token worthless.
func TestPairingStoresOnlyTheTokensHash(t *testing.T) {
	t.Parallel()
	svc, _, db := w27bPairService(t)
	code, _, err := svc.NewPairCode()
	require.NoError(t, err)
	token, err := svc.Pair(t.Context(), code, "Pixel")
	require.NoError(t, err)
	require.Len(t, token, 43, "32 bytes in unpadded base64url")

	var stored string
	require.NoError(t, db.SQL().QueryRowContext(t.Context(), `SELECT token_hash FROM remote_devices`).Scan(&stored))
	sum := sha256.Sum256([]byte(token))
	require.Equal(t, hex.EncodeToString(sum[:]), stored)
	require.NotContains(t, stored, token)

	d, err := svc.Device(t.Context(), token)
	require.NoError(t, err)
	require.Equal(t, "Pixel", d.Name)

	require.NoError(t, svc.Revoke(t.Context(), d.ID))
	_, err = svc.Device(t.Context(), token)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = svc.Device(t.Context(), "")
	require.ErrorIs(t, err, store.ErrNotFound, "no cookie is no phone")
}
