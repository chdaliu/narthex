package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"narthex/internal/store"
)

// HandleListCards returns all cards enriched with live status.
func (s *Service) HandleListCards(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	cards := make([]CardView, 0, len(s.State.Cards))
	for _, c := range s.State.Cards {
		cards = append(cards, s.view(c, r.Host))
	}
	s.mu.Unlock()
	WriteJSON(w, http.StatusOK, map[string]any{"cards": cards})
}

type cardRequest struct {
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	Background string `json:"background"`
	Kind       string `json:"kind"`
	// Dir is the project directory for kinds that point at a project
	// (mdbook); required when Kind is mdbook.
	Dir string `json:"dir"`
}

// HandleCreateCard validates the kind (the app must be installed and the
// kind not yet present — one card per kind) and adds a card.
func (s *Service) HandleCreateCard(w http.ResponseWriter, r *http.Request) {
	var req cardRequest
	if err := readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "err.invalidRequest")
		return
	}
	kind := req.Kind
	if kind == "" {
		kind = store.KindComfyUI
	}
	if kind != store.KindComfyUI && kind != store.KindOpencode && kind != store.KindMdbook && kind != store.KindVSCode && kind != store.KindVSCodium {
		s.writeError(w, http.StatusBadRequest, "err.kindUnsupported", kind)
		return
	}
	if !s.appInstalled(kind) {
		s.writeError(w, http.StatusBadRequest, "err.appNotFound", kindLabel(kind))
		return
	}
	dir := ""
	if kind == store.KindMdbook {
		// The mdbook card must point at a recognized project under the
		// configured directories; never an arbitrary path.
		dir = filepath.Clean(req.Dir)
		if !s.mdbookProjectDir(dir) {
			s.writeError(w, http.StatusBadRequest, "err.mdbookNoProject")
			return
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.State.Cards {
		if c.Kind == kind {
			s.writeError(w, http.StatusConflict, "err.duplicateKind", kindLabel(kind))
			return
		}
	}
	card := store.Card{
		ID:         store.RandomID(8),
		Name:       defaultName(req.Name, kind),
		Icon:       defaultIcon(req.Icon, kind),
		Background: defaultBackground(req.Background),
		Kind:       kind,
		Dir:        dir,
	}
	s.State.Cards = append(s.State.Cards, card)
	if err := s.save(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, s.view(card, r.Host))
}

// HandlePatchCard updates name/icon/background.
func (s *Service) HandlePatchCard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req cardRequest
	if err := readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "err.invalidRequest")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.findCard(id)
	if idx < 0 {
		s.writeError(w, http.StatusNotFound, "err.cardNotFound")
		return
	}
	card := &s.State.Cards[idx]
	if req.Name != "" {
		card.Name = strings.TrimSpace(req.Name)
	}
	if req.Icon != "" {
		card.Icon = req.Icon
	}
	if req.Background != "" {
		card.Background = req.Background
	}
	if err := s.save(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, s.view(*card, r.Host))
}

// HandleDeleteCard stops the instance (if any) and removes the card.
func (s *Service) HandleDeleteCard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.findCard(id)
	if idx < 0 {
		s.writeError(w, http.StatusNotFound, "err.cardNotFound")
		return
	}
	card := s.State.Cards[idx]
	if card.PID > 0 {
		s.Backend.Stop(card.PID)
	}
	s.State.Cards = append(s.State.Cards[:idx], s.State.Cards[idx+1:]...)
	if err := s.save(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleStartCard launches the desktop app for the card. The app bundle is
// auto-detected at start time.
func (s *Service) HandleStartCard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.findCard(id)
	if idx < 0 {
		s.writeError(w, http.StatusNotFound, "err.cardNotFound")
		return
	}
	card := &s.State.Cards[idx]
	if s.Backend.Status(card.Kind, card.PID, card.Port).Alive {
		WriteJSON(w, http.StatusOK, s.view(*card, r.Host))
		return
	}
	if !s.appInstalled(card.Kind) {
		s.writeError(w, http.StatusBadRequest, "err.appNotFound", kindLabel(card.Kind))
		return
	}
	dir := s.installDir(card.Kind, card.Dir)
	pid, port, err := s.Backend.Start(card.Kind, dir, card.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.startFailed", err.Error())
		return
	}
	card.PID = pid
	card.Port = port
	card.StartedAt = time.Now().Unix()
	if err := s.save(); err != nil {
		s.Backend.Stop(pid)
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, s.view(*card, r.Host))
}

