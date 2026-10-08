package voice

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// feed is a recording whose audio arrives as the test says.
type feed struct {
	audio []float32
	avail int
	err   error
}

func (f *feed) Since(i int) []float32 {
	if i >= f.avail {
		return nil
	}
	return append([]float32(nil), f.audio[i:f.avail]...)
}

func (f *feed) Stop() ([]float32, error) { return f.audio[:f.avail], f.err }

// tone is phrase k, or quiet for k < 0: a sine whose loudness names it.
func tone(k int, seconds float64) []float32 {
	amp := 0.002 // about -57 dBFS: a quiet room
	if k >= 0 {
		amp = 0.1 + 0.05*float64(k)
	}
	out := make([]float32, int(seconds*SampleRate))
	for i := range out {
		out[i] = float32(amp * math.Sin(float64(i)*0.3))
	}
	return out
}

// namer "transcribes" by naming the phrases it hears, in order, and keeps
// the length of each piece of audio it was given. A phrase is heard in two
// chunks running, as a chunk across the edge of one can sound like another.
type namer struct {
	calls  []int
	starts []float64 // the loudness each piece starts with, dBFS
}

func (n *namer) Transcribe(samples []float32) (string, error) {
	n.calls = append(n.calls, len(samples))
	n.starts = append(n.starts, dbfs(samples[:min(len(samples), vadWindow)]))
	var words []string
	last := ""
	for i := 0; i+chunk <= len(samples); i += chunk {
		db, heard := dbfs(samples[i:i+chunk]), ""
		for k := range 5 {
			if want := 20 * math.Log10((0.1+0.05*float64(k))/math.Sqrt2); math.Abs(db-want) < 0.5 {
				heard = fmt.Sprintf("p%d", k)
			}
		}
		if heard != "" && heard == last && (len(words) == 0 || words[len(words)-1] != heard) {
			words = append(words, heard)
		}
		last = heard
	}
	return strings.Join(words, " "), nil
}

// dbfs is the loudness of samples, -120 for digital silence.
func dbfs(samples []float32) float64 {
	var sum float64
	for _, v := range samples {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return -120
	}
	return 20 * math.Log10(math.Sqrt(sum/float64(len(samples))))
}

// loud is a Detector that hears speech in each window louder than a quiet
// room, and ends it after quiet seconds of quiet, as Silero does.
type loud struct {
	quiet    float64
	buf      []float32
	at       int // samples judged
	from, to int // the speech going on, from -1 for none
	ended    []Span
	closed   bool
}

func newLoud() *loud { return &loud{quiet: pause, from: -1} }

func (d *loud) Accept(samples []float32) {
	d.buf = append(d.buf, samples...)
	for ; len(d.buf) >= vadWindow; d.buf, d.at = d.buf[vadWindow:], d.at+vadWindow {
		switch {
		case dbfs(d.buf[:vadWindow]) > -40:
			if d.from < 0 {
				d.from = d.at
			}
			d.to = d.at + vadWindow
		case d.from >= 0 && d.at+vadWindow-d.to >= int(d.quiet*SampleRate):
			d.ended = append(d.ended, Span{d.from, d.to, d.at + vadWindow})
			d.from = -1
		}
	}
}

func (d *loud) Speaking() bool { return d.from >= 0 }

func (d *loud) Ended() []Span {
	out := d.ended
	d.ended = nil
	return out
}

func (d *loud) Flush() {
	if d.from >= 0 {
		end := d.at + len(d.buf)
		d.ended = append(d.ended, Span{d.from, end, end})
		d.from = -1
	}
}

func (d *loud) Close() { d.closed = true }

// session is a Session whose loop never ticks: the test steps it.
func session(src Source, eng Transcriber, onText func(string)) *Session {
	return sessionWith(src, eng, newLoud(), onText)
}

func sessionWith(src Source, eng Transcriber, vad Detector, onText func(string)) *Session {
	s := newSession(src, eng, vad, onText)
	go s.run(time.Hour)
	return s
}

