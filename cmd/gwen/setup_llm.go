package main

import (
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

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

// llm prompts for the provider, model, endpoint, and key, stores the key in
// the credentials directory, and patches the configuration.
func (s *setupRun) llm() error {
	cfg, err := s.api.GetConfig(s.ctx)
	if err != nil {
		return err
	}
	s.say("Providers: none, anthropic, or openai_compatible (such as Ollama on this machine or the Pi).")
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
