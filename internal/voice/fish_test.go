package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// oneByte reads a byte at a time, so every sample is split between reads.
type oneByte struct{ r io.Reader }

func (o oneByte) Read(p []byte) (int, error) { return o.r.Read(p[:min(1, len(p))]) }

func TestFishAsksForPCMAndStreamsIt(t *testing.T) {
	var got fishRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		require.Equal(t, FishModel, r.Header.Get("model"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "audio/pcm")
		w.Write(pcm(0, 16384))
		w.(http.Flusher).Flush()
		w.Write(pcm(-32768))
	}))
	defer srv.Close()
	f := &Fish{HTTP: srv.Client(), URL: srv.URL, Key: func() (string, error) { return "sk-test", nil }}
	var samples []float32
	err := f.Say(context.Background(), "Hi.", "", 1.5, func(s []float32) bool {
		samples = append(samples, s...)
		return true
	})
	require.NoError(t, err)
	require.Equal(t, []float32{0, 0.5, -1}, samples)
	require.Equal(t, fishRequest{Text: "Hi.", ReferenceID: FishVoice, Format: "pcm", SampleRate: SpeechRate, Latency: "balanced", Prosody: fishProsody{Speed: 1.5}}, got,
		"raw samples at the player's rate, in Gwen's voice when none is named")
}

func TestReadPCMJoinsSplitSamples(t *testing.T) {
	var samples []float32
	err := readPCM(oneByte{bytes.NewReader(pcm(1, -2, 3))}, func(s []float32) bool {
		samples = append(samples, s...)
		return true
	})
	require.NoError(t, err)
	require.Equal(t, []float32{1.0 / 32768, -2.0 / 32768, 3.0 / 32768}, samples)
}

func TestReadPCMStopsWhenTold(t *testing.T) {
	calls := 0
	err := readPCM(oneByte{bytes.NewReader(pcm(1, 2, 3))}, func([]float32) bool {
		calls++
		return false
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestFishSaysWhyItFailed(t *testing.T) {
	for status, want := range map[int]string{
		401: "did not accept the API key",
		402: "free allowance",
		429: "busy",
		400: "Fish Audio: reference not found",
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(`{"status":0,"message":"reference not found"}`))
		}))
		f := &Fish{HTTP: srv.Client(), URL: srv.URL, Key: func() (string, error) { return "sk-test", nil }}
		err := f.Say(context.Background(), "Hi.", FishVoice, 1, func([]float32) bool { return true })
		require.ErrorContains(t, err, want, "status %d", status)
		srv.Close()
	}

	f := &Fish{HTTP: http.DefaultClient, Key: func() (string, error) { return "", nil }}
	err := f.Say(context.Background(), "Hi.", FishVoice, 1, func([]float32) bool { return true })
	require.ErrorContains(t, err, "no Fish Audio API key")
}

func TestFishStopsWithItsContext(t *testing.T) {
	asked, done := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(asked)
		<-done // a reply that never comes
	}))
	defer srv.Close()
	defer close(done)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-asked
		cancel()
	}()
	f := &Fish{HTTP: srv.Client(), URL: srv.URL, Key: func() (string, error) { return "sk-test", nil }}
	err := f.Say(ctx, "Hi.", FishVoice, 1, func([]float32) bool { return true })
	require.ErrorIs(t, err, context.Canceled)
}
