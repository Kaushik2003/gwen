package voice

import (
	"bytes"
	_ "embed"
	"errors"
	"os"
	"path/filepath"

	sherpa "github.com/k2-fsa/sherpa-onnx-go-linux"
)

// sileroVAD is Silero VAD v4 (MIT, the Silero Team), as sherpa-onnx ships it:
// 0.6 MB, so it comes with the program rather than as a download.
//
//go:embed silero_vad.onnx
var sileroVAD []byte

// vadFile is where the VAD is written under the models directory, for
// sherpa-onnx, which only loads from a file.
const vadFile = "silero_vad.onnx"

// Speech detection, as Silero hears it: the upstream defaults, which did best
// on recorded dictation, quiet or noisy.
const (
	vadWindow     = 512  // samples Silero judges at a time, 32 ms
	vadThreshold  = 0.5  // the probability of speech that counts as speech
	vadMinSpeech  = 0.25 // seconds of speech before it counts, so a click is not speech
	vadMaxSpeech  = 20   // seconds of speech before it ends at its next 0.1 s dip
	vadBufferSecs = 60   // seconds of audio it holds while speech goes on
)

// Span is speech the Detector heard, in samples of the audio fed to it: speech
// over [From, To), then quiet until at least Quiet.
type Span struct{ From, To, Quiet int }

// Detector finds speech in 16 kHz mono audio fed to it in order.
type Detector interface {
	Accept(samples []float32) // the next samples
	Speaking() bool           // speech has started and not yet ended
	Ended() []Span            // the speech that ended since the last call
	Flush()                   // ends the speech going on, at the end of the audio
	Close()
}

// VAD is a Detector that is Silero VAD, through sherpa-onnx. Speech ends after
// pause seconds of quiet.
type VAD struct {
	v     *sherpa.VoiceActivityDetector
	fed   int
	ended []Span
}

// NewVAD loads Silero VAD, writing it into the models directory root first.
func NewVAD(root string) (*VAD, error) {
	path := filepath.Join(root, vadFile)
	if b, err := os.ReadFile(path); err != nil || !bytes.Equal(b, sileroVAD) {
		if err := writeAtomic(path, sileroVAD); err != nil {
			return nil, err
		}
	}
	c := sherpa.VadModelConfig{SampleRate: SampleRate, NumThreads: 1, Provider: "cpu"}
	c.SileroVad = sherpa.SileroVadModelConfig{
		Model:              path,
		Threshold:          vadThreshold,
		MinSilenceDuration: pause,
		MinSpeechDuration:  vadMinSpeech,
		MaxSpeechDuration:  vadMaxSpeech,
		WindowSize:         vadWindow,
	}
	v := sherpa.NewVoiceActivityDetector(&c, vadBufferSecs)
	if v == nil {
		return nil, errors.New("the voice activity detector did not load")
	}
	return &VAD{v: v}, nil
}

// Accept feeds samples a window at a time, so the quiet after each span is
// known to the window.
func (d *VAD) Accept(samples []float32) {
	for len(samples) > 0 {
		n := min(vadWindow, len(samples))
		d.v.AcceptWaveform(samples[:n])
		d.fed += n
		samples = samples[n:]
		d.take()
	}
}

func (d *VAD) Speaking() bool { return d.v.IsSpeech() }

func (d *VAD) Ended() []Span {
	out := d.ended
	d.ended = nil
	return out
}

func (d *VAD) Flush() {
	d.v.Flush()
	d.take()
}

func (d *VAD) Close() { sherpa.DeleteVoiceActivityDetector(d.v) }

// take moves the speech sherpa ended into ended.
func (d *VAD) take() {
	for !d.v.IsEmpty() {
		s := d.v.Front()
		d.v.Pop()
		d.ended = append(d.ended, Span{From: s.Start, To: s.Start + len(s.Samples), Quiet: d.fed})
	}
}

// writeAtomic writes b to path through a temporary file beside it.
func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
