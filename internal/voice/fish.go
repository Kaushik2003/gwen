package voice

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// FishURL is Fish Audio's text-to-speech endpoint.
const FishURL = "https://api.fish.audio/v1/tts"

// FishModel is the Fish Audio model asked for: s2.1-pro-free is its free
// developer tier, which understands tags such as [excited] in the text.
const FishModel = "s2.1-pro-free"

// FishVoice is the Fish Audio voice for an empty name: "Gwen Stacy".
const FishVoice = "3026acfe39134bcd8a559df4e9c1f203"

// Fish says text with Fish Audio (fish.audio), online: the text goes to Fish's
// servers and the speech streams back as it is made, at SpeechRate. A voice
// is the id of a Fish voice model, as its page on fish.audio shows it.
type Fish struct {
	HTTP *http.Client
	// Key reads the API key at each use, so a new one needs no restart. ""
	// is no key.
	Key   func() (string, error)
	URL   string // FishURL when empty, for tests
	Model string // FishModel when empty
}

// fishRequest is the body of a Fish Audio TTS request.
type fishRequest struct {
	Text        string      `json:"text"`
	ReferenceID string      `json:"reference_id"`
	Format      string      `json:"format"`
	SampleRate  int         `json:"sample_rate"`
	Latency     string      `json:"latency"`
	Prosody     fishProsody `json:"prosody"`
}

type fishProsody struct {
	Speed float32 `json:"speed"`
}

// Say speaks text in voice at speed, 1 for normal, handing chunk the samples
// as they arrive. It stops early when chunk returns false or ctx ends.
func (f *Fish) Say(ctx context.Context, text, voice string, speed float32, chunk func(samples []float32) bool) error {
	key, err := f.Key()
	if err != nil {
		return err
	}
	if key == "" {
		return errors.New("there is no Fish Audio API key; add it in Settings → AI")
	}
	if voice == "" {
		voice = FishVoice
	}
	body, err := json.Marshal(fishRequest{
		Text: text, ReferenceID: voice, Format: "pcm", SampleRate: SpeechRate,
		Latency: "balanced", // starts speaking sooner than "normal", for conversation
		Prosody: fishProsody{Speed: speed},
	})
	if err != nil {
		return err
	}
	url, model := f.URL, f.Model
	if url == "" {
		url = FishURL
	}
	if model == "" {
		model = FishModel
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("model", model)
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("reach Fish Audio: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fishError(resp)
	}
	if err := readPCM(resp.Body, chunk); err != nil {
		return fmt.Errorf("Fish Audio: %w", err)
	}
	return nil
}

// fishError is why Fish Audio refused, in words.
func fishError(resp *http.Response) error {
	var e struct {
		Message string `json:"message"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if json.Unmarshal(b, &e) != nil || e.Message == "" {
		e.Message = strings.TrimSpace(string(b))
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("Fish Audio did not accept the API key; check it in Settings → AI")
	case http.StatusPaymentRequired:
		return errors.New("Fish Audio wants payment: the free allowance may be used up")
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return errors.New("Fish Audio is busy; try again in a moment")
	}
	if e.Message == "" {
		e.Message = resp.Status
	}
	return fmt.Errorf("Fish Audio: %s", e.Message)
}

// readPCM hands chunk r's signed 16-bit little-endian mono samples as
// float32, as they arrive, until r ends or chunk returns false.
func readPCM(r io.Reader, chunk func(samples []float32) bool) error {
	buf := make([]byte, 8<<10)
	held := 0 // the first byte of a sample split between reads
	for {
		n, err := r.Read(buf[held:])
		n += held
		whole := n &^ 1
		if whole > 0 {
			samples := make([]float32, whole/2)
			for i := range samples {
				samples[i] = float32(int16(binary.LittleEndian.Uint16(buf[2*i:]))) / 32768
			}
			if !chunk(samples) {
				return nil
			}
		}
		held = n - whole
		if held > 0 {
			buf[0] = buf[whole]
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