// HandleStopCard terminates the card's instance.
func (s *Service) HandleStopCard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.findCard(id)
	if idx < 0 {
		s.writeError(w, http.StatusNotFound, "err.cardNotFound")
		return
	}
	card := &s.State.Cards[idx]
	if card.PID > 0 {
		s.Backend.Stop(card.PID)
	}
	card.PID = 0
	card.Port = 0
	card.StartedAt = 0
	if err := s.save(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, s.view(*card, r.Host))
}

func (s *Service) findCard(id string) int {
	for i := range s.State.Cards {
		if s.State.Cards[i].ID == id {
			return i
		}
	}
	return -1
}

// appInstalled reports whether the desktop app for kind is installed on
// this machine with a usable install. comfyui additionally needs at least
// one managed ComfyUI install (its code directory) to start a server;
// mdbook additionally needs at least one configured project directory
// (mdbook.dirs) — without it the feature is hidden entirely.
func (s *Service) appInstalled(kind string) bool {
	switch kind {
	case store.KindComfyUI:
		return s.ComfyUIAppDir() != "" && len(s.ComfyUIDirs()) > 0
	case store.KindOpencode:
		return s.OpencodeBin() != ""
	case store.KindMdbook:
		return s.MdbookBin() != "" && len(s.Config.Mdbook.Dirs) > 0
	case store.KindVSCode:
		return s.VscodeBin() != ""
	case store.KindVSCodium:
		return s.VscodiumBin() != ""
	}
	return false
}

// installDir returns the auto-detected install directory used to start
// the card: for comfyui the ComfyUI code directory; for opencode and the
// VS Code web server the working directory (the user's home); for mdbook
// the project directory stored on the card (cardDir).
func (s *Service) installDir(kind, cardDir string) string {
	switch kind {
	case store.KindComfyUI:
		if dirs := s.ComfyUIDirs(); len(dirs) > 0 {
			return dirs[0]
		}
	case store.KindMdbook:
		return cardDir
	case store.KindOpencode, store.KindVSCode, store.KindVSCodium:
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	return ""
}

// mdbookProjectDir reports whether dir is one of the mdBook projects
// recognized under the configured project directories.
func (s *Service) mdbookProjectDir(dir string) bool {
	if dir == "" {
		return false
	}
	for _, p := range s.MdbookProjects(s.Config.Mdbook.Dirs) {
		if p.Path == dir {
			return true
		}
	}
	return false
}

// kindLabel is the default card name and display label for a kind.
func kindLabel(kind string) string {
	switch kind {
	case store.KindComfyUI:
		return "ComfyUI"
	case store.KindOpencode:
		return "OpenCode"
	case store.KindMdbook:
		return "MdBook"
	case store.KindVSCode:
		return "VS Code"
	case store.KindVSCodium:
		return "VSCodium"
	}
	return kind
}

func (s *Service) save() error {
	return store.SaveState(filepath.Join(s.Dir, store.StateFile), s.State)
}

func defaultName(name, kind string) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return kindLabel(kind)
}

func defaultIcon(icon, kind string) string {
	if icon == "" {
		switch kind {
		case store.KindComfyUI:
			return "palette"
		case store.KindOpencode:
			return "terminal"
		case store.KindMdbook:
			return "book-open"
		case store.KindVSCode:
			return "code"
		case store.KindVSCodium:
			return "code"
		}
	}
	return icon
}

func defaultBackground(bg string) string {
	if bg == "" {
		return "bg-01"
	}
	return bg
}
