package voice

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// archive builds a .tar.bz2 of files under one top directory, as the
// sherpa-onnx releases are, with the bzip2 program (Go only reads bzip2).
func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	if _, err := exec.LookPath("bzip2"); err != nil {
		t.Skip("needs bzip2")
	}
	src := t.TempDir()
	top := filepath.Join(src, "model")
	require.NoError(t, os.MkdirAll(filepath.Join(top, "test_wavs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(top, "test_wavs", "0.wav"), []byte("unused"), 0o644))
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(top, name), []byte(body), 0o644))
	}
	out := filepath.Join(t.TempDir(), "model.tar.bz2")
	require.NoError(t, exec.Command("tar", "-cjf", out, "-C", src, "model").Run())
	b, err := os.ReadFile(out)
	require.NoError(t, err)
	return b
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func serve(t *testing.T, body []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(body) }))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestInstallKeepsTheCheckedFiles(t *testing.T) {
	t.Parallel()
	files := map[string]string{"encoder.int8.onnx": "enc", "tokens.txt": "a 0\nb 1\n"}
	m := Model{Name: "m", URL: serve(t, archive(t, files)), Files: map[string]string{}}
	for name, body := range files {
		m.Files[name] = sum(body)
	}
	root := t.TempDir()
	require.False(t, m.Installed(root))
	var last [2]int64
	require.NoError(t, m.Install(context.Background(), http.DefaultClient, root, func(done, total int64) { last = [2]int64{done, total} }))
	require.True(t, m.Installed(root))
	require.Equal(t, last[0], last[1], "the last progress is the whole")
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary directory is left")
	got, err := os.ReadFile(filepath.Join(m.Dir(root), "tokens.txt"))
	require.NoError(t, err)
	require.Equal(t, files["tokens.txt"], string(got))
	_, err = os.Stat(filepath.Join(m.Dir(root), "0.wav"))
	require.ErrorIs(t, err, os.ErrNotExist, "only the listed files are kept")

	require.NoError(t, m.Remove(root))
	require.False(t, m.Installed(root))
}

func TestInstallRefusesADamagedOrIncompleteDownload(t *testing.T) {
	t.Parallel()
	body := archive(t, map[string]string{"encoder.int8.onnx": "enc"})
	for name, files := range map[string]map[string]string{
		"damaged": {"encoder.int8.onnx": sum("something else")},
		"missing": {"encoder.int8.onnx": sum("enc"), "tokens.txt": sum("t")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := Model{Name: "m", URL: serve(t, body), Files: files}
			root := t.TempDir()
			require.Error(t, m.Install(context.Background(), http.DefaultClient, root, func(int64, int64) {}))
			require.False(t, m.Installed(root))
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestInstallKeepsTheOldCopyWhenADownloadFails(t *testing.T) {
	t.Parallel()
	files := map[string]string{"tokens.txt": "t"}
	m := Model{Name: "m", URL: serve(t, archive(t, files)), Files: map[string]string{"tokens.txt": sum("t")}}
	root := t.TempDir()
	require.NoError(t, m.Install(context.Background(), http.DefaultClient, root, func(int64, int64) {}))
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	m.URL = srv.URL
	require.ErrorContains(t, m.Install(context.Background(), http.DefaultClient, root, func(int64, int64) {}), "404")
	require.True(t, m.Installed(root))
}

func pcm(samples ...int16) []byte {
	var b bytes.Buffer
	for _, s := range samples {
		binary.Write(&b, binary.LittleEndian, s)
	}
	return b.Bytes()
}

func TestDecodeAndLoudness(t *testing.T) {
	t.Parallel()
	out, rms := decode(pcm(16384, -16384, 0, 32767))
	require.Equal(t, []float32{0.5, -0.5, 0, 32767.0 / 32768}, out)
	require.InDelta(t, 0.6124, rms, 1e-3)
	require.Zero(t, loudness(0))
	require.Zero(t, loudness(0.001), "-60 dBFS is silence")
	require.InDelta(t, 2.0/3, loudness(0.1), 1e-3, "-20 dBFS, speech, is two thirds")
	require.Equal(t, 1.0, loudness(0.5))
}

func TestRecordingCollectsUntilStopped(t *testing.T) {
	t.Parallel()
	r, w := io.Pipe()
	var mu sync.Mutex
	var levels []float64
	stopped := false
	rec := newRecording(r, func() error { stopped = true; return w.Close() }, func() error { return nil }, func(l float64) {
		mu.Lock()
		levels = append(levels, l)
		mu.Unlock()
	})
	loud := make([]int16, chunk)
	for i := range loud {
		loud[i] = 16384
	}
	w.Write(pcm(loud...))
	w.Write(pcm(1, 2, 3)) // part of a chunk, then the end
	got, err := rec.Stop()
	require.NoError(t, err)
	require.True(t, stopped)
	require.Len(t, got, chunk+3)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, levels, 2)
	require.InDelta(t, loudness(0.5), levels[0], 1e-6)
}

func TestRecordingThatHeardNothingReportsWhy(t *testing.T) {
	t.Parallel()
	r, w := io.Pipe()
	w.Close() // pw-record exited at once
	rec := newRecording(r, func() error { t.Error("an ended capture is not stopped again"); return nil },
		func() error { return io.ErrClosedPipe }, func(float64) {})
	<-rec.done
	_, err := rec.Stop()
	require.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestEngineSkipsTooShortARecording(t *testing.T) {
	t.Parallel()
	e := &Engine{Root: t.TempDir()}
	text, err := e.Transcribe(make([]float32, minSamples-1))
	require.NoError(t, err)
	require.Empty(t, text)
	_, err = e.Transcribe(make([]float32, minSamples))
	require.ErrorContains(t, err, "not downloaded")
}
