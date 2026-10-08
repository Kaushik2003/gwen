//go:build manual

package voice

import (
	"context"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/config"
)

// manualEngine is the engine over the dashboard's models directory, after
// downloading Parakeet there when it is missing, which checks the pinned sums.
func manualEngine(t *testing.T) *Engine {
	t.Helper()
	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(paths.DataDir, "models")
	if !Parakeet.Installed(root) {
		t.Logf("downloading %s into %s", Parakeet.Name, root)
		err := Parakeet.Install(context.Background(), http.DefaultClient, root, func(done, total int64) {
			if done == total {
				t.Logf("downloaded %d MB", total>>20)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	e := &Engine{Root: root, Idle: time.Minute}
	t.Cleanup(e.Close)
	return e
}

// TestManualTranscribeFile prints what Parakeet hears in a 16 kHz mono
// 16-bit WAV:
//
//	GWEN_VOICE_WAV=speech.wav go test -tags manual -run TestManualTranscribeFile -v ./internal/voice
func TestManualTranscribeFile(t *testing.T) {
	path := os.Getenv("GWEN_VOICE_WAV")
	if path == "" {
		t.Skip("set GWEN_VOICE_WAV")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	samples, _ := decode(b[44:]) // after the canonical WAV header
	e := manualEngine(t)
	start := time.Now()
	text, err := e.Transcribe(samples)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%.1f s of audio in %s: %q", float64(len(samples))/SampleRate, time.Since(start).Round(time.Millisecond), text)
}

// paced is a WAV played into a Session as fast as a microphone would.
type paced struct {
	samples []float32
	start   time.Time
}

func (p *paced) avail() int {
	return min(len(p.samples), int(time.Since(p.start).Seconds()*SampleRate))
}

func (p *paced) Since(i int) []float32 {
	return append([]float32(nil), p.samples[min(i, p.avail()):p.avail()]...)
}

func (p *paced) Stop() ([]float32, error) { return p.samples[:p.avail()], nil }

// TestManualLiveFile dictates a 16 kHz mono 16-bit WAV live, in real time,
// printing the text as it grows and how long Stop took at the end:
//
//	GWEN_VOICE_WAV=speech.wav go test -tags manual -run TestManualLiveFile -v ./internal/voice
func TestManualLiveFile(t *testing.T) {
	path := os.Getenv("GWEN_VOICE_WAV")
	if path == "" {
		t.Skip("set GWEN_VOICE_WAV")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	samples, _ := decode(b[44:])
	e := manualEngine(t)
	if err := e.Warm(); err != nil {
		t.Fatal(err)
	}
	vad, err := NewVAD(e.Root)
	if err != nil {
		t.Fatal(err)
	}
	p := &paced{samples: samples, start: time.Now()}
	s := Listen(p, e, vad, func(text string) { t.Logf("%5.1fs  %s", time.Since(p.start).Seconds(), text) })
	time.Sleep(time.Duration(float64(len(samples)) / SampleRate * float64(time.Second)))
	stopped := time.Now()
	text, err := s.Stop()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("stop took %s: %q", time.Since(stopped).Round(time.Millisecond), text)
}

// TestManualMicrophone dictates live for eight seconds from the default
// microphone, printing the text as it grows and then what Stop returned.
// Speak once it says so:
//
//	go test -tags manual -run TestManualMicrophone -v ./internal/voice
func TestManualMicrophone(t *testing.T) {
	e := manualEngine(t)
	if err := e.Warm(); err != nil {
		t.Fatal(err)
	}
	rec, err := Record(Recorder, func(float64) {})
	if err != nil {
		t.Fatal(err)
	}
	vad, err := NewVAD(e.Root)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	s := Listen(rec, e, vad, func(text string) { t.Logf("%5.1fs  %s", time.Since(start).Seconds(), text) })
	t.Log("speak now, for eight seconds")
	time.Sleep(8 * time.Second)
	stopped := time.Now()
	text, err := s.Stop()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("stop took %s: %q", time.Since(stopped).Round(time.Millisecond), text)
}

// TestManualAccuracy scores live dictation against one decode of the whole
// recording, by word error rate, for each 16 kHz mono 16-bit WAV in a
// directory: as recorded, and with a quiet room's and a loud fan's noise
// added. The text is checked against the .txt beside each WAV, or without
// one, against the whole recording's decode:
//
//	GWEN_VOICE_DIR=dir go test -tags manual -run TestManualAccuracy -v ./internal/voice
func TestManualAccuracy(t *testing.T) {
	dir := os.Getenv("GWEN_VOICE_DIR")
	if dir == "" {
		t.Skip("set GWEN_VOICE_DIR")
	}
	wavs, _ := filepath.Glob(filepath.Join(dir, "*.wav"))
	e := manualEngine(t)
	if err := e.Warm(); err != nil {
		t.Fatal(err)
	}
	var sumWhole, sumLive float64
	for _, path := range wavs {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		clean, _ := decode(b[44:])
		ref, err := os.ReadFile(strings.TrimSuffix(path, ".wav") + ".txt")
		if err != nil {
			text, _ := e.Transcribe(clean)
			ref = []byte(text)
		}
		for _, amp := range []float64{0, 0.003, 0.01} {
			samples := noisy(clean, amp)
			whole, err := e.Transcribe(samples)
			if err != nil {
				t.Fatal(err)
			}
			vad, err := NewVAD(e.Root)
			if err != nil {
				t.Fatal(err)
			}
			f := &feed{audio: samples}
			s := sessionWith(f, e, vad, func(string) {})
			play(s, f)
			live, err := s.Stop()
			if err != nil {
				t.Fatal(err)
			}
			w, l := wer(string(ref), whole), wer(string(ref), live)
			sumWhole, sumLive = sumWhole+w, sumLive+l
			t.Logf("%-24s noise %.3f  whole %5.1f%%  live %5.1f%%  %s", filepath.Base(path), amp, 100*w, 100*l, live)
		}
	}
	n := float64(3 * max(1, len(wavs)))
	t.Logf("mean word error rate: whole %.1f%%, live %.1f%%", 100*sumWhole/n, 100*sumLive/n)
}

// noisy is samples between a second of quiet each side, with white noise of
// amplitude amp over it all.
func noisy(samples []float32, amp float64) []float32 {
	r := rand.New(rand.NewPCG(7, 1))
	out := make([]float32, SampleRate+len(samples)+SampleRate)
	copy(out[SampleRate:], samples)
	for i := range out {
		out[i] += float32(amp * r.NormFloat64())
	}
	return out
}

var nonWord = regexp.MustCompile(`[^a-z0-9' ]+`)

// wer is the word error rate of hyp against ref, ignoring case and punctuation.
func wer(ref, hyp string) float64 {
	words := func(s string) []string {
		return strings.Fields(nonWord.ReplaceAllString(strings.ReplaceAll(strings.ToLower(s), "-", " "), " "))
	}
	r, h := words(ref), words(hyp)
	prev := make([]int, len(h)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(r); i++ {
		cur := make([]int, len(h)+1)
		cur[0] = i
		for j := 1; j <= len(h); j++ {
			sub := prev[j-1]
			if r[i-1] != h[j-1] {
				sub++
			}
			cur[j] = min(sub, prev[j]+1, cur[j-1]+1)
		}
		prev = cur
	}
	return float64(prev[len(h)]) / float64(max(1, len(r)))
}
