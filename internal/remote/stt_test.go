package remote

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// Voice goes through gwen-stt, a helper gwend starts on first use (plan.md
// 3.14.10): each request is a 4-byte little-endian length and that much
// 16 kHz mono 16-bit PCM, each reply one JSON line. These tests put this
// test binary in the helper's place, so they need no speech model.

// w27cFakeSTTEnv, set, makes this test binary a stand-in for gwen-stt.
const w27cFakeSTTEnv = "GWEN_W27C_FAKE_STT"

func init() {
	if os.Getenv(w27cFakeSTTEnv) == "1" {
		os.Exit(w27cFakeSTT(os.Args[1:], os.Stdin, os.Stdout))
	}
}

// What the fake does with a recording, by its first sample.
const (
	w27cHear   = 0 // answer with what arrived
	w27cRefuse = 1 // answer {"error": ...}
	w27cGarble = 2 // answer a line that isn't JSON
	w27cCrash  = 3 // exit without answering
	w27cHang   = 4 // never answer, until its input closes
)

// w27cBadArgs is the fake's exit status when it is not started the way
// gwend starts gwen-stt.
const w27cBadArgs = 2

// w27cFakeSTT speaks gwen-stt's protocol. Its answer says how many requests
// this process has had, how many samples arrived, and their sum, so a test
// sees both the framing and whether the helper was started again.
func w27cFakeSTT(args []string, stdin io.Reader, stdout io.Writer) int {
	if len(args) != 2 || args[0] != "--models" || args[1] == "" {
		fmt.Fprintln(os.Stderr, "fake gwen-stt: started with", args)
		return w27cBadArgs
	}
	r := bufio.NewReader(stdin)
	enc := json.NewEncoder(stdout)
	for n := 1; ; n++ {
		var size [4]byte
		if _, err := io.ReadFull(r, size[:]); err != nil {
			return 0
		}
		pcm := make([]byte, binary.LittleEndian.Uint32(size[:]))
		if _, err := io.ReadFull(r, pcm); err != nil {
			return 1
		}
		var sum int64
		for i := 0; i+1 < len(pcm); i += 2 {
			sum += int64(int16(binary.LittleEndian.Uint16(pcm[i:])))
		}
		mode := int16(binary.LittleEndian.Uint16(pcm))
		switch mode {
		case w27cRefuse:
			_ = enc.Encode(map[string]string{"error": "no speech found"})
		case w27cGarble:
			fmt.Fprintln(stdout, "<html>not json</html>")
		case w27cCrash:
			return 1
		case w27cHang:
			_, _ = io.Copy(io.Discard, r)
			return 0
		default:
			_ = enc.Encode(map[string]string{"text": fmt.Sprintf("request %d: %d samples, sum %d, models %s", n, len(pcm)/2, sum,
				filepath.Base(args[1]))})
		}
	}
}

// w27cPCM is a recording of the given samples.
func w27cPCM(samples ...int16) []byte {
	out := make([]byte, 2*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[2*i:], uint16(s))
	}
	return out
}

// w27cSTT is a helper whose program is this test binary, through a script
// that marks it as the fake, and a models folder with one model in it.
func w27cSTT(t *testing.T) (bin, models string) {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	dir := t.TempDir()
	bin = filepath.Join(dir, "gwen-stt")
	script := "#!/bin/sh\n" + w27cFakeSTTEnv + "=1 exec '" + strings.ReplaceAll(exe, "'", `'\''`) + "' \"$@\"\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	models = filepath.Join(dir, "models")
	require.NoError(t, os.MkdirAll(filepath.Join(models, "parakeet"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(models, "parakeet", "model.onnx"), []byte("x"), 0o644))
	return bin, models
}

func TestSTTFraming(t *testing.T) {
	t.Parallel()
	bin, models := w27cSTT(t)
	h := &sttHelper{bin: bin, models: models, clk: clock.NewFake(testutil.At("09:00:00"))}
	t.Cleanup(h.close)
	require.True(t, h.available())
	ctx := t.Context()

	text, err := h.transcribe(ctx, w27cPCM(w27cHear, 100, -40))
	require.NoError(t, err)
	require.Equal(t, "request 1: 3 samples, sum 60, models models", text)

	// The helper stays up between recordings: the model loads once.
	text, err = h.transcribe(ctx, w27cPCM(w27cHear, -32768, 32767, 5))
	require.NoError(t, err)
	require.Equal(t, "request 2: 4 samples, sum 4, models models", text)

	// An error it reports is the request's, and the helper stays.
	_, err = h.transcribe(ctx, w27cPCM(w27cRefuse, 0))
	require.EqualError(t, err, "no speech found")
	text, err = h.transcribe(ctx, w27cPCM(w27cHear))
	require.NoError(t, err)
	require.Equal(t, "request 4: 1 samples, sum 0, models models", text)

	// An answer that isn't JSON, a helper that quits, and a request given
	// up on each leave a helper whose state is unknown: the next recording
	// starts a fresh one.
	_, err = h.transcribe(ctx, w27cPCM(w27cGarble))
	require.EqualError(t, err, "the speech helper answered something Gwen can't read")
	text, err = h.transcribe(ctx, w27cPCM(w27cHear, 7))
	require.NoError(t, err)
	require.Equal(t, "request 1: 2 samples, sum 7, models models", text)

	_, err = h.transcribe(ctx, w27cPCM(w27cCrash))
	require.EqualError(t, err, "the speech helper stopped")
	text, err = h.transcribe(ctx, w27cPCM(w27cHear))
	require.NoError(t, err)
	require.Equal(t, "request 1: 1 samples, sum 0, models models", text)

	gaveUp, cancel := context.WithCancel(ctx)
	cancel()
	_, err = h.transcribe(gaveUp, w27cPCM(w27cHang, 1, 2))
	require.ErrorIs(t, err, context.Canceled)
	text, err = h.transcribe(ctx, w27cPCM(w27cHear, 1))
	require.NoError(t, err)
	require.Equal(t, "request 1: 2 samples, sum 1, models models", text)

	h.close()
	h.close() // twice is harmless
	h.mu.Lock()
	require.Nil(t, h.cmd, "closed")
	h.mu.Unlock()
}

// A full minute of audio is far more than a pipe holds: the length and
// every byte still arrive.
func TestSTTFramingOfAFullMinute(t *testing.T) {
	t.Parallel()
	bin, models := w27cSTT(t)
	h := &sttHelper{bin: bin, models: models, clk: clock.NewFake(testutil.At("09:00:00"))}
	t.Cleanup(h.close)
	samples := make([]int16, MaxSpeech/2)
	for i := range samples {
		samples[i] = int16(i%7 - 3)
	}
	samples[0] = w27cHear
	var sum int64
	for _, s := range samples {
		sum += int64(s)
	}
	text, err := h.transcribe(t.Context(), w27cPCM(samples...))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("request 1: %d samples, sum %d, models models", MaxSpeech/2, sum), text)
}

