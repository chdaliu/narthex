package api

import (
	"net/http"

	"narthex/internal/procman"
)

// HandleEmulatorGames lists the configured game directories and the ROMs
// recognized under them, so the add-card flow can offer them.
func (s *Service) HandleEmulatorGames(w http.ResponseWriter, r *http.Request) {
	dirs := s.Config.EmulatorJS.Dirs
	games := s.EmulatorGames(dirs)
	if games == nil {
		games = []procman.EmulatorGame{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"dirs":  dirs,
		"games": games,
	})
}
