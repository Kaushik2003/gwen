package voice

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strings"
	"sync/atomic"
)

// Player is the PipeWire playback program, from the same package as Recorder.
const Player = "pw-play"

// Playback is speech playing through Player as it is written.
type Playback struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	stderr bytes.Buffer
	killed atomic.Bool
}

// Play starts command, pw-play, on raw 32-bit float mono samples at rate,
// named Gwen in the sound settings.
func Play(command string, rate int) (*Playback, error) {
	p := &Playback{}
	p.cmd = exec.Command(command, "--raw", "--rate", fmt.Sprint(rate), "--channels", "1", "--format", "f32",
		"--properties", `{ "media.name": "Gwen speaking", "application.name": "Gwen" }`, "-")
	p.cmd.Stderr = &p.stderr
	in, err := p.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	p.in = in
	if err := p.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}
	return p, nil
}

// Write queues samples, waiting while the player is a moment behind, so
// speech is made no faster than it is heard.
func (p *Playback) Write(samples []float32) error {
	b := make([]byte, 4*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(s))
	}
	_, err := p.in.Write(b)
	return err
}

// Close waits for what was written to finish playing.
func (p *Playback) Close() error {
	p.in.Close()
	err := p.cmd.Wait()
	if err == nil || p.killed.Load() {
		return nil
	}
	if msg := strings.TrimSpace(p.stderr.String()); msg != "" {
		return fmt.Errorf("%s: %s", p.cmd.Path, msg)
	}
	return fmt.Errorf("%s: %w", p.cmd.Path, err)
}

// Stop cuts the speech off now; Close then returns at once.
func (p *Playback) Stop() {
	if !p.killed.Swap(true) {
		p.cmd.Process.Kill()
	}
}
