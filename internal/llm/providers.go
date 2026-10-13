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
	System    []anthropicBlock   `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

// anthropicBlock is a text block of the system prompt or of a message. A
// cache breakpoint on a block caches the prompt up to and including it.
type anthropicBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text"`
	CacheControl *anthropicCache `json:"cache_control,omitempty"`
}

// anthropicCache is a cache breakpoint: TTL "" keeps the default five
// minutes, "1h" an hour.
type anthropicCache struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

// anthropicMessage is a turn; Content is its text, or a list of blocks when
// it carries a cache breakpoint.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      Usage  `json:"usage"`
}

// apiError is the error body both providers send.
type apiError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// complete caches the prompt at up to three breakpoints, from the part that
// changes least: the system prompt for an hour, since it is the same in
// every request of a job; then the conversation's context; then the history
// up to its last turn, so the next turn of the conversation reads it back.
// The tail goes after the cached system prompt and context, and the newest
// user turn after everything. A part shorter than the model's minimum is
// simply not cached.
func (a anthropic) complete(ctx context.Context, p prompt, c conversation) (string, Usage, error) {
	system := []anthropicBlock{{Type: "text", Text: p.system,
		CacheControl: &anthropicCache{Type: "ephemeral", TTL: "1h"}}}
	// The blocks are joined as they are, so each one after the first brings
	// its own break.
	if c.context != "" {
		system = append(system, anthropicBlock{Type: "text", Text: "\n\n" + c.context,
			CacheControl: &anthropicCache{Type: "ephemeral"}})
	}
	if p.tail != "" {
		system = append(system, anthropicBlock{Type: "text", Text: "\n\n" + p.tail})
	}
	messages := make([]anthropicMessage, 0, len(c.history)+1)
	for i, t := range c.history {
		var content any = t.Text
		if i == len(c.history)-1 {
			content = []anthropicBlock{{Type: "text", Text: t.Text, CacheControl: &anthropicCache{Type: "ephemeral"}}}
		}
		messages = append(messages, anthropicMessage{Role: t.Role, Content: content})
	}
	messages = append(messages, anthropicMessage{Role: RoleUser, Content: c.user})
	body := anthropicRequest{Model: a.model, MaxTokens: maxTokens, System: system, Messages: messages}
	headers := map[string]string{"x-api-key": a.key, "anthropic-version": "2023-06-01"}
	var res anthropicResponse
	if err := post(ctx, a.client, a.url, headers, body, &res); err != nil {
		return "", Usage{}, fmt.Errorf("anthropic: %w", err)
	}
	switch res.StopReason {
	case "max_tokens":
		return "", Usage{}, errors.New("anthropic: the reply was cut off at the token limit")
	case "refusal":
		return "", Usage{}, errors.New("anthropic: the model declined the request")
	}
	// Only text blocks carry the answer; a thinking block may come first.
	var text strings.Builder
	for _, c := range res.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	if text.Len() == 0 {
		return "", Usage{}, errors.New("anthropic: the reply had no text")
	}
	return text.String(), res.Usage, nil
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
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

// complete sends the system prompt with the context first, then the history
// and the newest user turn, so servers that cache prompt prefixes by
// themselves can reuse them.
func (o openAI) complete(ctx context.Context, p prompt, c conversation) (string, Usage, error) {
	messages := make([]openAIMessage, 0, len(c.history)+2)
	messages = append(messages, openAIMessage{Role: "system", Content: p.full(c.context)})
	for _, t := range c.history {
		messages = append(messages, openAIMessage{Role: t.Role, Content: t.Text})
	}
	messages = append(messages, openAIMessage{Role: RoleUser, Content: c.user})
	body := openAIRequest{Model: o.model, ResponseFormat: map[string]string{"type": "json_object"},
		Messages: messages}
	headers := map[string]string{}
	if o.key != "" {
		headers["Authorization"] = "Bearer " + o.key
	}
	var res openAIResponse
	url := strings.TrimRight(o.endpoint, "/") + "/chat/completions"
	if err := post(ctx, o.client, url, headers, body, &res); err != nil {
		return "", Usage{}, fmt.Errorf("openai_compatible: %w", err)
	}
	if len(res.Choices) == 0 || res.Choices[0].Message.Content == "" {
		return "", Usage{}, errors.New("openai_compatible: the reply had no content")
	}
	if res.Choices[0].FinishReason == "length" {
		return "", Usage{}, errors.New("openai_compatible: the reply was cut off at the token limit")
	}
	cached := min(res.Usage.PromptTokensDetails.CachedTokens, res.Usage.PromptTokens)
	return res.Choices[0].Message.Content, Usage{Input: res.Usage.PromptTokens - cached, CacheRead: cached,
		Output: res.Usage.CompletionTokens}, nil
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
