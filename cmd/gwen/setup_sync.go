package main

import (
	"strings"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

func setupSyncCmd(begin func(cmd *cobra.Command) (*setupRun, error)) *cobra.Command {
	var hubURL, token string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync with your hub: store its token, set its URL, and sync now",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if hubURL == "" || token == "" {
				return usagef("--hub and --token are required, such as --hub http://pi:7777 --token <token>")
			}
			s, err := begin(cmd)
			if err != nil {
				return err
			}
			return s.sync(hubURL, token)
		},
	}
	cmd.Flags().StringVar(&hubURL, "hub", "", "the hub's URL, such as http://PI_ADDRESS:7777")
	cmd.Flags().StringVar(&token, "token", "", "the hub's sync token")
	return cmd
}

// sync writes the token, sets sync.hub_url, and runs a first sync; with an
// empty history the first pull restores everything the hub holds.
func (s *setupRun) sync(hubURL, token string) error {
	if err := config.WriteCredential(s.env.credDir, config.CredSyncToken, []byte(strings.TrimSpace(token)+"\n")); err != nil {
		return err
	}
	if _, err := s.api.PatchConfig(s.ctx, wire.ConfigPatch{"sync": {"hub_url": strings.TrimRight(hubURL, "/")}}); err != nil {
		return err
	}
	st, err := s.api.SyncNow(s.ctx)
	if err != nil {
		return err
	}
	if st.LastError != nil {
		s.say("Sync is set up, but the first sync failed: %s", *st.LastError)
		return nil
	}
	s.say("Synced with %s. Your phone can open the dashboard at %s/.", hubURL, strings.TrimRight(hubURL, "/"))
	return nil
}
