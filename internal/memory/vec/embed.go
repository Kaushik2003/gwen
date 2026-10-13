package vec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Embedding requests go in batches of batchSize texts, each with
// batchTimeout to answer.
const (
	batchSize    = 32
	batchTimeout = 30 * time.Second
	// maxErrorBody is how much of a failed response's body an error quotes.
	maxErrorBody = 300
)

// embedder asks an embedding server for vectors: an OpenAI-compatible one
// when the endpoint ends in /v1, Ollama otherwise.
type embedder struct {
	endpoint string
	model    string
	openAI   bool
	client   *http.Client
}

func newEmbedder(endpoint, model string, client *http.Client) embedder {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if client == nil {
		client = http.DefaultClient
	}
	return embedder{endpoint: endpoint, model: model, openAI: strings.HasSuffix(endpoint, "/v1"), client: client}
}

// embed returns a unit vector for each text, in order.
func (e embedder) embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		batch := texts[start:min(start+batchSize, len(texts))]
		vecs, err := e.batch(ctx, batch)
		if err != nil {
			return nil, err
		}
		if len(vecs) != len(batch) {
			return nil, fmt.Errorf("embed: asked for %d vectors, got %d", len(batch), len(vecs))
		}
		for _, v := range vecs {
			if len(v) == 0 {
				return nil, errors.New("embed: the server returned an empty vector")
			}
			out = append(out, normalize(v))
		}
	}
	return out, nil
}

// batch makes one request.
func (e embedder) batch(ctx context.Context, texts []string) ([][]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, batchTimeout)
	defer cancel()
	path := "/api/embed"
	if e.openAI {
		path = "/embeddings"
	}
	body, err := json.Marshal(map[string]any{"model": e.model, "input": texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("embed: read the answer: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > maxErrorBody {
			msg = msg[:maxErrorBody]
		}
		return nil, fmt.Errorf("embed: %s answered %s: %s", e.endpoint, resp.Status, msg)
	}
	if e.openAI {
		var ans struct {
			Data []struct {
				Index     *int      `json:"index"`
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &ans); err != nil {
			return nil, fmt.Errorf("embed: decode the answer: %w", err)
		}
		vecs := make([][]float32, len(ans.Data))
		for i, d := range ans.Data {
			j := i
			if d.Index != nil {
				j = *d.Index
			}
			if j < 0 || j >= len(vecs) || vecs[j] != nil {
				return nil, fmt.Errorf("embed: the answer has a bad index %d", j)
			}
			vecs[j] = d.Embedding
		}
		return vecs, nil
	}
	var ans struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(raw, &ans); err != nil {
		return nil, fmt.Errorf("embed: decode the answer: %w", err)
	}
	return ans.Embeddings, nil
}

// normalize scales v to unit length; a zero vector is kept as it is.
func normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	n := math.Sqrt(sum)
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / n)
	}
	return out
}

// loopback reports whether an endpoint's host is this machine: localhost,
// 127.0.0.0/8, or ::1.
func loopback(endpoint string) bool {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