// play feeds the audio 200 ms at a time, stepping after each, as Listen does.
func play(s *Session, f *feed) {
	for f.avail < len(f.audio) {
		f.avail = min(len(f.audio), f.avail+SampleRate/5)
		s.step()
	}
}

func concat(parts ...[]float32) []float32 {
	var out []float32
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestLiveTextFollowsThePhrases(t *testing.T) {
	t.Parallel()
	f := &feed{audio: concat(tone(-1, 0.3), tone(0, 1.5), tone(-1, 1), tone(1, 2), tone(-1, 1), tone(2, 1))}
	n := &namer{}
	var shown []string
	s := session(f, n, func(text string) { shown = append(shown, text) })
	play(s, f)
	require.Equal(t, []string{"p0", "p0 p1", "p0 p1 p2"}, shown, "each phrase shows while it is spoken")
	before := len(n.calls)
	text, err := s.Stop()
	require.NoError(t, err)
	require.Equal(t, "p0 p1 p2", text, "every phrase once: no cut split one")
	require.Len(t, n.calls, before+1)
	require.Less(t, n.calls[before], 2*SampleRate, "stop only transcribes the last phrase")
}

func TestLiveCutsWithinTheQuietItKnows(t *testing.T) {
	t.Parallel()
	// Speech that went on too long ends at a dip of 0.1 s, so the cut must
	// fall within that, not mid-way through a usual pause.
	f := &feed{audio: concat(tone(-1, 0.3), tone(0, 1.5), tone(-1, 0.2), tone(1, 1.5), tone(-1, 1))}
	n := &namer{}
	vad := newLoud()
	vad.quiet = 0.1
	s := sessionWith(f, n, vad, func(string) {})
	play(s, f)
	text, err := s.Stop()
	require.NoError(t, err)
	require.Equal(t, "p0 p1", text)
	for _, db := range n.starts {
		require.Less(t, db, -40.0, "each piece starts in quiet: no cut fell in speech")
	}
}

func TestLiveClosesTheDetector(t *testing.T) {
	t.Parallel()
	for _, end := range []func(*Session){func(s *Session) { _, _ = s.Stop() }, (*Session).Cancel} {
		f := &feed{audio: concat(tone(-1, 0.3), tone(0, 1))}
		vad := newLoud()
		s := sessionWith(f, &namer{}, vad, func(string) {})
		play(s, f)
		end(s)
		require.True(t, vad.closed)
	}
}

func TestLiveCutsUnbrokenSpeech(t *testing.T) {
	t.Parallel()
	f := &feed{audio: concat(tone(-1, 0.5), tone(0, 70))}
	n := &namer{}
	s := session(f, n, func(string) {})
	play(s, f)
	text, err := s.Stop()
	require.NoError(t, err)
	require.NotEmpty(t, text)
	for _, c := range n.calls {
		require.LessOrEqual(t, c, maxPhrase, "no piece is longer than 30 s")
	}
}

func TestLiveTranscribesNoSilence(t *testing.T) {
	t.Parallel()
	f := &feed{audio: tone(-1, 10)}
	n := &namer{}
	s := session(f, n, func(string) { t.Error("silence shows no text") })
	play(s, f)
	text, err := s.Stop()
	require.NoError(t, err)
	require.Empty(t, text)
	require.Empty(t, n.calls, "nothing is transcribed without speech, so noise is not misheard")
}

func TestLiveCancelTranscribesNothingMore(t *testing.T) {
	t.Parallel()
	f := &feed{audio: concat(tone(-1, 0.3), tone(0, 1))}
	n := &namer{}
	s := session(f, n, func(string) {})
	play(s, f)
	before := len(n.calls)
	s.Cancel()
	require.Len(t, n.calls, before)
}

func TestLiveStopReportsAFailedRecording(t *testing.T) {
	t.Parallel()
	boom := errors.New("pw-record: no PipeWire")
	s := session(&feed{err: boom}, &namer{}, func(string) {})
	_, err := s.Stop()
	require.ErrorIs(t, err, boom)
}
