package api

import (
	"net/http"
	"os"
	"path/filepath"

	"narthex/internal/procman"
	"narthex/internal/store"
)

// HandleMdbookProjects lists the configured mdBook project directories and
// the projects recognized under them, so the add-card flow can offer them.
func (s *Service) HandleMdbookProjects(w http.ResponseWriter, r *http.Request) {
	dirs := s.Config.Mdbook.Dirs
	projects := s.MdbookProjects(dirs)
	if projects == nil {
		projects = []procman.MdbookProject{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"dirs":     dirs,
		"projects": projects,
	})
}

type mdbookCreateRequest struct {
	Dir  string `json:"dir"`
	Name string `json:"name"`
}

// HandleMdbookCreate creates a new mdBook project <name> inside one of the
// configured project directories: `mdbook init <name>` in dir. The new
// project is returned so the caller can select it and add the card.
func (s *Service) HandleMdbookCreate(w http.ResponseWriter, r *http.Request) {
	var req mdbookCreateRequest
	if err := readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "err.invalidRequest")
		return
	}
	if s.MdbookBin() == "" {
		s.writeError(w, http.StatusBadRequest, "err.appNotFound", kindLabel(store.KindMdbook))
		return
	}
	if len(s.Config.Mdbook.Dirs) == 0 {
		s.writeError(w, http.StatusBadRequest, "err.mdbookNoDirs")
		return
	}
	parent := filepath.Clean(req.Dir)
	// The parent must be one of the configured project directories.
	if !s.mdbookParentDir(parent) {
		s.writeError(w, http.StatusBadRequest, "err.mdbookInvalidDir")
		return
	}
	if !procman.ValidBookName(req.Name) {
		s.writeError(w, http.StatusBadRequest, "err.mdbookNameInvalid")
		return
	}
	target := filepath.Join(parent, req.Name)
	if _, err := os.Stat(target); err == nil {
		s.writeError(w, http.StatusBadRequest, "err.mdbookExists")
		return
	}
	if err := s.MdbookCreate(parent, req.Name); err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.mdbookCreateFailed", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"name": req.Name,
		"path": filepath.Join(parent, req.Name),
	})
}

// mdbookParentDir reports whether dir is one of the configured mdBook
// project directories (where new books are created).
func (s *Service) mdbookParentDir(dir string) bool {
	for _, d := range s.Config.Mdbook.Dirs {
		if filepath.Clean(d) == dir {
			return true
		}
	}
	return false
}
