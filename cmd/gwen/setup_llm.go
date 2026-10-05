package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

// setupClaudeWait bounds asking Claude Code whether it is signed in.
const setupClaudeWait = 30 * time.Second

func setupLLMCmd(begin func(cmd *cobra.Command) (*setupRun, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "llm",
		Short: "Choose an LLM provider for goal breakdowns and weekly retros",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := begin(cmd)
			if err != nil {
				return err
			}
			return s.llm()
		},
	}
}

// llm prompts for the provider and model, then for an API provider the
// endpoint and key, which it stores in the credentials directory, or for
// claude_code the CLI, which it checks is signed in; then it patches the
// configuration.
func (s *setupRun) llm() error {
	cfg, err := s.api.GetConfig(s.ctx)
	if err != nil {
		return err
	}
	s.say("Providers: none, anthropic or openai_compatible (an API with a key, such as Ollama on this")
	s.say("machine or the Pi), or claude_code (your Claude subscription through Claude Code, no key).")
	provider, err := s.ask("Provider", cfg.LLM.Provider)
	if err != nil {
		return err
	}
	patch := map[string]any{"provider": provider}
	if provider != config.ProviderNone {
		model, err := s.ask("Model", cfg.LLM.Model)
		if err != nil {
			return err
		}
		patch["model"] = model
	}
	switch provider {
	case config.ProviderClaudeCode:
		def := cfg.LLM.Command
		if path, err := s.env.lookPath(def); err == nil {
			def = path // the daemon's PATH may not be this shell's
		}
		command, err := s.ask("Claude Code command", def)
		if err != nil {
			return err
		}
		patch["command"] = command
		s.claudeSignedIn(command)
	case config.ProviderAnthropic, config.ProviderOpenAICompatible:
		if provider == config.ProviderOpenAICompatible {
			endpoint, err := s.ask("Endpoint, such as http://localhost:11434/v1", cfg.LLM.Endpoint)
			if err != nil {
				return err
			}
			patch["endpoint"] = endpoint
		}
		key, err := s.ask("API key (empty keeps the current one; Ollama needs none)", "")
		if err != nil {
			return err
		}
		if key != "" {
			if err := config.WriteCredential(s.env.credDir, config.CredLLMAPIKey, []byte(key+"\n")); err != nil {
				return err
			}
			s.say("The key is stored in the credentials directory, readable only by you.")
		}
	}
	if _, err := s.api.PatchConfig(s.ctx, wire.ConfigPatch{"llm": patch}); err != nil {
		return err
	}
	if provider == config.ProviderNone {
		s.say("The LLM adapter is off.")
	} else {
		s.say("The LLM adapter uses %s. Try gwen goal breakdown G or gwen retro.", provider)
	}
	return nil
}

// claudeSignedIn says whether the Claude Code CLI at command is signed in,
// which is what lets it use the subscription.
func (s *setupRun) claudeSignedIn(command string) {
	ctx, cancel := context.WithTimeout(s.ctx, setupClaudeWait)
	defer cancel()
	out, err := s.env.output(ctx, command, "auth", "status", "--json")
	var st struct {
		LoggedIn         bool   `json:"loggedIn"`
		AuthMethod       string `json:"authMethod"`
		SubscriptionType string `json:"subscriptionType"`
	}
	switch {
	case json.Unmarshal(out, &st) != nil:
		reason := "it printed no status"
		if err != nil {
			reason = err.Error()
		}
		s.say("Could not ask %s whether Claude Code is signed in (%s). Install Claude Code, or give the", command, reason)
		s.say("full path to claude, then run gwen setup llm again.")
	case !st.LoggedIn:
		s.say("Claude Code is not signed in. Run claude auth login, then try again.")
	case st.SubscriptionType != "":
		s.say("Claude Code is signed in with your Claude %s subscription.", strings.ToLower(st.SubscriptionType))
	default:
		s.say("Claude Code is signed in (%s).", st.AuthMethod)
	}
}
