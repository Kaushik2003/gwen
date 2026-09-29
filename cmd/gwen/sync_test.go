package main

import (
	"testing"

	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestSyncGolden(t *testing.T) {
	t.Parallel()
	ok := &wire.SyncStatus{Configured: true, LastPushAt: testutil.Ptr(wire.Millis(testutil.At("10:00"))),
		LastPullAt: testutil.Ptr(wire.Millis(testutil.At("10:01")))}
	f := fake().Returns("SyncStatus", ok, nil)
	out, _, code := runCLI(t, f, "sync", "status")
	require.Equal(t, exitOK, code)
	require.Equal(t, "Sync: configured\nLast push: 2026-09-15 10:00\nLast pull: 2026-09-15 10:01\n", out)

	failed := &wire.SyncStatus{Configured: true, LastError: testutil.Ptr("push: connection refused")}
	f = fake().Returns("SyncNow", failed, nil)
	out, _, code = runCLI(t, f, "sync", "now")
	require.Equal(t, exitOK, code)
	require.Equal(t, "Sync: configured\nLast push: never\nLast pull: never\nLast error: push: connection refused\n", out)
}

func TestSetupSync(t *testing.T) {
	t.Parallel()
	f := clienttest.New().Returns("PatchConfig", &wire.Config{}, nil).
		Returns("SyncNow", &wire.SyncStatus{Configured: true}, nil)
	r := setupTestEnv(t, f, "")
	require.Error(t, setupExec(t, r, "sync", "--hub", "http://pi:7777"), "--token is required")
	require.NoError(t, setupExec(t, r, "sync", "--hub", "http://pi:7777/", "--token", " tok "))
	tok, err := config.ReadCredentialLine(r.env.credDir, config.CredSyncToken)
	require.NoError(t, err)
	require.Equal(t, "tok", tok)
	require.Equal(t, []any{wire.ConfigPatch{"sync": {"hub_url": "http://pi:7777"}}}, f.CallsTo("PatchConfig")[0].Args)
	require.Len(t, f.CallsTo("SyncNow"), 1)
	require.Contains(t, r.out.String(), "http://pi:7777/")
}
