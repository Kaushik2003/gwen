package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestCredentialsListAndSet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := &Server{CredDir: dir}
	handler := s.Handler()
	list := func() wire.Credentials {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/credentials", nil))
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "application/json", w.Header().Get("Content-Type"))
		var response wire.Credentials
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		return response
	}
	want := wire.Credentials{Stored: map[string]bool{
		"ntfy_token":           false,
		"google_client.json":   false,
		"google_token.json":    false,
		"llm_api_key":          false,
		"sync_token":           false,
		"fish_audio_api_key":   false,
		"claude_sandbox_token": false,
	}}
	require.Equal(t, want, list())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/v1/credentials/llm_api_key", strings.NewReader(`{"value":"k"}`))
	r.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(w, r)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	require.Empty(t, w.Body.String())
	path := filepath.Join(dir, "llm_api_key")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "k\n", string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	want.Stored["llm_api_key"] = true
	require.Equal(t, want, list())
}

func TestCredentialInvalidRequests(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path, body, field string
	}{
		{"unknown credential", "/v1/credentials/unknown", `{"value":"k"}`, "name"},
		{"escaped traversal", "/v1/credentials/..%2Fx", `{"value":"k"}`, "name"},
		{"unknown field", "/v1/credentials/llm_api_key", `{"value":"k","extra":true}`, "extra"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "credentials")
			require.NoError(t, os.Mkdir(dir, 0o700))
			s := &Server{CredDir: dir}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPut, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			s.Handler().ServeHTTP(w, r)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			var failure wire.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &failure))
			require.Equal(t, "invalid_request", failure.Error.Code)
			require.Equal(t, tc.field, failure.Error.Details["field"])
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Empty(t, entries)
			require.NoFileExists(t, filepath.Join(filepath.Dir(dir), "x"))
		})
	}
}

func TestCredentialRejectsTraversal(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "credentials")
	require.NoError(t, os.Mkdir(dir, 0o700))
	s := &Server{CredDir: dir}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/v1/credentials/../x", strings.NewReader(`{"value":"k"}`))
	// ServeMux redirects literal /../ paths before matching the credential route.
	r.SetPathValue("name", "../x")
	err := s.setCredential(w, r)
	require.Error(t, err)
	s.fail(w, r, err)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.JSONEq(t, `{"error":{"code":"invalid_request","message":"\"../x\" is not a credential Gwen knows","details":{"field":"name"}}}`, w.Body.String())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
	require.NoFileExists(t, filepath.Join(filepath.Dir(dir), "x"))
}
