package main

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/voice"
	"github.com/stretchr/testify/require"
)

// fakeSayer says each text as three chunks, waiting for next before each
// one after the first when next is set. Each chunk is its index, or chunk
// when that is set.
type fakeSayer struct {
	mu     sync.Mutex
	texts  []string
	voices []string
	next   chan struct{}
	chunk  []float32
}

func (f *fakeSayer) Say(_ context.Context, text, voice string, _ float32, chunk func([]float32) bool) error {
	f.mu.Lock()
	f.texts = append(f.texts, text)
	f.voices = append(f.voices, voice)
	f.mu.Unlock()
	for i := range 3 {
		if i > 0 && f.next != nil {
			<-f.next
		}
		samples := []float32{float32(i)}
		if f.chunk != nil {
			samples = f.chunk
		}
		if !chunk(samples) {
			return nil
		}
	}
	return nil
}

type fakeSink struct {
	mu      sync.Mutex
	written []float32
	closed  bool
	stopped bool
}

func (s *fakeSink) Write(samples []float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return errors.New("broken pipe")
	}
	s.written = append(s.written, samples...)
	return nil
}

func (s *fakeSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *fakeSink) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
}

// state is what a player was given: the samples, and whether it was closed
// or stopped.
func (s *fakeSink) state() ([]float32, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.written, s.closed, s.stopped
}

// fakePlayers are the players started, in order.
type fakePlayers struct {
	mu   sync.Mutex
	list []*fakeSink
}

func (p *fakePlayers) start() (sink, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := &fakeSink{}
	p.list = append(p.list, out)
	return out, nil
}

func (p *fakePlayers) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.list)
}

func (p *fakePlayers) at(i int) *fakeSink {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.list[i]
}

type event struct {
	name string
	data any
}

// newTestSpeech has a "kokoro" provider, its voice "installed" under a
// temporary directory, and an online "fish" one that needs a key, both
// saying through the returned fakeSayer, and sends every event to the
// returned channel.
func newTestSpeech(t *testing.T, installed bool) (*speech, *fakeSayer, *fakePlayers, chan event) {
	t.Helper()
	root := t.TempDir()
	if installed {
		dir := voice.Kokoro.Dir(root)
		for _, tree := range voice.Kokoro.Trees {
			require.NoError(t, os.MkdirAll(filepath.Join(dir, tree), 0o755))
		}
		for name := range voice.Kokoro.Files {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))
		}
	}
	events := make(chan event, 32)
	say := &fakeSayer{}
	players := &fakePlayers{}
	s := &speech{
		root:     root,
		lookPath: func(string) (string, error) { return "/usr/bin/pw-play", nil },
		hasKey:   func(string) bool { return false },
		emit:     func(name string, data any) { events <- event{name, data} },
		providers: []*speechProvider{
			{SpeechProvider: SpeechProvider{ID: "kokoro", Voices: []SpeechVoice{{"af_bella", "Bella"}, {"bf_emma", "Emma"}}}, engine: say, model: &voice.Kokoro},
			{SpeechProvider: SpeechProvider{ID: "fish", Name: "Fish Audio", Online: true, Key: "fish_audio_api_key", Voices: []SpeechVoice{{"gwen", "Gwen"}}}, engine: say},
		},
		play: players.start,
	}
	return s, say, players, events
}

