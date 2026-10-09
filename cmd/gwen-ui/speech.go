package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/voice"
)

// speech is Gwen's voice (internal/voice), from one of several providers,
// saying one thing at a time through PipeWire. Everything here works through
// sayer and the providers' descriptions, so swapping or adding a provider
// only touches speechProviders.
type speech struct {
	root      string // the models directory
	lookPath  func(command string) (string, error)
	hasKey    func(credential string) bool // whether a credential file is saved
	http      *http.Client
	emit      func(name string, data any) // to the frontend
	providers []*speechProvider           // the first is the default
	play      func() (sink, error)        // starts the player at voice.SpeechRate

	mu         sync.Mutex
	installing string // the provider whose model is downloading, "" for none
	saying     *utterance
	last       int // the latest utterance's id
}

// sayer turns text into speech in one of its voices at speed, 1 for normal,
// handing chunk mono samples at voice.SpeechRate as they are made. It stops
// early when chunk returns false or ctx ends. voice.Speaker and voice.Fish
// are sayers.
type sayer interface {
	Say(ctx context.Context, text, voice string, speed float32, chunk func(samples []float32) bool) error
}

// speechProvider is one way Gwen can speak, chosen in Settings → AI: its
// engine, a model to download before it speaks or an API key it needs, and
// how the picker describes it.
type speechProvider struct {
	SpeechProvider // the state fields are filled in by status
	engine         sayer
	model          *voice.Model // downloaded once before it speaks; nil for none
}

// speechProviders are the ways Gwen can speak, the default first. A new
// provider is an engine in internal/voice and an entry here; keys are read
// from credDir at each use.
func speechProviders(root, credDir string, hc *http.Client) []*speechProvider {
	// An online voice that has not started answering in this long is lost.
	online := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 20 * time.Second}}
	return []*speechProvider{
		{
			SpeechProvider: SpeechProvider{
				ID: "fish", Name: "Fish Audio", Online: true,
				About:  "Expressive voices from fish.audio, online, with an API key. What she says is sent to Fish Audio to be spoken.",
				Key:    config.CredFishAudioKey,
				KeyURL: "https://fish.audio/app/api-keys/",
				Voices: []SpeechVoice{
					{voice.FishVoice, "Gwen Stacy · playful"},
					{"933563129e564b19a115bedd57b7406a", "Sarah · soft"},
					{"536d3a5e000945adb7038665781a4aca", "Ethan · calm"},
					{"c5f56a6cc2ec4fa8920cb4c5889a3fb7", "Slax · smooth"},
				},
			},
			engine: &voice.Fish{HTTP: online, Key: func() (string, error) { return credentialLine(credDir, config.CredFishAudioKey) }},
		},
		{
			SpeechProvider: SpeechProvider{
				ID: "kokoro", Name: "On this computer",
				About: "Kokoro speaks offline after a one-time download, and only takes memory while she talks. Nothing leaves this computer.",
				// Kokoro's voices, the women first (voice.Voices).
				Voices: []SpeechVoice{
					{"af_bella", "Bella · American"}, {"af_sky", "Sky · American"}, {"af_nicole", "Nicole · American, soft"},
					{"af_sarah", "Sarah · American"}, {"af", "Blend · American"}, {"bf_emma", "Emma · British"},
					{"bf_isabella", "Isabella · British"}, {"am_adam", "Adam · American"}, {"am_michael", "Michael · American"},
					{"bm_george", "George · British"}, {"bm_lewis", "Lewis · British"},
				},
			},
			engine: &voice.Speaker{Root: root, Idle: 3 * time.Minute},
			model:  &voice.Kokoro,
		},
	}
}

// credentialLine reads a one-line credential; a missing file is "".
func credentialLine(dir, name string) (string, error) {
	key, err := config.ReadCredentialLine(dir, name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return key, err
}

// sink is speech playing as it is written, such as a voice.Playback.
type sink interface {
	Write(samples []float32) error
	Close() error // waits for the rest to play
	Stop()        // cuts it off
}

// utterance is one thing being said. Its player starts with the first sound,
// so a voice that fails before then plays nothing. Stopping it ends ctx, so
// an online voice stops waiting at once.
type utterance struct {
	id     int
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	out     sink
	stopped bool
}

// sink is the utterance's player, started now when it has none, and whether
// it just started; nil once stopped.
func (u *utterance) sink(play func() (sink, error)) (sink, bool, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.stopped {
		return nil, false, nil
	}
	if u.out != nil {
		return u.out, false, nil
	}
	out, err := play()
	if err != nil {
		return nil, false, err
	}
	u.out = out
	return out, true, nil
}

// player is the utterance's player, nil before the first sound.
func (u *utterance) player() sink {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.out
}

func (u *utterance) stop() {
	u.cancel()
	u.mu.Lock()
	defer u.mu.Unlock()
	u.stopped = true
	if u.out != nil {
		u.out.Stop()
	}
}

func (u *utterance) halted() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.stopped
}

