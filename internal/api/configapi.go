package api

import (
	"fmt"
	"net/http"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
)

func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) error {
	writeJSON(w, http.StatusOK, s.Tracker.Config().Wire())
	return nil
}

// patchConfig validates a partial config, saves it, applies it to the running
// engine at once, and emits config_changed.
func (s *Server) patchConfig(w http.ResponseWriter, r *http.Request) error {
	var patch wire.ConfigPatch
	if err := decode(r, &patch); err != nil {
		return err
	}
	c, err := config.Patch(s.Tracker.Config(), patch)
	if err != nil {
		return err
	}
	if err := config.Save(s.ConfigPath, c); err != nil {
		return fmt.Errorf("patch config: %w", err)
	}
	if err := s.Tracker.SetConfig(r.Context(), c); err != nil {
		return err
	}
	s.Hub.Publish(wire.EventConfigChanged, c.Wire())
	writeJSON(w, http.StatusOK, c.Wire())
	return nil
}
