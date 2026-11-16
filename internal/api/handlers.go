package api

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"narthex/internal/procman"
	"narthex/internal/store"
)

// HandleListCards returns all cards enriched with live status.
func (s *Service) HandleListCards(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.reconcileGatewayLocked()
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
	// (mdbook) or the ROM file (emulatorjs); required for both.
	Dir string `json:"dir"`
	// Proxy is the per-card reverse-proxy switch of the proxy-capable
	// built-in kinds. nil leaves it unchanged; true uses the same-origin
	// proxy, false the app's direct URL.
	Proxy *bool `json:"proxy"`
	// Custom is the complete custom-card configuration. Only used for
	// kind == "custom"; nil leaves it unchanged on PATCH.
	Custom *store.CustomCard `json:"custom"`
}

// HandleCreateCard validates the kind (built-in apps must be installed and
// not yet present; custom cards may be repeated) and adds a card.
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
	if kind != store.KindComfyUI && kind != store.KindOpencode && kind != store.KindMdbook && kind != store.KindGitvelo && kind != store.KindVSCode && kind != store.KindVSCodium && kind != store.KindWetty && kind != store.KindEmulatorJS && kind != store.KindCustom {
		s.writeError(w, http.StatusBadRequest, "err.kindUnsupported", kind)
		return
	}
	if kind != store.KindCustom && !s.appInstalled(kind) {
		s.writeError(w, http.StatusBadRequest, "err.appNotFound", kindLabel(kind))
		return
	}
	if kind != store.KindCustom && req.Custom != nil {
		s.writeError(w, http.StatusBadRequest, "err.lockedField")
		return
	}
	dir := ""
	custom := (*store.CustomCard)(nil)
	switch kind {
	case store.KindMdbook:
		// The mdbook card must point at a recognized project under the
		// configured directories; never an arbitrary path.
		dir = filepath.Clean(req.Dir)
		if !s.mdbookProjectDir(dir) {
			s.writeError(w, http.StatusBadRequest, "err.mdbookNoProject")
			return
		}
	case store.KindEmulatorJS:
		// The emulatorjs card must point at a ROM recognized under the
		// configured game directories; never an arbitrary path.
		dir = filepath.Clean(req.Dir)
		if !s.emulatorGamePath(dir) {
			s.writeError(w, http.StatusBadRequest, "err.emulatorNoGame")
			return
		}
	case store.KindCustom:
		custom = req.Custom
		if custom == nil {
			custom = &store.CustomCard{}
		}
		store.NormalizeCustomCard(custom)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileGatewayLocked()
	if kind != store.KindCustom {
		for _, c := range s.State.Cards {
			if c.Kind == kind {
				s.writeError(w, http.StatusConflict, "err.duplicateKind", kindLabel(kind))
				return
			}
		}
	}
	if custom != nil {
		if errKey := s.validateCustom(custom, ""); errKey != "" {
			s.writeError(w, http.StatusBadRequest, errKey)
			return
		}
		if errKey := s.ensureCustomGatewayPort(custom, ""); errKey != "" {
			s.writeError(w, http.StatusBadRequest, errKey)
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
		Custom:     custom,
	}
	s.State.Cards = append(s.State.Cards, card)
	if err := s.save(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	s.reconcileGatewayLocked()
	WriteJSON(w, http.StatusOK, s.view(card, r.Host))
}

// HandlePatchCard updates name/icon/background and, for custom cards, the
// custom configuration.
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
	if card.Kind != store.KindCustom && req.Custom != nil {
		s.writeError(w, http.StatusBadRequest, "err.lockedField")
		return
	}
	if req.Name != "" {
		card.Name = strings.TrimSpace(req.Name)
	}
	if req.Icon != "" {
		card.Icon = req.Icon
	}
	if req.Background != "" {
		card.Background = req.Background
	}
	if req.Custom != nil && card.Kind == store.KindCustom {
		next := *req.Custom
		store.NormalizeCustomCard(&next)
		if errKey := s.validateCustom(&next, card.ID); errKey != "" {
			s.writeError(w, http.StatusBadRequest, errKey)
			return
		}
		if s.Backend.Status(card.Kind, card.PID, card.Port).Alive && customRuntimeChanged(card.Custom, &next) {
			s.writeError(w, http.StatusConflict, "err.customEditRunning")
			return
		}
		if errKey := s.ensureCustomGatewayPort(&next, card.ID); errKey != "" {
			s.writeError(w, http.StatusBadRequest, errKey)
			return
		}
		card.Custom = &next
		delete(s.autoInstallFailed, card.ID)
		delete(s.lastErrors, card.ID)
		s.reconcileGatewayLocked()
	}
	if req.Proxy != nil && proxyCapable(card.Kind) && card.Kind != store.KindCustom {
		// A plain bool cannot distinguish "unset" from false, so the field
		// is a pointer: only an explicit value flips the switch.
		//
		// gitvelo's proxy switch changes how the child server is spawned
		// (--no-auth on loopback vs its own login on the configured
		// address), so it cannot be applied to an already running process.
		if card.Kind == store.KindGitvelo && card.DisableProxy != !*req.Proxy && s.Backend.Status(card.Kind, card.PID, card.Port).Alive {
			s.writeError(w, http.StatusConflict, "err.gitveloProxyRunning")
			return
		}
		card.DisableProxy = !*req.Proxy
	}
	if err := s.save(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, s.view(*card, r.Host))
}

// HandleReorderCards persists a new card order. The request carries the
// card IDs in their desired order; unknown or duplicate IDs are ignored,
// and any card not mentioned keeps its relative position at the end. The
// order lives in the card slice itself, so it is persisted with the state.
func (s *Service) HandleReorderCards(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "err.invalidRequest")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	byID := make(map[string]store.Card, len(s.State.Cards))
	for _, c := range s.State.Cards {
		byID[c.ID] = c
	}
	ordered := make([]store.Card, 0, len(s.State.Cards))
	seen := make(map[string]bool, len(s.State.Cards))
	for _, id := range req.IDs {
		c, ok := byID[id]
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		ordered = append(ordered, c)
	}
	for _, c := range s.State.Cards {
		if !seen[c.ID] {
			ordered = append(ordered, c)
		}
	}
	s.State.Cards = ordered
	if err := s.save(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
	// A running instance must be stopped first: deleting it out from under
	// the user hides the fact that the process is being killed and can lose
	// unsaved work in the app.
	if s.Backend.Status(card.Kind, card.PID, card.Port).Alive {
		s.writeError(w, http.StatusConflict, "err.cardRunning")
		return
	}
	if r := s.installs[card.ID]; r != nil && r.running {
		s.writeError(w, http.StatusConflict, "err.installRunning")
		return
	}
	if card.PID > 0 {
		s.Backend.Stop(card.PID)
	}
	if g := s.customGateways[card.ID]; g != nil {
		g.Stop()
		delete(s.customGateways, card.ID)
		delete(s.customGatewayWanted, card.ID)
	}
	delete(s.installs, card.ID)
	delete(s.lastErrors, card.ID)
	delete(s.autoInstallFailed, card.ID)
	s.State.Cards = append(s.State.Cards[:idx], s.State.Cards[idx+1:]...)
	if err := s.save(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	s.reconcileGatewayLocked()
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleStartCard launches the app for the card. A custom card whose program
// is missing may start an automatic install first and start itself once the
// install succeeds.
func (s *Service) HandleStartCard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	idx := s.findCard(id)
	if idx < 0 {
		s.mu.Unlock()
		s.writeError(w, http.StatusNotFound, "err.cardNotFound")
		return
	}
	card := &s.State.Cards[idx]
	if s.Backend.Status(card.Kind, card.PID, card.Port).Alive {
		s.reconcileGatewayLocked()
		v := s.view(*card, r.Host)
		s.mu.Unlock()
		WriteJSON(w, http.StatusOK, v)
		return
	}
	if card.Kind != store.KindCustom && !s.appInstalled(card.Kind) {
		s.mu.Unlock()
		s.writeError(w, http.StatusBadRequest, "err.appNotFound", kindLabel(card.Kind))
		return
	}
	if s.needsAutoInstallLocked(card) {
		if s.autoInstallFailed[card.ID] {
			s.mu.Unlock()
			s.writeError(w, http.StatusBadRequest, "err.customExecNotFound", card.Custom.Exec)
			return
		}
		st, errKey := s.beginInstallLocked(card, true, s.browserHosts(r.Host))
		if errKey != "" {
			s.mu.Unlock()
			s.writeError(w, http.StatusBadRequest, errKey)
			return
		}
		v := s.view(*card, r.Host)
		s.mu.Unlock()
		go s.runInstall(st, card.ID, true, s.browserHosts(r.Host))
		WriteJSON(w, http.StatusOK, v)
		return
	}
	v, errKey, err := s.startCardLocked(card, r.Host)
	if errKey != "" {
		s.mu.Unlock()
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, errKey, err.Error())
		} else {
			s.writeError(w, http.StatusBadRequest, errKey)
		}
		return
	}
	s.mu.Unlock()
	WriteJSON(w, http.StatusOK, v)
}

