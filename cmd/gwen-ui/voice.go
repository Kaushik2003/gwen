package main

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/kzark/gwen/internal/voice"
)

// dictation is the dashboard's voice input (internal/voice): Parakeet under
// the data directory, one live dictation at a time, and the engine that
// turns it into text as you talk, all on this computer.
type dictation struct {
	root     string // the models directory
	lookPath func(command string) (string, error)
	http     *http.Client
	emit     func(name string, data any) // to the frontend
	engine   transcriber
	record   func(level func(float64)) (voice.Source, error)
	detect   func() (voice.Detector, error) // finds the pauses between phrases

	mu         sync.Mutex
	session    *voice.Session
	installing bool
}

type transcriber interface {
	voice.Transcriber
	Warm() error
}

// VoiceStatus is what the mic button and Settings → AI show about voice input.
type VoiceStatus struct {
	Installed  bool `json:"installed"`
	Installing bool `json:"installing"`
	// Recorder is whether pw-record, which captures the microphone, is installed.
	Recorder   bool `json:"recorder"`
	Listening  bool `json:"listening"`
	DownloadMB int  `json:"download_mb"`
}

func (v *dictation) status() VoiceStatus {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, err := v.lookPath(voice.Recorder)
	return VoiceStatus{
		Installed:  voice.Parakeet.Installed(v.root),
		Installing: v.installing,
		Recorder:   err == nil,
		Listening:  v.session != nil,
		DownloadMB: int(voice.Parakeet.Size >> 20),
	}
}

func (v *dictation) install(ctx context.Context) error {
	v.mu.Lock()
	if v.installing {
		v.mu.Unlock()
		return errors.New("the speech model is already downloading")
	}
	v.installing = true
	v.mu.Unlock()
	v.emit("voice:changed", nil)
	err := voice.Parakeet.Install(ctx, v.http, v.root, func(done, total int64) {
		v.emit("voice:progress", map[string]int64{"done": done, "total": total})
	})
	v.mu.Lock()
	v.installing = false
	v.mu.Unlock()
	v.emit("voice:changed", nil)
	return err
}

func (v *dictation) remove() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.installing {
		return errors.New("wait for the download to finish")
	}
	if v.session != nil {
		return errors.New("stop listening first")
	}
	err := voice.Parakeet.Remove(v.root)
	v.emit("voice:changed", nil)
	return err
}

// start begins live dictation, loading the model meanwhile. It emits
// "voice:level", 0 to 1, ten times a second, and "voice:text", the whole
// text so far, each time it changes.
func (v *dictation) start() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	switch {
	case v.session != nil:
		return errors.New("already listening")
	case !voice.Parakeet.Installed(v.root):
		return errors.New("the speech model is not downloaded")
	}
	vad, err := v.detect()
	if err != nil {
		return err
	}
	src, err := v.record(func(l float64) { v.emit("voice:level", l) })
	if err != nil {
		vad.Close()
		return err
	}
	v.session = voice.Listen(src, v.engine, vad, func(text string) { v.emit("voice:text", text) })
	go v.engine.Warm() // a failure shows again, and is reported, on stop
	return nil
}

// stop ends listening and returns everything said, "" for nothing.
func (v *dictation) stop() (string, error) {
	if s := v.take(); s != nil {
		return s.Stop()
	}
	return "", nil
}

// cancel ends listening and drops what was said.
func (v *dictation) cancel() {
	if s := v.take(); s != nil {
		s.Cancel()
	}
}

func (v *dictation) take() *voice.Session {
	v.mu.Lock()
	defer v.mu.Unlock()
	s := v.session
	v.session = nil
	return s
}

// VoiceStatus reports whether voice input is ready, downloading, or in use.
func (a *App) VoiceStatus() VoiceStatus { return a.voice.status() }

// InstallVoice downloads the speech model, about 100 MB, once. It emits
// "voice:progress" {done, total} in bytes as it goes, and "voice:changed".
func (a *App) InstallVoice() error { return wrap(a.voice.install(a.ctx)) }

// RemoveVoice deletes the speech model.
func (a *App) RemoveVoice() error { return wrap(a.voice.remove()) }

// StartListening records the microphone until StopListening or
// CancelListening, transcribing as it goes (see dictation.start).
func (a *App) StartListening() error { return wrap(a.voice.start()) }

// StopListening ends the recording and returns what was said.
func (a *App) StopListening() (string, error) { return call(a.voice.stop()) }

// CancelListening ends the recording without transcribing it.
func (a *App) CancelListening() { a.voice.cancel() }
