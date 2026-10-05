package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxTokens bounds every Anthropic reply.
const maxTokens = 16000

// anthropic calls the Messages API with plain net/http.
type anthropic struct {
	client *http.Client
	url    string
	key    string
	model  string
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
}

// apiError is the error body both providers send.
type apiError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (a anthropic) complete(ctx context.Context, p prompt, user string) (string, error) {
	body := anthropicRequest{Model: a.model, MaxTokens: maxTokens, System: p.system,
		Messages: []anthropicMessage{{Role: "user", Content: user}}}
	headers := map[string]string{"x-api-key": a.key, "anthropic-version": "2023-06-01"}
	var res anthropicResponse
	if err := post(ctx, a.client, a.url, headers, body, &res); err != nil {
		return "", fmt.Errorf("anthropic: %w", err)
	}
	switch res.StopReason {
	case "max_tokens":
		return "", errors.New("anthropic: the reply was cut off at the token limit")
	case "refusal":
		return "", errors.New("anthropic: the model declined the request")
	}
	// Only text blocks carry the answer; a thinking block may come first.
	var text strings.Builder
	for _, c := range res.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	if text.Len() == 0 {
		return "", errors.New("anthropic: the reply had no text")
	}
	return text.String(), nil
}

// openAI calls an OpenAI-compatible chat completions endpoint, such as Ollama.
type openAI struct {
	client   *http.Client
	endpoint string
	key      string // empty sends no Authorization header
	model    string
}

type openAIRequest struct {
	Model          string            `json:"model"`
	Messages       []openAIMessage   `json:"messages"`
	ResponseFormat map[string]string `json:"response_format"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func (o openAI) complete(ctx context.Context, p prompt, user string) (string, error) {
	body := openAIRequest{Model: o.model, ResponseFormat: map[string]string{"type": "json_object"},
		Messages: []openAIMessage{{Role: "system", Content: p.system}, {Role: "user", Content: user}}}
	headers := map[string]string{}
	if o.key != "" {
		headers["Authorization"] = "Bearer " + o.key
	}
	var res openAIResponse
	url := strings.TrimRight(o.endpoint, "/") + "/chat/completions"
	if err := post(ctx, o.client, url, headers, body, &res); err != nil {
		return "", fmt.Errorf("openai_compatible: %w", err)
	}
	if len(res.Choices) == 0 || res.Choices[0].Message.Content == "" {
		return "", errors.New("openai_compatible: the reply had no content")
	}
	if res.Choices[0].FinishReason == "length" {
		return "", errors.New("openai_compatible: the reply was cut off at the token limit")
	}
	return res.Choices[0].Message.Content, nil
}

// post sends one JSON request and decodes a 2xx reply into out; any other
// status is an error carrying the provider's message.
func post(ctx context.Context, client *http.Client, url string, headers map[string]string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		var ae apiError
		if json.Unmarshal(raw, &ae) == nil && ae.Error.Message != "" {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, ae.Error.Message)
		}
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode the reply: %w", err)
	}
	return nil
}

// unfence strips a ```json … ``` fence some models wrap JSON in.
func unfence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:] // drop the language tag line
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}

func jsonString(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