// SpeechStatus is what the speaker buttons and Settings → AI show about
// Gwen's voice.
type SpeechStatus struct {
	// Player is whether pw-play, which plays every voice, is installed.
	Player    bool             `json:"player"`
	Speaking  int              `json:"speaking"`  // the utterance being said, 0 for none
	Providers []SpeechProvider `json:"providers"` // the first is the default
}

// SpeechProvider is a way Gwen can speak, as the frontend sees it.
type SpeechProvider struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	About  string        `json:"about"`  // a line for the picker
	Online bool          `json:"online"` // what she says goes to an online service
	Voices []SpeechVoice `json:"voices"` // the first is the default
	// DownloadMB is the size of a one-time download, 0 for none; Installed
	// whether it is done and Installing whether it is under way.
	DownloadMB int  `json:"download_mb"`
	Installed  bool `json:"installed"`
	Installing bool `json:"installing"`
	// Key is the credential file of the API key it needs, "" for none;
	// KeyURL where to get one, and HasKey whether it is saved.
	Key    string `json:"key"`
	KeyURL string `json:"key_url"`
	HasKey bool   `json:"has_key"`
	// Ready is whether it can speak, once the player is installed.
	Ready bool `json:"ready"`
}

// SpeechVoice is one of a provider's voices: the id it is asked for, and its
// name as a person reads it.
type SpeechVoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// speechEnd is the "speech:end" event: the utterance, and why it stopped
// short, "" when it was said or cut off.
type speechEnd struct {
	ID    int    `json:"id"`
	Error string `json:"error"`
}

func (s *speech) status() SpeechStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.lookPath(voice.Player)
	st := SpeechStatus{Player: err == nil}
	for _, p := range s.providers {
		v := p.SpeechProvider
		v.Installed = p.model == nil || p.model.Installed(s.root)
		if p.model != nil {
			v.DownloadMB = int(p.model.Size >> 20)
		}
		v.Installing = s.installing == p.ID
		v.HasKey = p.Key != "" && s.hasKey(p.Key)
		v.Ready = s.unready(p) == nil
		st.Providers = append(st.Providers, v)
	}
	if s.saying != nil {
		st.Speaking = s.saying.id
	}
	return st
}

