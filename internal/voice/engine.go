package voice

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go-linux"
)

// minSamples is the shortest recording worth transcribing: 0.3 s.
const minSamples = SampleRate * 3 / 10

// Engine transcribes with Parakeet from the models directory Root, loaded on
// first use and freed after Idle without use, so its memory is only taken
// while you talk.
type Engine struct {
	Root string
	Idle time.Duration

	mu    sync.Mutex
	rec   *sherpa.OfflineRecognizer
	timer *time.Timer
	uses  int // counts uses, so a stale idle timer frees nothing
}

// Warm loads the model now, so a transcription soon after need not wait.
func (e *Engine) Warm() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	err := e.load()
	e.touch()
	return err
}

// Transcribe returns the text of 16 kHz mono samples, "" for silence or a
// recording too short to hold a word.
func (e *Engine) Transcribe(samples []float32) (string, error) {
	if len(samples) < minSamples {
		return "", nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	defer e.touch()
	if err := e.load(); err != nil {
		return "", err
	}
	s := sherpa.NewOfflineStream(e.rec)
	defer sherpa.DeleteOfflineStream(s)
	s.AcceptWaveform(SampleRate, samples)
	e.rec.Decode(s)
	return strings.TrimSpace(s.GetResult().Text), nil
}

// Close frees the model now.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.timer != nil {
		e.timer.Stop()
	}
	e.unload()
}

func (e *Engine) load() error {
	if e.rec != nil {
		return nil
	}
	if !Parakeet.Installed(e.Root) {
		return errors.New("the speech model is not downloaded; download it in Settings → AI")
	}
	dir := Parakeet.Dir(e.Root)
	c := sherpa.OfflineRecognizerConfig{DecodingMethod: "greedy_search"}
	c.FeatConfig = sherpa.FeatureConfig{SampleRate: SampleRate, FeatureDim: 80}
	c.ModelConfig.Transducer = sherpa.OfflineTransducerModelConfig{
		Encoder: filepath.Join(dir, encoderFile),
		Decoder: filepath.Join(dir, decoderFile),
		Joiner:  filepath.Join(dir, joinerFile),
	}
	c.ModelConfig.Tokens = filepath.Join(dir, tokensFile)
	c.ModelConfig.ModelType = "nemo_transducer"
	c.ModelConfig.Provider = "cpu"
	// Four threads is near the fastest on a laptop; more only adds heat.
	c.ModelConfig.NumThreads = max(1, min(4, runtime.NumCPU()/2))
	rec := sherpa.NewOfflineRecognizer(&c)
	if rec == nil {
		return errors.New("the speech model did not load; download it again in Settings → AI")
	}
	e.rec = rec
	return nil
}

func (e *Engine) unload() {
	if e.rec != nil {
		sherpa.DeleteOfflineRecognizer(e.rec)
		e.rec = nil
	}
}

// touch restarts the idle countdown. Called with mu held.
func (e *Engine) touch() {
	e.uses++
	use := e.uses
	if e.timer != nil {
		e.timer.Stop()
	}
	e.timer = time.AfterFunc(e.Idle, func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.uses == use {
			e.unload()
		}
	})
}
