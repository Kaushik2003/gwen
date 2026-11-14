package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// gwen-stt's side of the framing (plan.md 3.14.10), with a stand-in for the
// speech engine: each request on stdin is a 4-byte little-endian length and
// that much 16 kHz mono 16-bit PCM; each reply on stdout is one JSON line.

type fakeEngine struct {
	got [][]float32
}

func (f *fakeEngine) Transcribe(samples []float32) (string, error) {
	f.got = append(f.got, samples)
	switch {
	case len(samples) == 0:
		return "", nil
	case samples[0] < 0:
		return "", errors.New("the model failed")
	}
	return strings.Repeat("word ", len(samples)), nil
}

func request(samples ...int16) []byte {
	out := make([]byte, 4+2*len(samples))
	binary.LittleEndian.PutUint32(out, uint32(2*len(samples)))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[4+2*i:], uint16(s))
	}
	return out
}

func TestServeFraming(t *testing.T) {
	t.Parallel()
	var in bytes.Buffer
	in.Write(request(16384, 32767))
	in.Write(request(-32768))
	in.Write(request())
	in.Write(request(1, 2, 3))
	var out bytes.Buffer
	eng := &fakeEngine{}
	require.NoError(t, serve(&in, &out, eng), "stdin closing between requests is the end")

	require.Equal(t, `{"text":"word word "}`+"\n"+
		`{"error":"the model failed"}`+"\n"+
		// Silence is no text, and still one answer line.
		`{}`+"\n"+
		`{"text":"word word word "}`+"\n", out.String())
	require.Equal(t, [][]float32{{0.5, 32767.0 / 32768}, {-1}, {}, {1.0 / 32768, 2.0 / 32768, 3.0 / 32768}}, eng.got)
}

// gwend never sends half a sample or more than a minute: either means the
// two ends lost their place in the stream, so the helper stops.
func TestServeStopsOnWhatGwendNeverSends(t *testing.T) {
	t.Parallel()
	oversize := make([]byte, 4)
	binary.LittleEndian.PutUint32(oversize, maxRequest+2)
	odd := make([]byte, 4)
	binary.LittleEndian.PutUint32(odd, 3)
	for name, in := range map[string][]byte{
		"too long":       oversize,
		"half a sample":  append(odd, 1, 2, 3),
		"cut short":      request(1, 2, 3)[:7],
		"length cut off": {2, 0},
	} {
		eng := &fakeEngine{}
		err := serve(bytes.NewReader(in), io.Discard, eng)
		require.Error(t, err, name)
		require.Empty(t, eng.got, name)
	}
	require.NoError(t, serve(bytes.NewReader(nil), io.Discard, &fakeEngine{}), "no request at all")
}
