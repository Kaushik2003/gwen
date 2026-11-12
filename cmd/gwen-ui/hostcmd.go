package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/kzark/gwen/internal/config"
)

// `gwen-ui host <method> [args]` answers what Settings › AI needs about
// voice input and Gwen's voice without opening a window (plan.md 3.15).
// Gwen's screens live inside Gwen Terminal, whose mods can run a program but
// cannot link the speech models; gwen-ui links them for the Talk window, so
// it answers for them here too. Every answer is one JSON value on stdout; a
// failure is a message on stderr and exit status 1.
//
//	voice-status                           VoiceStatus, with progress while downloading
//	voice-install | voice-remove           the speech-to-text model
//	speech-status                          SpeechStatus, with progress while downloading
//	speech-install ID | speech-remove ID   a provider's voice model
//	speech-choose ID [VOICE]               the provider Gwen speaks with, and its voice
//	say PROVIDER VOICE SPEED TEXT          speaks, and returns when she has finished
//	stop-saying                            cuts off a say that is running

// hostProgress is a download under way, as the file beside the socket holds
// it for the next status call to read.
type hostProgress struct {
	Done  int64 `json:"done"`
	Total int64 `json:"total"`
	PID   int   `json:"pid"`
}

// hostFiles are the small files the host commands leave in the runtime
// directory to find each other: a download's progress, and who is speaking.
type hostFiles struct{ dir string }

func (f hostFiles) progress(what string) string {
	return filepath.Join(f.dir, "download-"+what+".json")
}
func (f hostFiles) saying() string { return filepath.Join(f.dir, "saying.pid") }

// alive reports whether pid is a running process.
func alive(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

// read is the progress of a download that is still running, nil for none.
func (f hostFiles) read(what string) *hostProgress {
	b, err := os.ReadFile(f.progress(what))
	if err != nil {
		return nil
	}
	var p hostProgress
	if json.Unmarshal(b, &p) != nil || !alive(p.PID) {
		os.Remove(f.progress(what))
		return nil
	}
	return &p
}

func (f hostFiles) write(what string, done, total int64) {
	b, _ := json.Marshal(hostProgress{Done: done, Total: total, PID: os.Getpid()})
	tmp := f.progress(what) + ".new"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, f.progress(what))
	}
}

// hostVoiceStatus is VoiceStatus with a running download's progress.
type hostVoiceStatus struct {
	VoiceStatus
	Progress *hostProgress `json:"progress"`
}

// hostSpeechStatus is SpeechStatus with each provider's running download.
type hostSpeechStatus struct {
	Player    bool                 `json:"player"`
	Providers []hostSpeechProvider `json:"providers"`
	Provider  string               `json:"provider"`
	Voices    map[string]string    `json:"voices"`
}

type hostSpeechProvider struct {
	SpeechProvider
	Progress *hostProgress `json:"progress"`
}

