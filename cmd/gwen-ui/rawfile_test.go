package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRawFilesServesFilesInsideTheFolderOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.pdf"), []byte("%PDF-1.4 hi"), 0o644))
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("no"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "link.txt")))

	assets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("asset")) })
	h := rawFiles(assets)
	get := func(target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		return rec
	}
	file := func(p string) string {
		return rawFilePath + "?" + url.Values{"dir": {dir}, "path": {p}}.Encode()
	}

	rec := get(file("doc.pdf"))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "%PDF-1.4 hi", rec.Body.String())
	require.Equal(t, "application/pdf", rec.Header().Get("Content-Type"))
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	require.Equal(t, http.StatusNotFound, get(file("../"+filepath.Base(outside)+"/secret.txt")).Code)
	require.Equal(t, http.StatusNotFound, get(file("link.txt")).Code)
	require.Equal(t, http.StatusNotFound, get(file("missing.png")).Code)
	require.Equal(t, http.StatusNotFound, get(file(".")).Code)

	// Everything else is the frontend's.
	require.Equal(t, "asset", get("/index.html").Body.String())
}
