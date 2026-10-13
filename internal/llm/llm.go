// Package llm is the optional LLM adapter of docs/07-integrations.md#llm-adapter:
// it proposes a task breakdown for a goal, plans a day in a conversation,
// writes a weekly retrospective, talks with the user as the assistant, and
// does the jobs of Gwen's memory (memory.go). It never writes; the
// deterministic planner stays authoritative, and a person accepts every
// proposal.
package llm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/config"
)

// Planner is one provider's implementation of the three jobs.
type Planner interface {
	Breakdown(ctx context.Context, req BreakdownRequest) (BreakdownOutput, error)
	DayPlan(ctx context.Context, req DayPlanRequest) (DayPlanOutput, error)
	Retro(ctx context.Context, req RetroRequest) (RetroOutput, error)
	Assistant(ctx context.Context, req AssistantRequest) (AssistantOutput, error)
	Name() string
}

// ErrUnavailable is matched by New's error when no provider is configured or
// its credentials are missing; the message names the setup step.
var ErrUnavailable = errors.New("llm unavailable")

type unavailable struct{ msg string }

func (e *unavailable) Error() string        { return e.msg }
func (e *unavailable) Is(target error) bool { return target == ErrUnavailable }

// AnthropicURL is the Messages API endpoint.
const AnthropicURL = "https://api.anthropic.com/v1/messages"

// Options are what New needs beyond the configuration.
type Options struct {
	CredDir    string
	HTTPClient *http.Client // nil means http.DefaultClient
	// AnthropicURL overrides the Messages API endpoint, for tests.
	AnthropicURL string
	// Self is how the assistant has chosen to treat the user, which closes
	// the prompts that talk to them.
	Self Self
}

// New returns the adapter for the configured provider. Credentials are read
// now, at the moment of use, so replacing the key file needs no restart.
func New(cfg config.LLM, o Options) (Planner, error) {
	a, err := newAdapter(cfg, o, cfg.Provider)
	if err != nil {
		return nil, err
	}
	a.persona, a.self = PersonaPrompt(cfg), SelfPrompt(o.Self)
	return a, nil
}

// NewMemory returns the memory jobs over the configured provider, as New
// does the planner's: the same providers, credentials, and errors. Its
// prompts carry no persona, since none of them talks to the user. Its name
// is the provider and model, which every claim it extracts records.
func NewMemory(cfg config.LLM, o Options) (Memory, error) {
	name := cfg.Provider
	if cfg.Model != "" {
		name += "/" + cfg.Model
	}
	a, err := newAdapter(cfg, o, name)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// newAdapter picks the provider's completion call.
func newAdapter(cfg config.LLM, o Options, name string) (*adapter, error) {
	client := o.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	a := &adapter{name: name, timeout: cfg.Timeout}
	switch cfg.Provider {
	case config.ProviderAnthropic:
		key, err := apiKey(o.CredDir)
		if err != nil {
			return nil, err
		}
		if key == "" {
			return nil, &unavailable{"no API key for Anthropic; run gwen setup llm"}
		}
		url := o.AnthropicURL
		if url == "" {
			url = AnthropicURL
		}
		a.complete = anthropic{client: client, url: url, key: key, model: cfg.Model}.complete
	case config.ProviderOpenAICompatible:
		if cfg.Endpoint == "" {
			return nil, &unavailable{"llm.endpoint is not set; run gwen setup llm"}
		}
		key, err := apiKey(o.CredDir)
		if err != nil {
			return nil, err
		}
		a.complete = openAI{client: client, endpoint: cfg.Endpoint, key: key, model: cfg.Model}.complete
	case config.ProviderClaudeCode:
		c, err := newClaudeCode(cfg)
		if err != nil {
			return nil, err
		}
		a.complete = c.complete
	default:
		return nil, &unavailable{"no LLM provider is configured; run gwen setup llm"}
	}
	return a, nil
}

// apiKey reads the HTTP providers' key file; a missing file is no key.
func apiKey(credDir string) (string, error) {
	key, err := config.ReadCredentialLine(credDir, config.CredLLMAPIKey)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return key, err
}

// Usage is what one request used, in tokens, as the provider reported it:
// Input is the input read in full, CacheRead the input read from the prompt
// cache, and CacheWrite the input written to it. What a provider does not
// report is 0.
type Usage struct {
	Input      int `json:"input_tokens"`
	CacheRead  int `json:"cache_read_input_tokens"`
	CacheWrite int `json:"cache_creation_input_tokens"`
	Output     int `json:"output_tokens"`
}

// adapter implements the jobs over one provider's completion call.
type adapter struct {
	name     string
	timeout  time.Duration
	persona  string // opens the prompts that talk to the user
	self     string // closes them
	complete func(ctx context.Context, p prompt, c conversation) (string, Usage, error)
}

// conversation is what one request sends besides the system prompt: context,
// which follows the system prompt and is cached apart from it because it
// changes more often (the assistant's profile card and observation log);
// history, the earlier turns, oldest first, already normalised; and user,
// the newest user turn.
type conversation struct {
	context string
	history []Turn
	user    string
}

func (a *adapter) Name() string { return a.name }

// ask sends one request with no context or history, bounded by llm.timeout,
// and returns the model's reply with any Markdown code fence around the JSON
// removed, and what the request used.
func (a *adapter) ask(ctx context.Context, p prompt, payload any) (string, Usage, error) {
	return a.askTurns(ctx, p, "", nil, payload)
}

// askTurns is ask with the context that follows the system prompt and the
// earlier turns of the conversation, which go before payload, the newest
// user turn.
func (a *adapter) askTurns(ctx context.Context, p prompt, contextText string, history []Turn,
	payload any) (string, Usage, error) {
	user, err := jsonString(payload)
	if err != nil {
		return "", Usage{}, err
	}
	if p.persona {
		if a.persona != "" {
			p.system = a.persona + "\n\n" + p.system
		}
		p.tail = a.self
	}
	turns, user := normalizeHistory(history, user)
	if a.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}
	text, use, err := a.complete(ctx, p, conversation{context: contextText, history: turns, user: user})
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", Usage{}, fmt.Errorf("the %s request timed out after %s", a.name, a.timeout)
		}
		return "", Usage{}, err
	}
	slog.Info("llm request", "provider", a.name, "job", p.name, "input_tokens", use.Input,
		"cache_read_tokens", use.CacheRead, "cache_write_tokens", use.CacheWrite, "output_tokens", use.Output)
	return unfence(text), use, nil
}

// normalizeHistory makes the earlier turns a conversation every provider
// takes: turns with no text or another role are left out, leading assistant
// turns are dropped, and consecutive turns of one role are merged with a
// blank line, so the history starts with the user and alternates. It ends
// with an assistant turn: a trailing user turn is merged into the newest
// message, before it. The caller's slice is not changed, and the same turns
// always give the same result, so the prompt cache holds.
func normalizeHistory(history []Turn, user string) ([]Turn, string) {
	var out []Turn
	for _, t := range history {
		if t.Role != RoleUser && t.Role != RoleAssistant || strings.TrimSpace(t.Text) == "" {
			continue
		}
		if len(out) == 0 && t.Role == RoleAssistant {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Role == t.Role {
			out[n-1].Text += "\n\n" + t.Text
			continue
		}
		out = append(out, t)
	}
	if n := len(out); n > 0 && out[n-1].Role == RoleUser {
		user = out[n-1].Text + "\n\n" + user
		out = out[:n-1]
	}
	return out, user
}
