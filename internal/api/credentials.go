package api

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
)

// The credential files Settings may write (docs/07-integrations.md#credentials).
// Gwen's screens live inside Gwen Terminal and cannot write files in the data
// directory themselves, so the daemon does it for them. A value is never
// read back: only whether one is stored.

// credentialNames are the files PUT /v1/credentials/{name} may write.
var credentialNames = []string{config.CredNtfyToken, config.CredGoogleClient, config.CredGoogleToken,
	config.CredLLMAPIKey, config.CredSyncToken, config.CredFishAudioKey, config.CredClaudeSandboxToken}

func (s *Server) listCredentials(w http.ResponseWriter, _ *http.Request) error {
	out := wire.Credentials{Stored: map[string]bool{}}
	for _, name := range credentialNames {
		info, err := os.Stat(filepath.Join(s.CredDir, name))
		out.Stored[name] = err == nil && info.Size() > 0
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) setCredential(w http.ResponseWriter, r *http.Request) error {
	name := r.PathValue("name")
	if !slices.Contains(credentialNames, name) {
		return badRequest("name", "%q is not a credential Gwen knows", name)
	}
	if s.CredDir == "" {
		return Unavailable("this daemon keeps no credentials")
	}
	var req wire.SetCredential
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := config.WriteCredential(s.CredDir, name, []byte(req.Value+"\n")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
