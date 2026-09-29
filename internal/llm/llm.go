// Package llm is the optional LLM adapter of docs/07-integrations.md#llm-adapter:
// it proposes a task breakdown for a goal and writes a weekly retrospective. It
// never schedules and never writes; the deterministic planner stays
// authoritative, and a person accepts every proposal.
package llm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/kzark/gwen/internal/config"
)

// Planner is one provider's implementation of the two jobs.
type Planner interface {
	Breakdown(ctx context.Context, req BreakdownRequest) (BreakdownOutput, error)
	Retro(ctx context.Context, req RetroRequest) (RetroOutput, error)
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
}

// New returns the adapter for the configured provider. Credentials are read
// now, at the moment of use, so replacing the key file needs no restart.
func New(cfg config.LLM, o Options) (Planner, error) {
	client := o.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	key, err := config.ReadCredentialLine(o.CredDir, config.CredLLMAPIKey)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		key = ""
	case err != nil:
		return nil, err
	}
	a := &adapter{model: cfg.Model, timeout: cfg.Timeout}
	switch cfg.Provider {
	case config.ProviderAnthropic:
		if key == "" {
			return nil, &unavailable{"no API key for Anthropic; run gwen setup llm"}
		}
		url := o.AnthropicURL
		if url == "" {
			url = AnthropicURL
		}
		a.name, a.complete = config.ProviderAnthropic, anthropic{client: client, url: url, key: key, model: cfg.Model}.complete
	case config.ProviderOpenAICompatible:
		if cfg.Endpoint == "" {
			return nil, &unavailable{"llm.endpoint is not set; run gwen setup llm"}
		}
		a.name, a.complete = config.ProviderOpenAICompatible,
			openAI{client: client, endpoint: cfg.Endpoint, key: key, model: cfg.Model}.complete
	default:
		return nil, &unavailable{"no LLM provider is configured; run gwen setup llm"}
	}
	return a, nil
}

// adapter implements both jobs over one provider's completion call.
type adapter struct {
	name     string
	model    string
	timeout  time.Duration
	complete func(ctx context.Context, system, user string) (string, error)
}

func (a *adapter) Name() string { return a.name }

// ask sends one request, bounded by llm.timeout, and returns the model's
// reply with any Markdown code fence around the JSON removed.
func (a *adapter) ask(ctx context.Context, system string, payload any) (string, error) {
	user, err := jsonString(payload)
	if err != nil {
		return "", err
	}
	if a.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}
	text, err := a.complete(ctx, system, user)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("the %s request timed out after %s", a.name, a.timeout)
		}
		return "", err
	}
	return unfence(text), nil
}
