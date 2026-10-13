package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestWriteExportIsPrivate(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "memory.json")
	// A file that was there, readable by others, is replaced and made private.
	require.NoError(t, os.WriteFile(path, []byte("old and longer than the export"), 0o644))
	export := &wire.MemoryExport{ExportedAt: 1, Claims: []wire.MemoryClaim{{ID: "c1", Text: "Works at Acme"}}}
	require.NoError(t, writeExport(path, export))

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	var got wire.MemoryExport
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, "Works at Acme", got.Claims[0].Text)
}

func TestHomeRelative(t *testing.T) {
	t.Parallel()
	require.Equal(t, "~/Gwen/memory", homeRelative("/home/me/Gwen/memory", "/home/me"))
	require.Equal(t, "/home/me", homeRelative("/home/me", "/home/me"))
	require.Equal(t, "/srv/memory", homeRelative("/srv/memory", "/home/me"))
	require.Equal(t, "/home/me2/memory", homeRelative("/home/me2/memory", "/home/me"))
	require.Equal(t, "/home/me/x", homeRelative("/home/me/x", ""))
}

func TestExpandHome(t *testing.T) {
	t.Parallel()
	require.Equal(t, "/home/me/Gwen/memory", expandHome("~/Gwen/memory", "/home/me"))
	require.Equal(t, "/srv/memory", expandHome("/srv/memory", "/home/me"))
	require.Equal(t, "", expandHome("", "/home/me"))
}

func TestMemoryServiceKeysAreCredentials(t *testing.T) {
	t.Parallel()
	h, _, _ := newHost(t, clienttest.New())
	for _, name := range []string{config.CredMem0APIKey, config.CredSupermemoryAPIKey} {
		require.NoError(t, h.setCredential(name, "key"))
		require.True(t, h.hasCredential(name))
	}
}

func TestOpenMemoryFolder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := clienttest.New().Returns("GetConfig", &wire.Config{Memory: wire.MemoryConfig{FilesDir: dir}}, nil)
	h, r, _ := newHost(t, f)
	a := &App{ctx: context.Background(), api: f, host: h}
	require.NoError(t, a.OpenMemoryFolder())
	require.Equal(t, [][]string{{"xdg-open", dir}}, r.all())

	// Not made yet, or off: nothing opens, and the error says why.
	missing := filepath.Join(dir, "not-yet")
	f = clienttest.New().Returns("GetConfig", &wire.Config{Memory: wire.MemoryConfig{FilesDir: missing}}, nil)
	a = &App{ctx: context.Background(), api: f, host: h}
	require.ErrorContains(t, a.OpenMemoryFolder(), "isn't there yet")
	f = clienttest.New().Returns("GetConfig", &wire.Config{}, nil)
	a = &App{ctx: context.Background(), api: f, host: h}
	require.ErrorContains(t, a.OpenMemoryFolder(), "memory folder is off")
	require.Len(t, r.all(), 1)
}
