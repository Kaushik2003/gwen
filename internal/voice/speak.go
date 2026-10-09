package voice

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go-linux"
)

// SpeechRate is the sample rate every voice speaks at, in Hz, mono: Kokoro's
// own, and what Fish is asked for. A voice from another service must give
// samples at this rate too, so it plays through the same player.
const SpeechRate = 24000

// Kokoro is hexgrad's Kokoro 82M v0.19, English, as sherpa-onnx: eleven
// voices, about half a gigabyte of memory while loaded and three times faster
// than real time on four laptop cores. Its int8 build is a third the download
// but no faster than real time, too slow to talk with. Licence Apache-2.0.
var Kokoro = Model{
	Name: "kokoro-en-v0_19",
	URL:  "https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/kokoro-en-v0_19.tar.bz2",
	Size: 319_625_534,
	Files: map[string]string{
		kokoroModel:  "10ff414106a038ce7e9e0126c6461e4dc8a86efaa89dc91d2009d69fe635e339",
		kokoroVoices: "a372c67b056ef0b695c375d39b99630d23fb07ad4c8d87aa32a19a62fca523ad",
		tokensFile:   "4f31c71282d14af4e926cd12462078fe9d20d00c589e63fe2750a8f56d6d7f7b",
	},
	Trees: []string{kokoroData},
	Sum:   "912804855a04745fa77a30be545b3f9a5d15c4d66db00b88cbcd4921df605ac7",
}

// The files of a Kokoro voice, besides its tokens.
const (
	kokoroModel  = "model.onnx"
	kokoroVoices = "voices.bin"
	kokoroData   = "espeak-ng-data" // how words are said, for espeak-ng
)

// Voices are Kokoro's speakers by name, in speaker id order: a is American
// and b British, f female and m male.
var Voices = []string{"af", "af_bella", "af_nicole", "af_sarah", "af_sky", "am_adam", "am_michael", "bf_emma", "bf_isabella", "bm_george", "bm_lewis"}

// DefaultVoice is the voice for a name Voices does not have.
const DefaultVoice = "af_bella"

// voiceID is the speaker id of a voice name.
func voiceID(name string) int {
	if i := slices.Index(Voices, name); i >= 0 {
		return i
	}
	return slices.Index(Voices, DefaultVoice)
}

// Speaker says text with Kokoro from the models directory Root, loaded on
// first use and freed after Idle without use, as Engine is.
type Speaker struct {
	Root string
	Idle time.Duration

	mu    sync.Mutex
	tts   *sherpa.OfflineTts
	timer *time.Timer
	uses  int
}

// Say speaks text in a voice of Voices at speed, 1 for normal, handing chunk
// the samples, at SpeechRate, a sentence at a time as they are made. It stops
// early when chunk returns false or ctx ends.
func (s *Speaker) Say(ctx context.Context, text, voice string, speed float32, chunk func(samples []float32) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.touch()
	if err := s.load(); err != nil {
		return err
	}
	s.tts.GenerateWithCallback(text, voiceID(voice), speed, func(samples []float32) bool {
		return ctx.Err() == nil && chunk(samples)
	})
	return ctx.Err()
}

// Close frees the voice now.
func (s *Speaker) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.unload()
}

func (s *Speaker) load() error {
	if s.tts != nil {
		return nil
	}
	if !Kokoro.Installed(s.Root) {
		return errors.New("the voice is not downloaded; download it in Settings → AI")
	}
	dir := Kokoro.Dir(s.Root)
	c := sherpa.OfflineTtsConfig{MaxNumSentences: 1} // a chunk per sentence, so speech starts soon
	c.Model.Kokoro = sherpa.OfflineTtsKokoroModelConfig{
		Model:       filepath.Join(dir, kokoroModel),
		Voices:      filepath.Join(dir, kokoroVoices),
		Tokens:      filepath.Join(dir, tokensFile),
		DataDir:     filepath.Join(dir, kokoroData),
		LengthScale: 1,
	}
	c.Model.Provider = "cpu"
	c.Model.NumThreads = max(1, min(4, runtime.NumCPU()/2))
	tts := sherpa.NewOfflineTts(&c)
	if tts == nil {
		return errors.New("the voice did not load; download it again in Settings → AI")
	}
	s.tts = tts
	return nil
}

func (s *Speaker) unload() {
	if s.tts != nil {
		sherpa.DeleteOfflineTts(s.tts)
		s.tts = nil
	}
}

// touch restarts the idle countdown. Called with mu held.
func (s *Speaker) touch() {
	s.uses++
	use := s.uses
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.Idle, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.uses == use {
			s.unload()
		}
	})
}

var (
	markup = regexp.MustCompile("[*_`#>|~]+")                  // Markdown marks, which are not said
	bullet = regexp.MustCompile(`(?m)^\s*(?:[-•]|\d+[.)])\s+`) // a list item's mark
	link   = regexp.MustCompile(`https?://\S+`)                // a web address, which no one wants read out
	dash   = regexp.MustCompile(`\s*[–—]\s*`)                  // a dash between words is a pause
	spaces = regexp.MustCompile(`[ \t]*\n[ \t\n]*|[ \t]{2,}`)  // runs of space, and line breaks
	ends   = regexp.MustCompile(`([^.!?:;,\s])\s*\n`)          // a line without a stop gets one, so it is a sentence
)

// Readable is text as it should be said: without Markdown marks, list marks,
// or web addresses, a line break a sentence's end, and dashes a pause.
func Readable(text string) string {
	text = link.ReplaceAllString(text, "the link")
	text = bullet.ReplaceAllString(text, "")
	text = markup.ReplaceAllString(text, "")
	text = dash.ReplaceAllString(text, ", ")
	text = ends.ReplaceAllString(strings.TrimSpace(text)+"\n", "$1.\n")
	return strings.TrimSpace(spaces.ReplaceAllString(text, " "))
}
