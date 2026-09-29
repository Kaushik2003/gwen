package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

// setupCalendarSteps are the Google Cloud steps of
// docs/07-integrations.md#one-time-setup-by-the-user.
var setupCalendarSteps = []string{
	"Google needs every app to have its own OAuth client, so you make one once:",
	"  1. In Google Cloud Console, create a project and enable the Google Calendar API.",
	"  2. Set up the OAuth consent screen as External, add yourself as a test user, then",
	"     publish the app to production. Unverified is fine for personal use; in testing",
	"     mode Google expires the sign-in after 7 days.",
	"  3. Create an OAuth client of type Desktop app and download its JSON.",
	"  4. Run gwen setup calendar --client-file <path>, then gwen cal connect.",
}

func setupCalendarCmd(begin func(cmd *cobra.Command) (*setupRun, error)) *cobra.Command {
	var clientFile string
	cmd := &cobra.Command{
		Use:   "calendar",
		Short: "Connect Google Calendar: install your OAuth client and turn the calendar on",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := begin(cmd)
			if err != nil {
				return err
			}
			return s.calendar(clientFile)
		},
	}
	cmd.Flags().StringVar(&clientFile, "client-file", "", "the OAuth client JSON downloaded from Google Cloud Console")
	return cmd
}

// calendar prints the Google Cloud steps and, given the client file, installs
// it and turns the calendar on.
func (s *setupRun) calendar(clientFile string) error {
	for _, line := range setupCalendarSteps {
		s.say("%s", line)
	}
	if clientFile == "" {
		return nil
	}
	b, err := os.ReadFile(clientFile)
	if err != nil {
		return fmt.Errorf("read the client file: %w", err)
	}
	var client struct {
		Installed *struct {
			ClientID string `json:"client_id"`
		} `json:"installed"`
	}
	if err := json.Unmarshal(b, &client); err != nil || client.Installed == nil || client.Installed.ClientID == "" {
		return usagef("%s is not the JSON of a Desktop app OAuth client", clientFile)
	}
	if err := config.WriteCredential(s.env.credDir, config.CredGoogleClient, b); err != nil {
		return err
	}
	if _, err := s.api.PatchConfig(s.ctx, wire.ConfigPatch{"calendar": {"enabled": true}}); err != nil {
		return err
	}
	s.say("")
	s.say("The OAuth client is installed and the calendar is on. Now run gwen cal connect.")
	return nil
}