// startCardLocked spawns the instance for card. Caller holds s.mu and has
// verified that the card is stopped. It returns the updated view, an i18n
// error key ("" on success), and the raw error for formatting.
func (s *Service) startCardLocked(card *store.Card, reqHost string) (CardView, string, error) {
	dir := s.installDir(card.Kind, card.Dir)
	if card.Kind == store.KindCustom && card.Custom != nil && card.Custom.WorkDir != "" {
		dir = store.ExpandPath(s.Dir, card.Custom.WorkDir)
	}
	spec := procman.StartSpec{
		Card:   *card,
		Dir:    dir,
		Hosts:  s.browserHosts(reqHost),
		NoAuth: card.Kind == store.KindGitvelo && !card.DisableProxy,
		Launch: s.launchFor(*card),
	}
	pid, port, err := s.Backend.Start(spec)
	if err != nil {
		s.lastErrors[card.ID] = err.Error()
		return CardView{}, "err.startFailed", err
	}
	card.PID = pid
	card.Port = port
	card.StartedAt = time.Now().Unix()
	if card.Kind == store.KindCustom && card.Custom != nil && card.Custom.PortMode == store.CustomPortAuto && port > 0 {
		card.LastPort = port
	}
	delete(s.lastErrors, card.ID)
	delete(s.autoInstallFailed, card.ID)
	if err := s.save(); err != nil {
		s.Backend.Stop(pid)
		return CardView{}, "err.saveFailed", err
	}
	s.reconcileGatewayLocked()
	return s.view(*card, reqHost), "", nil
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
	s.reconcileGatewayLocked()
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

// opencodeRunningLocked reports whether the opencode card currently has a
// live instance. Caller holds s.mu.
func (s *Service) opencodeRunningLocked() bool {
	for _, c := range s.State.Cards {
		if c.Kind == store.KindOpencode {
			return s.Backend.Status(c.Kind, c.PID, c.Port).Alive
		}
	}
	return false
}

// gitveloRunningLocked reports whether the gitvelo card currently has a
// live instance. Caller holds s.mu.
func (s *Service) gitveloRunningLocked() bool {
	for _, c := range s.State.Cards {
		if c.Kind == store.KindGitvelo {
			return s.Backend.Status(c.Kind, c.PID, c.Port).Alive
		}
	}
	return false
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
	case store.KindGitvelo:
		return s.GitveloBin() != ""
	case store.KindVSCode:
		return s.VscodeBin() != ""
	case store.KindVSCodium:
		return s.VscodiumBin() != ""
	case store.KindWetty:
		return s.WettyBin() != ""
	case store.KindEmulatorJS:
		// EmulatorJS has no external CLI: the kind is available once at
		// least one game directory is configured.
		return len(s.Config.EmulatorJS.Dirs) > 0
	case store.KindCustom:
		// Custom cards need no detection: the user supplies the program.
		return true
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
	case store.KindEmulatorJS:
		return cardDir
	case store.KindOpencode, store.KindVSCode, store.KindVSCodium, store.KindWetty:
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	case store.KindGitvelo:
		// The GitVelo web service reads its repositories from the whole
		// filesystem; start it from the user's home like the other CLIs.
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	case store.KindCustom:
		// startCardLocked substitutes the card's WorkDir; the home is the
		// fallback when it is empty.
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

// emulatorGamePath reports whether dir is one of the ROM files recognized
// under the configured game directories.
func (s *Service) emulatorGamePath(dir string) bool {
	if dir == "" {
		return false
	}
	for _, g := range s.EmulatorGames(s.Config.EmulatorJS.Dirs) {
		if g.Path == dir {
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
	case store.KindGitvelo:
		return "GitVelo"
	case store.KindVSCode:
		return "VS Code"
	case store.KindVSCodium:
		return "VSCodium"
	case store.KindWetty:
		return "WeTTY"
	case store.KindEmulatorJS:
		return "EmulatorJS"
	case store.KindCustom:
		return "Custom"
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
		case store.KindGitvelo:
			return "git-branch"
		case store.KindVSCode:
			return "code"
		case store.KindVSCodium:
			return "code"
		case store.KindWetty:
			return "terminal"
		case store.KindEmulatorJS:
			return "gamepad-2"
		case store.KindCustom:
			return "rocket"
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

// validateCustom checks a custom-card configuration and returns an i18n
// error key, or "" when it is valid. cardID is the card being updated (empty
// on create), used to ignore its own fixed ports during conflict checks.
func (s *Service) validateCustom(c *store.CustomCard, cardID string) string {
	if c == nil {
		return "err.customRequired"
	}
	store.NormalizeCustomCard(c)
	if len(c.Exec) > 1024 || len(c.Command) > 4096 || len(c.WorkDir) > 1024 || len(c.InstallCommand) > 4096 {
		return "err.customTooLong"
	}
	if c.ExecMode == store.CustomExecArgs && strings.TrimSpace(c.Exec) == "" {
		return "err.customExecRequired"
	}
	if c.ExecMode == store.CustomExecShell && strings.TrimSpace(c.Command) == "" {
		return "err.customCommandRequired"
	}
	if c.ProxyMode != store.CustomProxyOff && c.PortMode == store.CustomPortNone {
		return "err.customProxyNeedsPort"
	}
	if c.PortMode == store.CustomPortFixed {
		if c.Port < 1 || c.Port > 65535 {
			return "err.customPortInvalid"
		}
		if c.Port == s.Config.Port {
			return "err.customPortConflict"
		}
		for i := range s.State.Cards {
			other := &s.State.Cards[i]
			if other.ID == cardID || other.Kind != store.KindCustom || other.Custom == nil {
				continue
			}
			if other.Custom.PortMode == store.CustomPortFixed && other.Custom.Port == c.Port {
				return "err.customPortConflict"
			}
		}
	}
	if c.ProxyMode == store.CustomProxyGateway && c.GatewayPort != 0 {
		if c.GatewayPort < 1 || c.GatewayPort > 65535 || c.GatewayPort == s.Config.Port {
			return "err.customGatewayPortInvalid"
		}
		for i := range s.State.Cards {
			other := &s.State.Cards[i]
			if other.ID == cardID || other.Kind != store.KindCustom || other.Custom == nil {
				continue
			}
			if other.Custom.ProxyMode == store.CustomProxyGateway && other.Custom.GatewayPort == c.GatewayPort {
				return "err.customGatewayPortConflict"
			}
		}
	}
	if c.InstallEnabled && strings.TrimSpace(c.InstallCommand) == "" {
		return "err.installNoCommand"
	}
	return ""
}

// customRuntimeChanged reports whether next changes a field that is baked
// into the running process and therefore requires a stop/start.
func customRuntimeChanged(cur, next *store.CustomCard) bool {
	if cur == nil {
		return true
	}
	return cur.ExecMode != next.ExecMode ||
		cur.Exec != next.Exec ||
		cur.Command != next.Command ||
		cur.WorkDir != next.WorkDir ||
		cur.PortMode != next.PortMode ||
		cur.Port != next.Port
}

// ensureCustomGatewayPort allocates and persists a preferred gateway port
// for a custom card in gateway proxy mode. Caller holds s.mu.
func (s *Service) ensureCustomGatewayPort(c *store.CustomCard, cardID string) string {
	if c == nil || c.ProxyMode != store.CustomProxyGateway || c.GatewayPort != 0 {
		return ""
	}
	lo, hi := s.Config.Custom.GatewayPortRange[0], s.Config.Custom.GatewayPortRange[1]
	for p := lo; p <= hi; p++ {
		if p == s.Config.Port || p == s.Config.Opencode.GatewayPort || p == s.Config.Gitvelo.GatewayPort {
			continue
		}
		used := false
		for i := range s.State.Cards {
			other := &s.State.Cards[i]
			if other.ID == cardID || other.Kind != store.KindCustom || other.Custom == nil {
				continue
			}
			if other.Custom.ProxyMode == store.CustomProxyGateway && other.Custom.GatewayPort == p {
				used = true
				break
			}
		}
		if used || !apiPortAvailable(p) {
			continue
		}
		c.GatewayPort = p
		return ""
	}
	return "err.customGatewayPortUnavailable"
}

func apiPortAvailable(port int) bool {
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	l.Close()
	return true
}
