package voice

import (
	"slices"
	"strings"
	"sync"
	"time"
)

// Live dictation.
const (
	pause        = 0.5             // seconds of quiet that end a phrase; the gaps between words are shorter
	maxPhrase    = 30 * SampleRate // a phrase is cut by 30 s, should the speaker never pause
	partialEvery = SampleRate / 2  // the phrase being spoken is heard again after 0.5 s more of it
	lead         = SampleRate      // the quiet kept before speech, which is found a moment after it starts
)

// Transcriber turns 16 kHz mono samples into text.
type Transcriber interface {
	Transcribe(samples []float32) (string, error)
}

// Source is a recording in progress, such as a Recording.
type Source interface {
	Since(i int) []float32    // the samples from i on, so far
	Stop() ([]float32, error) // ends it, returning every sample
}

// Session is live dictation. Its Detector finds the pauses between phrases,
// and it transcribes each phrase once its pause comes, cut mid-way through the
// pause, and the phrase still being spoken again every half second, so the
// text follows the speaker and Stop only has the last phrase left to do.
// Audio without speech is never transcribed, so noise is not heard as words.
type Session struct {
	src    Source
	eng    Transcriber
	vad    Detector
	onText func(text string)

	samples   []float32
	cut       int      // the sample the phrase being spoken starts at
	done      []string // the text of the phrases before cut
	partial   string   // the phrase being spoken, as last heard
	partialAt int      // len(samples) when it was heard
	shown     string
	err       error

	halt   sync.Once
	stop   chan struct{}
	exited chan struct{}
}

// Listen transcribes src through eng while it records, finding its phrases
// with vad, which it closes when done, and calling onText with the whole text
// so far each time it changes.
func Listen(src Source, eng Transcriber, vad Detector, onText func(text string)) *Session {
	s := newSession(src, eng, vad, onText)
	go s.run(200 * time.Millisecond)
	return s
}

func newSession(src Source, eng Transcriber, vad Detector, onText func(string)) *Session {
	return &Session{src: src, eng: eng, vad: vad, onText: onText, stop: make(chan struct{}), exited: make(chan struct{})}
}

func (s *Session) run(every time.Duration) {
	defer close(s.exited)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.step()
		}
	}
}

// Stop ends the recording and returns the whole text, transcribing the last
// phrase.
func (s *Session) Stop() (string, error) {
	s.end()
	defer s.vad.Close()
	all, err := s.src.Stop()
	if len(all) == 0 {
		return "", err
	}
	s.vad.Accept(all[min(len(s.samples), len(all)):])
	s.samples = all
	s.vad.Flush()
	s.hear()
	return join(s.done), s.err
}

// Cancel ends the recording and drops it.
func (s *Session) Cancel() {
	s.end()
	s.vad.Close()
	_, _ = s.src.Stop()
}

// end stops the loop and waits for a transcription in flight.
func (s *Session) end() {
	s.halt.Do(func() {
		close(s.stop)
		<-s.exited
	})
}

// step takes in the new audio, transcribes each phrase that ended, and the
// phrase being spoken again once enough of it is new.
func (s *Session) step() {
	n := len(s.samples)
	s.samples = append(s.samples, s.src.Since(n)...)
	s.vad.Accept(s.samples[n:])
	s.hear()
	switch {
	case !s.vad.Speaking():
		// Quiet since the last phrase: keep only a little of it.
		s.cut = max(s.cut, len(s.samples)-lead)
		s.partialAt = s.cut
	case len(s.samples)-s.partialAt >= partialEvery:
		s.partial = s.transcribe(s.cut, len(s.samples))
		s.partialAt = len(s.samples)
	}
	if text := join(append(slices.Clone(s.done), s.partial)); text != s.shown {
		s.shown = text
		s.onText(text)
	}
}

// hear ends the phrase mid-way through each pause after speech, so each side
// keeps some quiet and a word trailing off is not cut, or where it is when it
// has gone on too long.
func (s *Session) hear() {
	for _, sp := range s.vad.Ended() {
		if sp.To > s.cut {
			s.commit(min(len(s.samples), (sp.To+sp.Quiet)/2))
		}
	}
	if len(s.samples)-s.cut >= maxPhrase && s.vad.Speaking() {
		s.commit(len(s.samples))
	}
}

// commit transcribes the phrase before sample end and starts the next there.
func (s *Session) commit(end int) {
	if text := s.transcribe(s.cut, end); text != "" {
		s.done = append(s.done, text)
	}
	s.cut, s.partial, s.partialAt = end, "", end
}

func (s *Session) transcribe(from, to int) string {
	text, err := s.eng.Transcribe(s.samples[from:to])
	if err != nil && s.err == nil {
		s.err = err
	}
	return text
}

func join(parts []string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}