// provider is the provider with id.
func (s *speech) provider(id string) (*speechProvider, error) {
	for _, p := range s.providers {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, fmt.Errorf("there is no voice provider %q", id)
}

// unready is why p cannot speak yet, nil when it can.
func (s *speech) unready(p *speechProvider) error {
	if p.model != nil && !p.model.Installed(s.root) {
		return errors.New("the voice is not downloaded; download it in Settings → AI")
	}
	if p.Key != "" && !s.hasKey(p.Key) {
		return fmt.Errorf("there is no %s API key; add it in Settings → AI", p.Name)
	}
	return nil
}

// downloadable is the provider with id when it has a model to download.
func (s *speech) downloadable(id string) (*speechProvider, error) {
	p, err := s.provider(id)
	if err != nil {
		return nil, err
	}
	if p.model == nil {
		return nil, fmt.Errorf("%s has nothing to download", p.Name)
	}
	return p, nil
}

func (s *speech) install(ctx context.Context, id string) error {
	p, err := s.downloadable(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.installing != "" {
		s.mu.Unlock()
		return errors.New("the voice is already downloading")
	}
	s.installing = p.ID
	s.mu.Unlock()
	s.emit("speech:changed", nil)
	err = p.model.Install(ctx, s.http, s.root, func(done, total int64) {
		s.emit("speech:progress", map[string]int64{"done": done, "total": total})
	})
	s.mu.Lock()
	s.installing = ""
	s.mu.Unlock()
	s.emit("speech:changed", nil)
	return err
}

func (s *speech) remove(id string) error {
	p, err := s.downloadable(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.installing != "" {
		return errors.New("wait for the download to finish")
	}
	if s.saying != nil {
		s.saying.stop()
	}
	err = p.model.Remove(s.root)
	s.emit("speech:changed", nil)
	return err
}

// say starts saying text with provider in one of its voices, or its default
// for another name, at speed, cutting off whatever was being said, and
// returns the utterance's id. It emits "speech:start" with the id when the
// sound starts, and "speech:end".
func (s *speech) say(text, provider, name string, speed float64) (int, error) {
	text = voice.Readable(text)
	if text == "" {
		return 0, errors.New("there is nothing to say")
	}
	p, err := s.provider(provider)
	if err != nil {
		return 0, err
	}
	if err := s.unready(p); err != nil {
		return 0, err
	}
	if _, err := s.lookPath(voice.Player); err != nil {
		return 0, errors.New("pw-play, which plays Gwen's voice, is not installed; install the pipewire-utils package")
	}
	if !slices.ContainsFunc(p.Voices, func(v SpeechVoice) bool { return v.ID == name }) {
		name = p.Voices[0].ID
	}
	s.mu.Lock()
	if s.saying != nil {
		s.saying.stop()
	}
	s.last++
	u := &utterance{id: s.last}
	u.ctx, u.cancel = context.WithCancel(context.Background())
	s.saying = u
	s.mu.Unlock()
	go s.run(u, p.engine, text, name, float32(min(2, max(0.5, speed))))
	return u.id, nil
}

func (s *speech) run(u *utterance, engine sayer, text, name string, speed float32) {
	defer u.cancel()
	var failed error
	meter := &voiceMeter{}
	quit, metered := make(chan struct{}), make(chan struct{})
	err := engine.Say(u.ctx, text, name, speed, func(samples []float32) bool {
		out, started, err := u.sink(s.play)
		if err != nil {
			failed = err
		}
		if out == nil {
			return false
		}
		if started {
			s.emit("speech:start", u.id)
			go s.follow(u.id, meter, time.Now(), quit, metered)
		}
		meter.write(samples)
		return out.Write(samples) == nil
	})
	err = errors.Join(err, failed)
	if out := u.player(); out != nil {
		err = errors.Join(err, out.Close())
		close(quit)
		<-metered // no level after the end
	}
	if u.halted() {
		err = nil // cut off on purpose: the player's complaint is no failure
	}
	s.mu.Lock()
	if s.saying == u {
		s.saying = nil
	}
	s.mu.Unlock()
	end := speechEnd{ID: u.id}
	if err != nil {
		slog.Warn("speech failed", "err", err)
		end.Error = err.Error()
	}
	s.emit("speech:end", end)
}

// levelEvery is how often "speech:level" says how loud she is, for her face
// to move with her voice.
const levelEvery = 50 * time.Millisecond

// speechLevel is the "speech:level" event: how loud the utterance is now,
// from 0 to 1.
type speechLevel struct {
	ID    int     `json:"id"`
	Level float64 `json:"level"`
}

// voiceMeter is how loud each levelEvery of an utterance is, as written to
// the player.
type voiceMeter struct {
	mu     sync.Mutex
	levels []float64
	sum    float64 // the squares of the stretch being filled
	n      int     // and its samples
}

func (m *voiceMeter) write(samples []float32) {
	per := int(voice.SpeechRate * levelEvery / time.Second)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range samples {
		m.sum += float64(x) * float64(x)
		if m.n++; m.n == per {
			m.levels = append(m.levels, voice.Loudness(math.Sqrt(m.sum/float64(per))))
			m.sum, m.n = 0, 0
		}
	}
}

// at is the level of the i-th stretch, and whether it was written yet.
func (m *voiceMeter) at(i int) (float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.levels) {
		return 0, false
	}
	return m.levels[i], true
}

// follow emits "speech:level" for the stretch being heard, by the time since
// the sound started: writes run a moment ahead of the speaker, so the level
// written last is not the one heard now. It ends with quit, closing done.
func (s *speech) follow(id int, m *voiceMeter, start time.Time, quit <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	tick := time.NewTicker(levelEvery)
	defer tick.Stop()
	for {
		select {
		case <-quit:
			return
		case now := <-tick.C:
			if l, ok := m.at(int(now.Sub(start) / levelEvery)); ok {
				s.emit("speech:level", speechLevel{ID: id, Level: l})
			}
		}
	}
}

// stop cuts off whatever is being said.
func (s *speech) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saying != nil {
		s.saying.stop()
	}
}

// SpeechStatus reports whether Gwen's voice is ready, downloading, or speaking.
func (a *App) SpeechStatus() SpeechStatus { return a.speech.status() }

// InstallSpeech downloads a provider's voice model once, such as Kokoro's
// 300 MB. It emits "speech:progress" {done, total} in bytes as it goes, and
// "speech:changed".
func (a *App) InstallSpeech(provider string) error { return wrap(a.speech.install(a.ctx, provider)) }

// RemoveSpeech deletes a provider's voice model.
func (a *App) RemoveSpeech(provider string) error { return wrap(a.speech.remove(provider)) }

// Say speaks text aloud with provider, one of SpeechStatus.Providers, in one
// of its voices at speed, 1 for normal, cutting off whatever was being said
// (see speech.say).
func (a *App) Say(text, provider, voice string, speed float64) (int, error) {
	return call(a.speech.say(text, provider, voice, speed))
}

// StopSaying cuts off whatever is being said.
func (a *App) StopSaying() { a.speech.stop() }