// next is the next event other than "speech:level", which comes as time passes.
func next(t *testing.T, events chan event) event {
	t.Helper()
	for {
		select {
		case e := <-events:
			if e.name != "speech:level" {
				return e
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no event")
			return event{}
		}
	}
}

func TestSpeechSaysItAllThenEnds(t *testing.T) {
	s, say, players, events := newTestSpeech(t, true)
	id, err := s.say("**Done.**\n- Essay at ten", "kokoro", "af_bella", 1)
	require.NoError(t, err)
	require.Equal(t, event{"speech:start", id}, next(t, events))
	require.Equal(t, event{"speech:end", speechEnd{ID: id}}, next(t, events))
	require.Equal(t, []string{"Done. Essay at ten."}, say.texts, "the text is said as it reads")
	require.Equal(t, 1, players.count(), "one player for the whole of it")
	written, closed, _ := players.at(0).state()
	require.Equal(t, []float32{0, 1, 2}, written)
	require.True(t, closed, "it waits for the end to play")
	require.Zero(t, s.status().Speaking)
}

func TestSpeechStopCutsItOff(t *testing.T) {
	s, say, players, events := newTestSpeech(t, true)
	say.next = make(chan struct{})
	id, err := s.say("A long answer.", "kokoro", "af_bella", 1)
	require.NoError(t, err)
	require.Equal(t, event{"speech:start", id}, next(t, events))
	require.Equal(t, id, s.status().Speaking)
	s.stop()
	close(say.next)
	require.Equal(t, event{"speech:end", speechEnd{ID: id}}, next(t, events), "cut off is no failure")
	written, _, stopped := players.at(0).state()
	require.True(t, stopped)
	require.Equal(t, []float32{0}, written, "nothing more is played")
}

func TestSpeechNewTextCutsOffTheOld(t *testing.T) {
	s, say, players, events := newTestSpeech(t, true)
	say.next = make(chan struct{})
	first, err := s.say("First.", "kokoro", "af_bella", 1)
	require.NoError(t, err)
	require.Equal(t, event{"speech:start", first}, next(t, events))
	second, err := s.say("Second.", "kokoro", "af_bella", 1)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	_, _, stopped := players.at(0).state()
	require.True(t, stopped, "the first is cut off at once")
	close(say.next)
	var got []event
	for range 3 {
		got = append(got, next(t, events))
	}
	require.ElementsMatch(t, []event{{"speech:end", speechEnd{ID: first}}, {"speech:start", second}, {"speech:end", speechEnd{ID: second}}}, got,
		"the first ends as the second starts, in either order")
}

func TestSpeechNeedsTheVoiceAndThePlayer(t *testing.T) {
	s, _, _, _ := newTestSpeech(t, false)
	_, err := s.say("Hi.", "kokoro", "af_bella", 1)
	require.ErrorContains(t, err, "not downloaded")

	s, _, _, _ = newTestSpeech(t, true)
	s.lookPath = func(string) (string, error) { return "", errors.New("not found") }
	require.False(t, s.status().Player)
	_, err = s.say("Hi.", "kokoro", "af_bella", 1)
	require.ErrorContains(t, err, "pipewire-utils")

	_, err = s.say("**  **", "kokoro", "af_bella", 1)
	require.ErrorContains(t, err, "nothing to say")
}

func TestSpeechReportsAPlayerThatWouldNotStart(t *testing.T) {
	s, _, _, events := newTestSpeech(t, true)
	s.play = func() (sink, error) { return nil, errors.New("start pw-play: no such file") }
	id, err := s.say("Hi.", "kokoro", "af_bella", 1)
	require.NoError(t, err)
	require.Equal(t, event{"speech:end", speechEnd{ID: id, Error: "start pw-play: no such file"}}, next(t, events))
}

func TestSpeechProvidersSayWhatTheyNeed(t *testing.T) {
	s, _, _, _ := newTestSpeech(t, false)
	st := s.status()
	require.Len(t, st.Providers, 2)
	kokoro, fish := st.Providers[0], st.Providers[1]
	require.False(t, kokoro.Ready)
	require.False(t, kokoro.Installed)
	require.Equal(t, int(voice.Kokoro.Size>>20), kokoro.DownloadMB)
	require.Empty(t, kokoro.Key)
	require.False(t, fish.Ready, "no key yet")
	require.True(t, fish.Installed, "nothing to download")
	require.Zero(t, fish.DownloadMB)
	require.False(t, fish.HasKey)

	_, err := s.say("Hi.", "fish", "gwen", 1)
	require.ErrorContains(t, err, "no Fish Audio API key")
	s.hasKey = func(name string) bool { return name == "fish_audio_api_key" }
	require.True(t, s.status().Providers[1].Ready)
	require.True(t, s.status().Providers[1].HasKey)

	_, err = s.say("Hi.", "elevenlabs", "", 1)
	require.ErrorContains(t, err, `no voice provider "elevenlabs"`)
	require.ErrorContains(t, s.install(context.Background(), "fish"), "nothing to download")
}

func TestSpeechFallsBackToTheProvidersFirstVoice(t *testing.T) {
	s, say, _, events := newTestSpeech(t, true)
	id, err := s.say("Hi.", "kokoro", "bf_emma", 1)
	require.NoError(t, err)
	require.Equal(t, event{"speech:start", id}, next(t, events))
	require.Equal(t, event{"speech:end", speechEnd{ID: id}}, next(t, events))
	id, err = s.say("Hi.", "kokoro", "a voice of another provider", 1)
	require.NoError(t, err)
	require.Equal(t, event{"speech:start", id}, next(t, events))
	require.Equal(t, event{"speech:end", speechEnd{ID: id}}, next(t, events))
	require.Equal(t, []string{"bf_emma", "af_bella"}, say.voices)
}

// waitingSayer stands for an online voice that has not answered: it waits
// for its context to end.
type waitingSayer struct{ asked chan struct{} }

func (w waitingSayer) Say(ctx context.Context, _, _ string, _ float32, _ func([]float32) bool) error {
	close(w.asked)
	<-ctx.Done()
	return ctx.Err()
}

func TestSpeechStopEndsAWaitingRequest(t *testing.T) {
	s, _, players, events := newTestSpeech(t, true)
	asked := make(chan struct{})
	s.providers[0].engine = waitingSayer{asked}
	id, err := s.say("Hi.", "kokoro", "af_bella", 1)
	require.NoError(t, err)
	<-asked
	s.stop()
	require.Equal(t, event{"speech:end", speechEnd{ID: id}}, next(t, events), "stopped before a sound is no failure")
	require.Zero(t, players.count(), "nothing was played")
}

func TestVoiceMeterIsHowLoudEachStretchIs(t *testing.T) {
	per := int(voice.SpeechRate * levelEvery / time.Second)
	m := &voiceMeter{}
	m.write(make([]float32, per/2))
	_, ok := m.at(0)
	require.False(t, ok, "a stretch counts once it is whole")
	loud := make([]float32, per+per/2)
	for i := range loud {
		loud[i] = 0.5
	}
	m.write(loud)
	quiet, ok := m.at(0)
	require.True(t, ok)
	require.InDelta(t, voice.Loudness(0.5/math.Sqrt2), quiet, 1e-6, "half silence, half loud")
	full, ok := m.at(1)
	require.True(t, ok)
	require.InDelta(t, voice.Loudness(0.5), full, 1e-6)
	_, ok = m.at(2)
	require.False(t, ok)
}

func TestSpeechLevelsFollowTheVoiceUntilItEnds(t *testing.T) {
	s, say, _, events := newTestSpeech(t, true)
	say.next = make(chan struct{})
	say.chunk = make([]float32, voice.SpeechRate) // a second of sound
	for i := range say.chunk {
		say.chunk[i] = 0.5
	}
	id, err := s.say("Hello.", "kokoro", "af_bella", 1)
	require.NoError(t, err)
	require.Equal(t, event{"speech:start", id}, next(t, events))
	e := <-events
	require.Equal(t, "speech:level", e.name, "her loudness as she is heard")
	require.Equal(t, speechLevel{ID: id, Level: voice.Loudness(0.5)}, e.data)
	s.stop()
	close(say.next)
	for e := range events {
		if e.name == "speech:end" {
			break
		}
	}
	select {
	case e := <-events:
		t.Fatalf("%s after the end", e.name)
	case <-time.After(3 * levelEvery):
	}
}