// runHost runs one host command and returns the exit status.
func runHost(args []string, stdout, stderr io.Writer) int {
	out, err := hostCommand(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(out); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func hostCommand(args []string) (any, error) {
	if len(args) == 0 {
		return nil, errors.New("gwen-ui host: which command?")
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(paths.RuntimeDir, 0o700); err != nil {
		return nil, err
	}
	files := hostFiles{dir: paths.RuntimeDir}
	models := filepath.Join(paths.DataDir, "models")
	h := &host{credDir: paths.CredentialsDir()}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// What a download reports goes to its progress file; what speech reports comes back on ended.
	var mu sync.Mutex
	what := ""
	ended := make(chan speechEnd, 1)
	emit := func(name string, data any) {
		switch name {
		case "voice:progress", "speech:progress":
			if p, ok := data.(map[string]int64); ok {
				mu.Lock()
				defer mu.Unlock()
				files.write(what, p["done"], p["total"])
			}
		case "speech:end":
			if e, ok := data.(speechEnd); ok {
				select {
				case ended <- e:
				default:
				}
			}
		}
	}
	need := func(n int) error {
		if len(args)-1 != n {
			return fmt.Errorf("gwen-ui host %s: expected %d argument(s), got %d", args[0], n, len(args)-1)
		}
		return nil
	}
	download := func(name string, run func() error) (any, error) {
		if files.read(name) != nil {
			return nil, errors.New("it is already downloading")
		}
		mu.Lock()
		what = name
		mu.Unlock()
		files.write(name, 0, 0)
		defer os.Remove(files.progress(name))
		return struct{}{}, run()
	}

	switch args[0] {
	case "voice-status":
		if err := need(0); err != nil {
			return nil, err
		}
		st := hostVoiceStatus{VoiceStatus: newDictation(models, emit).status(), Progress: files.read("voice")}
		st.Installing = st.Progress != nil
		return st, nil
	case "voice-install":
		if err := need(0); err != nil {
			return nil, err
		}
		return download("voice", func() error { return newDictation(models, emit).install(ctx) })
	case "voice-remove":
		if err := need(0); err != nil {
			return nil, err
		}
		if files.read("voice") != nil {
			return nil, errors.New("wait for the download to finish")
		}
		return struct{}{}, newDictation(models, emit).remove()
	case "speech-status":
		if err := need(0); err != nil {
			return nil, err
		}
		st := newSpeech(models, paths.CredentialsDir(), speechChoices(paths), h.hasCredential, emit).status()
		out := hostSpeechStatus{Player: st.Player, Providers: []hostSpeechProvider{}, Provider: st.Provider, Voices: st.Voices}
		for _, p := range st.Providers {
			hp := hostSpeechProvider{SpeechProvider: p, Progress: files.read("speech-" + p.ID)}
			hp.Installing = hp.Progress != nil
			out.Providers = append(out.Providers, hp)
		}
		return out, nil
	case "speech-install":
		if err := need(1); err != nil {
			return nil, err
		}
		return download("speech-"+args[1], func() error {
			return newSpeech(models, paths.CredentialsDir(), speechChoices(paths), h.hasCredential, emit).install(ctx, args[1])
		})
	case "speech-remove":
		if err := need(1); err != nil {
			return nil, err
		}
		if files.read("speech-"+args[1]) != nil {
			return nil, errors.New("wait for the download to finish")
		}
		return struct{}{}, newSpeech(models, paths.CredentialsDir(), speechChoices(paths), h.hasCredential, emit).remove(args[1])
	case "speech-choose":
		if len(args) != 2 && len(args) != 3 {
			return nil, errors.New("gwen-ui host speech-choose: give a provider, and maybe a voice")
		}
		voiceID := ""
		if len(args) == 3 {
			voiceID = args[2]
		}
		return struct{}{}, newSpeech(models, paths.CredentialsDir(), speechChoices(paths), h.hasCredential, emit).choose(args[1], voiceID)
	case "say":
		if err := need(4); err != nil {
			return nil, err
		}
		speed, err := strconv.ParseFloat(args[3], 64)
		if err != nil {
			return nil, fmt.Errorf("gwen-ui host say: the speed %q is not a number", args[3])
		}
		stopSaying(files) // one voice at a time
		sp := newSpeech(models, paths.CredentialsDir(), speechChoices(paths), h.hasCredential, emit)
		if _, err := sp.say(args[4], args[1], args[2], speed); err != nil {
			return nil, err
		}
		_ = os.WriteFile(files.saying(), []byte(strconv.Itoa(os.Getpid())), 0o600)
		defer os.Remove(files.saying())
		select {
		case e := <-ended:
			if e.Error != "" {
				return nil, errors.New(e.Error)
			}
		case <-ctx.Done():
			sp.stop()
			<-ended
		}
		return struct{}{}, nil
	case "stop-saying":
		if err := need(0); err != nil {
			return nil, err
		}
		stopSaying(files)
		return struct{}{}, nil
	}
	return nil, fmt.Errorf("gwen-ui host: there is no command %q", args[0])
}

// stopSaying ends a say that is running, if one is.
func stopSaying(files hostFiles) {
	b, err := os.ReadFile(files.saying())
	if err != nil {
		return
	}
	if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && alive(pid) && pid != os.Getpid() {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	os.Remove(files.saying())
}