func TestSTTAvailable(t *testing.T) {
	t.Parallel()
	bin, models := w27cSTT(t)
	require.True(t, (&sttHelper{bin: bin, models: models}).available())
	require.False(t, (&sttHelper{models: models}).available(), "no helper")
	require.False(t, (&sttHelper{bin: filepath.Join(t.TempDir(), "gwen-stt"), models: models}).available(), "helper not installed")
	require.False(t, (&sttHelper{bin: bin, models: filepath.Join(t.TempDir(), "none")}).available(), "no models folder")
	empty := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(empty, "parakeet"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(empty, "README"), []byte("x"), 0o644))
	require.False(t, (&sttHelper{bin: bin, models: empty}).available(), "a model folder with nothing in it is no model")
}

// Transcribe checks the recording before it starts anything, and only
// returns text: it never sends (section 7, rule 3).
func TestTranscribeChecksTheRecording(t *testing.T) {
	t.Parallel()
	bin, models := w27cSTT(t)
	service := func(bin, models string) *Service {
		clk := clock.NewFake(testutil.At("09:00:00"))
		db := testutil.NewDBWithClock(t, clk)
		svc, err := New(Options{DB: db, Repos: store.NewRepos(db), Clock: clk, Config: config.Defaults, Home: t.TempDir(),
			SttBin: bin, ModelsDir: models})
		require.NoError(t, err)
		t.Cleanup(svc.stt.close)
		return svc
	}
	svc := service(bin, models)
	code := func(err error) string {
		var e *Error
		require.ErrorAs(t, err, &e)
		return e.Code + ": " + e.Message
	}
	ctx := t.Context()

	_, err := svc.Transcribe(ctx, nil)
	require.Equal(t, wire.CodeInvalidRequest+": that recording is empty", code(err))
	_, err = svc.Transcribe(ctx, []byte{1, 2, 3})
	require.Equal(t, wire.CodeInvalidRequest+": that recording is empty", code(err), "half a sample is no recording")
	_, err = svc.Transcribe(ctx, make([]byte, MaxSpeech+2))
	require.Equal(t, wire.CodeInvalidRequest+": that recording is longer than 60 seconds", code(err))

	got, err := svc.Transcribe(ctx, make([]byte, MaxSpeech))
	require.NoError(t, err)
	require.Equal(t, wire.CodeTranscription{Text: fmt.Sprintf("request 1: %d samples, sum 0, models models", MaxSpeech/2)}, got)

	_, err = svc.Transcribe(ctx, w27cPCM(w27cRefuse))
	require.Equal(t, wire.CodeUnavailable+": Couldn't turn that into text: no speech found", code(err))

	noVoice := service(bin, filepath.Join(t.TempDir(), "models"))
	_, err = noVoice.Transcribe(ctx, w27cPCM(1, 2))
	require.Equal(t, wire.CodeUnavailable+": Voice isn't set up on the laptop. Open Gwen › Settings › Voice.", code(err))
	noVoice.stt.mu.Lock()
	require.Nil(t, noVoice.stt.cmd, "nothing was started")
	noVoice.stt.mu.Unlock()
}
