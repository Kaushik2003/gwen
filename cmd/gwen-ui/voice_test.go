package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kzark/gwen/internal/voice"
	"github.com/stretchr/testify/require"
)

type fakeRecording struct {
	samples []float32
	stopped int
}

func (r *fakeRecording) Since(int) []float32 { return nil }

func (r *fakeRecording) Stop() ([]float32, error) {
	r.stopped++
	return r.samples, nil
}

// fakeDetector hears everything as one stretch of speech, ending at Flush.
type fakeDetector struct {
	n     int
	ended []voice.Span
}

func (d *fakeDetector) Accept(s []float32) { d.n += len(s) }
func (d *fakeDetector) Speaking() bool     { return d.n > 0 && d.ended == nil }
func (d *fakeDetector) Flush()             { d.ended = []voice.Span{{From: 0, To: d.n, Quiet: d.n}} }
func (d *fakeDetector) Close()             {}

func (d *fakeDetector) Ended() []voice.Span {
	out := d.ended
	d.ended = nil
	return out
}

type fakeEngine struct {
	mu    sync.Mutex
	warm  int
	heard []float32
}

func (e *fakeEngine) Warm() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.warm++
	return nil
}

func (e *fakeEngine) Transcribe(s []float32) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.heard = s
	return "move the report to tomorrow", nil
}

// newTestDictation has the model "installed" under a temporary directory.
func newTestDictation(t *testing.T, installed bool) (*dictation, *fakeRecording, *fakeEngine) {
	t.Helper()
	root := t.TempDir()
	if installed {
		dir := voice.Parakeet.Dir(root)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		for name := range voice.Parakeet.Files {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))
		}
	}
	rec := &fakeRecording{samples: []float32{0.1, 0.2}}
	eng := &fakeEngine{}
	v := &dictation{
		root:     root,
		lookPath: func(string) (string, error) { return "/usr/bin/pw-record", nil },
		emit:     func(string, any) {},
		engine:   eng,
		record:   func(func(float64)) (voice.Source, error) { return rec, nil },
		detect:   func() (voice.Detector, error) { return &fakeDetector{}, nil },
	}
	return v, rec, eng
}

func TestDictationRecordsThenTranscribes(t *testing.T) {
	t.Parallel()
	v, rec, eng := newTestDictation(t, true)
	require.NoError(t, v.start())
	require.True(t, v.status().Listening)
	require.ErrorContains(t, v.start(), "already listening")
	text, err := v.stop()
	require.NoError(t, err)
	require.Equal(t, "move the report to tomorrow", text)
	require.Equal(t, rec.samples, eng.heard)
	require.False(t, v.status().Listening)

	text, err = v.stop()
	require.NoError(t, err, "stopping twice is harmless")
	require.Empty(t, text)
	require.Equal(t, 1, rec.stopped)
}

func TestDictationCancelDropsTheRecording(t *testing.T) {
	t.Parallel()
	v, rec, eng := newTestDictation(t, true)
	require.NoError(t, v.start())
	v.cancel()
	require.Equal(t, 1, rec.stopped)
	require.Nil(t, eng.heard)
	require.False(t, v.status().Listening)
}

func TestDictationNeedsTheModel(t *testing.T) {
	t.Parallel()
	v, _, _ := newTestDictation(t, false)
	st := v.status()
	require.False(t, st.Installed)
	require.True(t, st.Recorder)
	require.Equal(t, 99, st.DownloadMB)
	require.ErrorContains(t, v.start(), "not downloaded")
}

func TestDictationReportsAMissingRecorder(t *testing.T) {
	t.Parallel()
	v, _, _ := newTestDictation(t, true)
	v.lookPath = func(string) (string, error) { return "", errors.New("not found") }
	require.False(t, v.status().Recorder)
}

func TestDictationRemoveWaitsForTheRecording(t *testing.T) {
	t.Parallel()
	v, _, _ := newTestDictation(t, true)
	require.NoError(t, v.start())
	require.ErrorContains(t, v.remove(), "stop listening")
	v.cancel()
	require.NoError(t, v.remove())
	require.False(t, v.status().Installed)
}
