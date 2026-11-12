package remote

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/wire"
)

// Voice from the phone (plan.md 3.14.10): the phone sends 16 kHz mono 16-bit
// PCM, and gwen-stt, a small helper that links the speech model, turns it
// into text. gwend starts the helper on first use and closes it after three
// minutes unused, so the model takes memory only while you talk.

const (
	sttIdle = 3 * time.Minute
	// MaxSpeech is 60 seconds of 16 kHz mono 16-bit PCM.
	MaxSpeech = 60 * 16000 * 2
)

type sttHelper struct {
	bin    string
	models string
	clk    clock.Clock

	mu    sync.Mutex
	cmd   *exec.Cmd
	in    io.WriteCloser
	out   *bufio.Reader
	timer *time.Timer
}

// available reports whether voice can work: the helper and the model exist.
func (h *sttHelper) available() bool {
	if h.bin == "" {
		return false
	}
	if _, err := os.Stat(h.bin); err != nil {
		return false
	}
	entries, err := os.ReadDir(h.models)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			if files, _ := os.ReadDir(filepath.Join(h.models, e.Name())); len(files) > 0 {
				return true
			}
		}
	}
	return false
}

func (h *sttHelper) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stop()
}

func (h *sttHelper) stop() {
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
	if h.cmd == nil {
		return
	}
	h.in.Close()
	done := make(chan struct{})
	cmd := h.cmd
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
	}
	h.cmd, h.in, h.out = nil, nil, nil
}

func (h *sttHelper) start() error {
	cmd := exec.Command(h.bin, "--models", h.models)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	h.cmd, h.in, h.out = cmd, in, bufio.NewReaderSize(out, 64<<10)
	return nil
}

// transcribe sends one recording and reads its text.
func (h *sttHelper) transcribe(ctx context.Context, pcm []byte) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cmd == nil {
		if err := h.start(); err != nil {
			return "", err
		}
	}
	if h.timer != nil {
		h.timer.Stop()
	}
	h.timer = time.AfterFunc(sttIdle, h.close)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(pcm)))
	type result struct {
		line []byte
		err  error
	}
	done := make(chan result, 1)
	in, out := h.in, h.out
	go func() {
		if _, err := in.Write(size[:]); err != nil {
			done <- result{err: err}
			return
		}
		if _, err := in.Write(pcm); err != nil {
			done <- result{err: err}
			return
		}
		line, err := out.ReadBytes('\n')
		done <- result{line, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			h.stop()
			return "", errors.New("the speech helper stopped")
		}
		var reply struct {
			Text  string `json:"text"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(r.line, &reply); err != nil {
			h.stop()
			return "", errors.New("the speech helper answered something Gwen can't read")
		}
		if reply.Error != "" {
			return "", errors.New(reply.Error)
		}
		return reply.Text, nil
	case <-ctx.Done():
		h.stop() // the helper is mid-request; start a clean one next time
		return "", ctx.Err()
	}
}

// Transcribe turns a recording from the phone into text for review. It
// never sends anything by itself.
func (s *Service) Transcribe(ctx context.Context, pcm []byte) (wire.CodeTranscription, error) {
	if len(pcm) == 0 || len(pcm)%2 != 0 {
		return wire.CodeTranscription{}, fail(wire.CodeInvalidRequest, "that recording is empty")
	}
	if len(pcm) > MaxSpeech {
		return wire.CodeTranscription{}, fail(wire.CodeInvalidRequest, "that recording is longer than 60 seconds")
	}
	if !s.stt.available() {
		return wire.CodeTranscription{}, fail(wire.CodeUnavailable, "Voice isn't set up on the laptop. Open Gwen › Settings › Voice.")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	text, err := s.stt.transcribe(ctx, pcm)
	if err != nil {
		return wire.CodeTranscription{}, fail(wire.CodeUnavailable, "Couldn't turn that into text: %v", err)
	}
	return wire.CodeTranscription{Text: text}, nil
}
