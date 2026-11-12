// Command gwen-stt turns speech into text for gwend (plan.md 3.14.10). It
// links the speech model, which gwend itself never does, and lives only
// while you talk from the phone: gwend starts it on first use and closes
// its input after three minutes unused.
//
// Each request on stdin is a 4-byte little-endian length followed by that
// many bytes of 16 kHz mono 16-bit little-endian PCM. Each reply on stdout
// is one JSON line: {"text": "..."} or {"error": "..."}.
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/kzark/gwen/internal/voice"
)

// maxRequest is 60 seconds of audio, the most gwend sends.
const maxRequest = 60 * voice.SampleRate * 2

func main() {
	models := flag.String("models", "", "the speech models folder")
	flag.Parse()
	if *models == "" {
		fmt.Fprintln(os.Stderr, "gwen-stt: --models is required")
		os.Exit(2)
	}
	if err := serve(os.Stdin, os.Stdout, &voice.Engine{Root: *models, Idle: 3 * time.Minute}); err != nil {
		fmt.Fprintln(os.Stderr, "gwen-stt:", err)
		os.Exit(1)
	}
}

type transcriber interface {
	Transcribe(samples []float32) (string, error)
}

type reply struct {
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

// serve answers requests until stdin closes.
func serve(in io.Reader, out io.Writer, eng transcriber) error {
	r := bufio.NewReaderSize(in, 1<<20)
	enc := json.NewEncoder(out)
	for {
		var size [4]byte
		if _, err := io.ReadFull(r, size[:]); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		n := binary.LittleEndian.Uint32(size[:])
		if n > maxRequest || n%2 != 0 {
			return fmt.Errorf("a request of %d bytes is not audio gwend sends", n)
		}
		pcm := make([]byte, n)
		if _, err := io.ReadFull(r, pcm); err != nil {
			return err
		}
		samples := make([]float32, n/2)
		for i := range samples {
			samples[i] = float32(int16(binary.LittleEndian.Uint16(pcm[2*i:]))) / 32768
		}
		text, err := eng.Transcribe(samples)
		res := reply{Text: text}
		if err != nil {
			res = reply{Error: err.Error()}
		}
		if err := enc.Encode(res); err != nil {
			return err
		}
	}
}
