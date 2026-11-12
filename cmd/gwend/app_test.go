package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestAppListener(t *testing.T) {
	t.Parallel()
	runtime := t.TempDir()
	app, err := startApp(runtime, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	require.NoError(t, err)
	t.Cleanup(app.close)

	path := filepath.Join(runtime, "app.json")
	info := readAppInfo(t, path)
	host := fmt.Sprintf("127.0.0.1:%d", info.Port)
	client := &http.Client{Transport: &http.Transport{}, Timeout: 5 * time.Second}
	t.Cleanup(client.CloseIdleConnections)

	for _, tc := range []struct {
		name, authorization, host string
		status                    int
	}{
		{"no authorization", "", host, http.StatusUnauthorized},
		{"wrong token", "Bearer wrong", host, http.StatusUnauthorized},
		{"wrong host", "Bearer " + info.Token, fmt.Sprintf("localhost:%d", info.Port), http.StatusUnauthorized},
		{"authorized", "Bearer " + info.Token, host, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+host+"/", nil)
			require.NoError(t, err)
			req.Host = tc.host
			if tc.authorization != "" {
				req.Header.Set("Authorization", tc.authorization)
			}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.status, resp.StatusCode, string(body))
			require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
			if tc.status == http.StatusOK {
				require.Equal(t, "ok", string(body))
			} else {
				require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
				require.JSONEq(t, `{"error":{"code":"unauthorized","message":"not allowed","details":{}}}`, string(body))
			}
		})
	}

	app.close()
	require.NoFileExists(t, path)
}

func TestAppDaemonRoutes(t *testing.T) {
	t.Parallel()
	d := startDaemonWith(t, setup{}, func(o *options) { o.app = true })
	path := filepath.Join(filepath.Dir(d.socket), "app.json")
	info := readAppInfo(t, path)
	host := fmt.Sprintf("127.0.0.1:%d", info.Port)
	client := &http.Client{Transport: &http.Transport{}, Timeout: 5 * time.Second}
	t.Cleanup(client.CloseIdleConnections)

	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/v1/health", http.StatusOK},
		{"/v1/nope", http.StatusNotFound},
	} {
		t.Run(tc.path, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+host+tc.path, nil)
			require.NoError(t, err)
			req.Host = host
			req.Header.Set("Authorization", "Bearer "+info.Token)
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, tc.status, resp.StatusCode)
			if tc.status == http.StatusOK {
				var health wire.Health
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&health))
				require.True(t, health.OK)
				require.Equal(t, "1.0.0-test", health.Version)
				require.Equal(t, os.Getpid(), health.PID)
			} else {
				var failure wire.ErrorResponse
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&failure))
				require.Equal(t, "not_found", failure.Error.Code)
			}
		})
	}

	require.NoError(t, d.stop())
	require.NoFileExists(t, path)
}

func TestAppWritePrivate(t *testing.T) {
	t.Parallel()
	for _, stale := range []bool{false, true} {
		t.Run(fmt.Sprintf("stale=%t", stale), func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "app.json")
			if stale {
				require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
				require.NoError(t, os.WriteFile(path+".new", []byte("stale"), 0o644))
				require.NoError(t, os.Chmod(path+".new", 0o644))
			}
			require.NoError(t, writePrivate(path, []byte("private\n")))
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "private\n", string(data))
			require.Equal(t, os.FileMode(0o600), fileMode(t, path))
			require.NoFileExists(t, path+".new")
		})
	}
}

func readAppInfo(t *testing.T, path string) appFile {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fileMode(t, path))
	var info appFile
	require.NoError(t, json.Unmarshal(data, &info))
	require.Positive(t, info.Port)
	require.LessOrEqual(t, info.Port, 65535)
	require.Equal(t, os.Getpid(), info.PID)
	token, err := hex.DecodeString(info.Token)
	require.NoError(t, err)
	require.Len(t, token, 32)
	require.JSONEq(t, fmt.Sprintf(`{"port":%d,"token":%q,"pid":%d}`, info.Port, info.Token, info.PID), string(data))
	return info
}
