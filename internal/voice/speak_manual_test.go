//go:build manual

package voice

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/config"
)

// manualSpeaker is Kokoro over the dashboard's models directory, after
// downloading it there when it is missing, which checks the pinned sums.
func manualSpeaker(t *testing.T) *Speaker {
	t.Helper()
	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(paths.DataDir, "models")
	if !Kokoro.Installed(root) {
		t.Logf("downloading %s into %s", Kokoro.Name, root)
		err := Kokoro.Install(context.Background(), http.DefaultClient, root, func(done, total int64) {
			if done == total {
				t.Logf("downloaded %d MB", total>>20)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	s := &Speaker{Root: root, Idle: time.Minute}
	t.Cleanup(s.Close)
	return s
}

// TestManualSpeakHeard has Kokoro say a sentence and Parakeet hear it back,
// silently, printing how fast each was. GWEN_VOICE_OUT keeps what was said as
// a 24 kHz WAV:
//
//	go test -tags manual -run TestManualSpeakHeard -v ./internal/voice
func TestManualSpeakHeard(t *testing.T) {
	const said = "Your essay is planned for ten thirty tomorrow morning, and the report is due on Friday."
	s := manualSpeaker(t)
	load := time.Now()
	if err := s.Say(context.Background(), "Hi.", DefaultVoice, 1, func([]float32) bool { return true }); err != nil {
		t.Fatal(err)
	}
	t.Logf("loaded and said hi in %s", time.Since(load).Round(time.Millisecond))
	var speech []float32
	chunks := 0
	start := time.Now()
	var first time.Duration
	err := s.Say(context.Background(), Readable(said), DefaultVoice, 1, func(samples []float32) bool {
		if chunks == 0 {
			first = time.Since(start)
		}
		chunks++
		speech = append(speech, samples...)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	secs := float64(len(speech)) / SpeechRate
	t.Logf("%.1f s of speech in %s (first sound after %s, %d chunks)", secs, took.Round(time.Millisecond), first.Round(time.Millisecond), chunks)
	if secs < 2 {
		t.Fatalf("only %.1f s of speech", secs)
	}
	if out := os.Getenv("GWEN_VOICE_OUT"); out != "" {
		if err := os.WriteFile(out, wav(speech, SpeechRate), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	heard, err := manualEngine(t).Transcribe(resample(speech, SpeechRate, SampleRate))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("heard %q", heard)
	if e := wer(said, heard); e > 0.2 {
		t.Fatalf("word error rate %.0f%%", e*100)
	}
}

// TestManualFishHeard has Fish Audio say a sentence with the key saved in
// Settings → AI, or FISH_API_KEY, and Parakeet hear it back, printing how
// soon the first sound came. GWEN_VOICE_OUT keeps it as a WAV:
//
//	go test -tags manual -run TestManualFishHeard -v ./internal/voice
func TestManualFishHeard(t *testing.T) {
	const said = "Your essay is planned for ten thirty tomorrow morning, and the report is due on Friday."
	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	key := os.Getenv("FISH_API_KEY")
	if key == "" {
		key, _ = config.ReadCredentialLine(paths.CredentialsDir(), config.CredFishAudioKey)
	}
	if key == "" {
		t.Skip("no Fish Audio key: save one in Settings → AI or set FISH_API_KEY")
	}
	f := &Fish{HTTP: http.DefaultClient, Key: func() (string, error) { return key, nil }}
	var speech []float32
	chunks := 0
	start := time.Now()
	var first time.Duration
	err = f.Say(context.Background(), said, FishVoice, 1, func(samples []float32) bool {
		if chunks == 0 {
			first = time.Since(start)
		}
		chunks++
		speech = append(speech, samples...)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	secs := float64(len(speech)) / SpeechRate
	t.Logf("%.1f s of speech in %s (first sound after %s, %d chunks)", secs, time.Since(start).Round(time.Millisecond), first.Round(time.Millisecond), chunks)
	if secs < 2 {
		t.Fatalf("only %.1f s of speech", secs)
	}
	if out := os.Getenv("GWEN_VOICE_OUT"); out != "" {
		if err := os.WriteFile(out, wav(speech, SpeechRate), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	heard, err := manualEngine(t).Transcribe(resample(speech, SpeechRate, SampleRate))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("heard %q", heard)
	if e := wer(said, heard); e > 0.2 {
		t.Fatalf("word error rate %.0f%%", e*100)
	}
}

// TestManualPlay plays GWEN_SAY aloud through pw-play, or a quarter second of
// silence when it is unset, which checks the player takes the stream:
//
//	GWEN_SAY="Hi, I'm Gwen." go test -tags manual -run TestManualPlay -v ./internal/voice
func TestManualPlay(t *testing.T) {
	samples := make([]float32, SpeechRate/4)
	if text := os.Getenv("GWEN_SAY"); text != "" {
		samples = nil
		err := manualSpeaker(t).Say(context.Background(), Readable(text), DefaultVoice, 1, func(s []float32) bool {
			samples = append(samples, s...)
			return true
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	p, err := Play(Player, SpeechRate)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := p.Write(samples); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("played %.2f s in %s", float64(len(samples))/SpeechRate, time.Since(start).Round(time.Millisecond))
}

// resample changes the rate of samples by linear interpolation.
func resample(samples []float32, from, to int) []float32 {
	out := make([]float32, len(samples)*to/from)
	for i := range out {
		x := float64(i) * float64(from) / float64(to)
		j := int(x)
		if j+1 >= len(samples) {
			out[i] = samples[len(samples)-1]
			continue
		}
		f := float32(x - float64(j))
		out[i] = samples[j]*(1-f) + samples[j+1]*f
	}
	return out
}

// wav is samples as a mono 16-bit WAV file.
func wav(samples []float32, rate int) []byte {
	le := func(b []byte, v uint32, n int) []byte {
		for i := range n {
			b = append(b, byte(v>>(8*i)))
		}
		return b
	}
	data := uint32(2 * len(samples))
	b := []byte("RIFF")
	b = le(b, 36+data, 4)
	b = append(b, "WAVEfmt "...)
	b = le(b, 16, 4)
	b = le(b, 1, 2) // PCM
	b = le(b, 1, 2) // mono
	b = le(b, uint32(rate), 4)
	b = le(b, uint32(rate*2), 4)
	b = le(b, 2, 2)
	b = le(b, 16, 2)
	b = append(b, "data"...)
	b = le(b, data, 4)
	for _, s := range samples {
		b = le(b, uint32(uint16(int16(max(-1, min(1, s))*32767))), 2)
	}
	return b
}
