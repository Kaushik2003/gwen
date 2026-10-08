package voice

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
)

// Recording limits.
const (
	SampleRate = 16000 // Hz, mono, what Parakeet takes
	MaxSeconds = 120   // a longer recording keeps only its first two minutes
	chunk      = SampleRate / 10
)

// Recorder is the PipeWire capture program.
const Recorder = "pw-record"

// Recording is a microphone capture in progress.
type Recording struct {
	stop func() error // asks the capture to end
	wait func() error // waits for it to end
	done chan struct{}

	mu      sync.Mutex
	samples []float32
	err     error
}

// Record captures the default microphone as 16 kHz mono through command,
// pw-record. level gets the loudness of each 100 ms, from 0 to 1 (see loudness).
func Record(command string, level func(float64)) (*Recording, error) {
	cmd := exec.Command(command, "--raw", "--rate", fmt.Sprint(SampleRate), "--channels", "1", "--format", "s16", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}
	wait := func() error {
		err := cmd.Wait()
		msg := strings.TrimSpace(stderr.String())
		switch {
		case err == nil, !cmd.ProcessState.Exited():
			return nil // done, or ended by our signal
		case msg != "":
			return fmt.Errorf("%s: %s", command, msg)
		}
		return fmt.Errorf("%s: %w", command, err)
	}
	return newRecording(out, func() error { return cmd.Process.Signal(os.Interrupt) }, wait, level), nil
}

func newRecording(r io.Reader, stop, wait func() error, level func(float64)) *Recording {
	rec := &Recording{stop: stop, wait: wait, done: make(chan struct{})}
	go rec.read(r, level)
	return rec
}

// read collects samples until the capture ends, reporting each chunk's level.
func (rec *Recording) read(r io.Reader, level func(float64)) {
	defer close(rec.done)
	buf := make([]byte, 2*chunk)
	for {
		n, err := io.ReadFull(r, buf)
		if n >= 2 {
			samples, rms := decode(buf[:n&^1])
			rec.mu.Lock()
			if room := MaxSeconds*SampleRate - len(rec.samples); room > 0 {
				rec.samples = append(rec.samples, samples[:min(room, len(samples))]...)
			}
			rec.mu.Unlock()
			level(loudness(rms))
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				rec.mu.Lock()
				rec.err = err
				rec.mu.Unlock()
			}
			return
		}
	}
}

// Since returns a copy of the samples from i on, as captured so far.
func (rec *Recording) Since(i int) []float32 {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if i >= len(rec.samples) {
		return nil
	}
	return slices.Clone(rec.samples[i:])
}

// Stop ends the capture and returns what it heard.
func (rec *Recording) Stop() ([]float32, error) {
	select {
	case <-rec.done: // the capture already ended, as when pw-record fails
	default:
		_ = rec.stop()
	}
	<-rec.done
	werr := rec.wait()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.samples) == 0 {
		return nil, errors.Join(werr, rec.err)
	}
	return rec.samples, nil
}

// decode turns 16-bit little-endian PCM into samples in [-1, 1] and their RMS.
func decode(b []byte) ([]float32, float64) {
	out := make([]float32, len(b)/2)
	var sum float64
	for i := range out {
		s := float32(int16(binary.LittleEndian.Uint16(b[2*i:]))) / 32768
		out[i] = s
		sum += float64(s) * float64(s)
	}
	if len(out) == 0 {
		return out, 0
	}
	return out, math.Sqrt(sum / float64(len(out)))
}

// loudness maps an RMS onto 0–1 across -40 to -10 dBFS: a laptop mic's fan
// and room noise sit near -35, speech near -20, so talking stands out.
func loudness(rms float64) float64 {
	if rms <= 0 {
		return 0
	}
	return max(0, min(1, (20*math.Log10(rms)+40)/30))
}
