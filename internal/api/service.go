package api

import (
	"crypto/subtle"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"narthex/internal/auth"
	"narthex/internal/i18n"
	"narthex/internal/procman"
	"narthex/internal/secretbox"
	"narthex/internal/store"
	"narthex/web"
)

// Service owns the runtime state shared by all handlers.
type Service struct {
	mu      sync.Mutex
	Dir     string // config directory (contains config.json, state.json, logs/)
	Config  *store.Config
	State   *store.State
	Backend procman.Backend
	Limiter *auth.RateLimiter
	// ComfyUIAppDir returns the Comfy Desktop.app bundle directory, or ""
	// when the desktop app is not installed. Defaults to
	// procman.DetectComfyUIApp; overridable in tests.
	ComfyUIAppDir func() string
	// ComfyUIDirs returns the code directories of the ComfyUI Desktop
	// managed installs. Defaults to procman.DetectComfyUIInstalls;
	// overridable in tests.
	ComfyUIDirs func() []string
	// OpencodeBin returns the path to the opencode CLI, or "" when it is
	// not installed. Defaults to procman.DetectOpencode; overridable in
	// tests.
	OpencodeBin func() string
	// MdbookBin returns the path to the mdbook CLI, or "" when it is not
	// installed. Defaults to procman.DetectMdbook; overridable in tests.
	MdbookBin func() string
	// MdbookProjects returns the mdBook projects found under the given
	// directories. Defaults to procman.DetectMdbookProjects; overridable
	// in tests.
	MdbookProjects func(dirs []string) []procman.MdbookProject
	// MdbookCreate creates a new mdBook project <name> inside parent using
	// the mdbook CLI. Defaults to a wrapper around
	// procman.CreateMdbookProject; overridable in tests.
	MdbookCreate func(parent, name string) error
	// VscodeBin returns the path to the VS Code CLI (`code`), or "" when it
	// is not installed. Defaults to procman.DetectVSCode; overridable in
	// tests.
	VscodeBin func() string
	// VscodiumBin returns the path to the VSCodium CLI (`codium`), or ""
	// when it is not installed. Defaults to procman.DetectVSCodium;
	// overridable in tests.
	VscodiumBin func() string
	// Username is the decrypted login username (plaintext in memory only).
	Username string
}

// NewService builds a Service. State and config are already loaded by the
// caller and remain owned by it.
func NewService(dir string, cfg *store.Config, state *store.State, backend procman.Backend, username string) *Service {
	return &Service{
		Dir:            dir,
		Config:         cfg,
		State:          state,
		Backend:        backend,
		Limiter:        auth.NewRateLimiter(5, time.Minute),
		ComfyUIAppDir:  procman.DetectComfyUIApp,
		ComfyUIDirs:    procman.DetectComfyUIInstalls,
		OpencodeBin:    procman.DetectOpencode,
		MdbookBin:      procman.DetectMdbook,
		MdbookProjects: procman.DetectMdbookProjects,
		MdbookCreate: func(parent, name string) error {
			return procman.CreateMdbookProject(procman.DetectMdbook(), parent, name)
		},
		VscodeBin:   procman.DetectVSCode,
		VscodiumBin: procman.DetectVSCodium,
		Username:    username,
	}
}

// lang returns the configured language, falling back to en.
func (s *Service) lang() i18n.Lang {
	if l, ok := i18n.Parse(s.Config.Language); ok {
		return l
	}
	return i18n.EN
}

// writeError sends a localized error response. args are applied to the
// translation with fmt.Sprintf.
func (s *Service) writeError(w http.ResponseWriter, code int, key string, args ...any) {
	msg := i18n.T(s.lang(), key)
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	WriteJSON(w, code, map[string]string{"error": msg})
}

// CardView is a Card enriched with live runtime status.
type CardView struct {
	store.Card
	Running  bool   `json:"running"`
	Healthy  bool   `json:"healthy"`
	MemoryKB int64  `json:"memoryKB"`
	Uptime   int64  `json:"uptime"`
	URL      string `json:"url"`
	// Username and Password are the opencode web basic-auth credentials;
	// only set for opencode cards.
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func (s *Service) view(c store.Card, reqHost string) CardView {
	v := CardView{Card: c}
	switch c.Kind {
	case store.KindOpencode:
		v.Username = store.OpencodeUsername
		v.Password = s.Config.Opencode.Password
	case store.KindVSCode:
		// The web UI asks for the connection token only (no username).
		v.Password = s.Config.Vscode.ConnectionToken
	case store.KindVSCodium:
		// The web UI asks for the connection token only (no username).
		v.Password = s.Config.Vscodium.ConnectionToken
	}
	st := s.Backend.Status(c.Kind, c.PID, c.Port)
	v.Running = st.Alive
	if st.Alive {
		v.Healthy = st.Healthy
		v.MemoryKB = st.MemoryKB
		if c.StartedAt > 0 {
			v.Uptime = time.Now().Unix() - c.StartedAt
		}
		// Servers bind 0.0.0.0 (LAN access) when configured so: the link
		// is built from the visitor's request host so it works from other
		// devices. An explicitly configured internal network address wins
		// over the request host.
		v.URL = s.instanceURL(reqHost, c.Kind, c.Port)
	}
	return v
}

// instanceURL builds the base URL for a running card. When an internal
// network address is configured it is used as-is (it may carry its own
// port); otherwise the host mirrors how the visitor reached narthex (LAN
// access) or falls back to loopback. The VS Code-family kinds append
// their connection token (`?tkn=`) because the server answers 403 without
// it.
func (s *Service) instanceURL(reqHost, kind string, port int) string {
	var base string
	if addr := strings.TrimSpace(s.Config.InternalAddress); addr != "" {
		addr = strings.TrimPrefix(addr, "http://")
		addr = strings.TrimPrefix(addr, "https://")
		addr = strings.TrimSuffix(addr, "/")
		if _, _, err := net.SplitHostPort(addr); err == nil {
			base = "http://" + addr + "/"
		} else {
			base = fmt.Sprintf("http://%s:%d/", addr, port)
		}
	} else {
		base = fmt.Sprintf("http://%s:%d/", s.instanceHost(reqHost, kind), port)
	}
	if token := s.codeToken(kind); token != "" {
		base += "?tkn=" + token
	}
	return base
}

// codeToken returns the connection token of the VS Code-family kinds. It
// is embedded in the "Open" URL so the browser sets the auth cookie on
// first load (the server redirects to / and answers 200 afterwards).
func (s *Service) codeToken(kind string) string {
	switch kind {
	case store.KindVSCode:
		return s.Config.Vscode.ConnectionToken
	case store.KindVSCodium:
		return s.Config.Vscodium.ConnectionToken
	}
	return ""
}

// instanceHost picks the host used in card "open" URLs: when a server
// kind binds a non-loopback address (LAN access), links are built from
// the visitor's request host; loopback-only kinds use 127.0.0.1.
func (s *Service) instanceHost(reqHost string, kind string) string {
	host := s.Config.ComfyUI.Hostname
	switch kind {
	case store.KindOpencode:
		host = s.Config.Opencode.Hostname
	case store.KindMdbook:
		host = s.Config.Mdbook.Hostname
	case store.KindVSCode:
		host = s.Config.Vscode.Hostname
	case store.KindVSCodium:
		host = s.Config.Vscodium.Hostname
	}
	if !IsLoopback(host) && reqHost != "" {
		if hostname, _, err := net.SplitHostPort(reqHost); err == nil {
			return strings.Trim(hostname, "[]")
		}
		return strings.Trim(reqHost, "[]")
	}
	return "127.0.0.1"
}

// IsLoopback reports whether host is a loopback address.
func IsLoopback(host string) bool {
	h := host
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		h = hostname
	}
	h = strings.Trim(h, "[]")
	return h == "127.0.0.1" || h == "localhost" || h == "::1" || h == ""
}

// HandleLogin verifies username and password and issues a signed session
// cookie. Failures are deliberately reported with the same error to avoid
// account enumeration.
func (s *Service) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr
	if host := r.Header.Get("X-Forwarded-For"); host != "" {
		ip = host
	}
	if !s.Limiter.Allow(ip) {
		s.writeError(w, http.StatusTooManyRequests, "err.tooManyAttempts")
		return
	}
	if s.Config.PasswordHash == "" {
		s.writeError(w, http.StatusInternalServerError, "err.noPasswordConfigured")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "err.invalidRequest")
		return
	}
	userOK := subtle.ConstantTimeCompare([]byte(s.Username), []byte(req.Username)) == 1
	if !userOK || !auth.VerifyPassword(s.Config.PasswordHash, req.Password) {
		s.writeError(w, http.StatusUnauthorized, "err.credentialsWrong")
		return
	}
	// A correct login resets the quota so that success never counts
	// against the rate limiter.
	s.Limiter.Reset(ip)
	ttl := s.sessionTTL()
	token := auth.Sign(s.Config.SessionSecret, store.RandomID(12), time.Now().Add(ttl).Unix())
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleAccount changes the username and/or password. The current password
// must be provided. Any change rotates the session secret (all other
// sessions die) and re-encrypts the username with the new secret; a fresh
// session cookie is issued for the current request so the caller stays
// logged in.
func (s *Service) HandleAccount(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr
	if host := r.Header.Get("X-Forwarded-For"); host != "" {
		ip = host
	}
	if !s.Limiter.Allow(ip) {
		s.writeError(w, http.StatusTooManyRequests, "err.tooManyAttempts")
		return
	}
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		Username        string `json:"username"`
		NewPassword     string `json:"newPassword"`
	}
	if err := readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "err.invalidRequest")
		return
	}
	if !auth.VerifyPassword(s.Config.PasswordHash, req.CurrentPassword) {
		s.writeError(w, http.StatusUnauthorized, "err.credentialsWrong")
		return
	}
	username := s.Username
	if req.Username != "" {
		if len(req.Username) > 32 {
			s.writeError(w, http.StatusBadRequest, "err.usernameInvalid")
			return
		}
		username = req.Username
	}
	if req.NewPassword != "" && len(req.NewPassword) < 6 {
		s.writeError(w, http.StatusBadRequest, "err.passwordTooShort")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.Config.SessionSecret = store.RandomID(32)
	enc, err := secretbox.Encrypt(s.Config.SessionSecret, username)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	s.Config.UsernameEnc = enc
	if req.NewPassword != "" {
		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "err.hashPassword", err)
			return
		}
		s.Config.PasswordHash = hash
	}
	if err := store.SaveConfig(filepath.Join(s.Dir, store.ConfigFile), s.Config); err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	s.Username = username
	s.Limiter.Reset(ip)

	ttl := s.sessionTTL()
	token := auth.Sign(s.Config.SessionSecret, store.RandomID(12), time.Now().Add(ttl).Unix())
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "username": username})
}

func (s *Service) sessionTTL() time.Duration {
	hours := s.Config.SessionTTLHours
	if hours <= 0 {
		hours = 720
	}
	return time.Duration(hours) * time.Hour
}

// HandleLogout clears the session cookie.
func (s *Service) HandleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HandleSession reports whether the request carries a valid session, and
// which UI language the config uses (so the login page can render the
// right language before authentication).
func (s *Service) HandleSession(w http.ResponseWriter, r *http.Request) {
	_, ok := sessionID(s.Config.SessionSecret, r)
	WriteJSON(w, http.StatusOK, map[string]any{"authed": ok, "lang": s.Config.Language})
}

func sessionID(secret string, r *http.Request) (string, bool) {
	c, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return "", false
	}
	return auth.Verify(secret, c.Value)
}

// HandleSettings updates runtime settings: language, compactCards,
// pageBackground, slogan, internalAddress. Fields left zero (or nil
// pointers) are untouched.
func (s *Service) HandleSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Language        string  `json:"language"`
		Compact         *bool   `json:"compact"`
		PageBackground  string  `json:"pageBackground"`
		Slogan          *string `json:"slogan"`
		InternalAddress *string `json:"internalAddress"`
	}
	if err := readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "err.invalidRequest")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if req.Language != "" {
		lang, ok := i18n.Parse(req.Language)
		if !ok {
			s.writeError(w, http.StatusBadRequest, "err.invalidLang", req.Language)
			return
		}
		s.Config.Language = string(lang)
	}
	if req.Compact != nil {
		s.Config.CompactCards = *req.Compact
	}
	if req.PageBackground != "" {
		if !s.backgroundAllowed(req.PageBackground) {
			s.writeError(w, http.StatusBadRequest, "err.invalidBackground")
			return
		}
		s.Config.PageBackground = req.PageBackground
	}
	if req.Slogan != nil {
		if len(*req.Slogan) > 80 {
			s.writeError(w, http.StatusBadRequest, "err.sloganTooLong")
			return
		}
		s.Config.Slogan = req.Slogan
	}
	if req.InternalAddress != nil {
		addr := strings.TrimSpace(*req.InternalAddress)
		if len(addr) > 253 {
			s.writeError(w, http.StatusBadRequest, "err.addressTooLong")
			return
		}
		s.Config.InternalAddress = addr
	}
	if err := store.SaveConfig(filepath.Join(s.Dir, store.ConfigFile), s.Config); err != nil {
		s.writeError(w, http.StatusInternalServerError, "err.saveFailed", err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"language":        s.Config.Language,
		"compact":         s.Config.CompactCards,
		"pageBackground":  s.Config.PageBackground,
		"slogan":          s.sloganValue(),
		"internalAddress": s.Config.InternalAddress,
	})
}

func (s *Service) sloganValue() string {
	if s.Config.Slogan != nil {
		return *s.Config.Slogan
	}
	return i18n.T(s.lang(), "slogan.default")
}

// appInfo describes a detectable desktop app.
type appInfo struct {
	Installed bool   `json:"installed"`
	Label     string `json:"label"`
}

// HandleMeta lists the icon presets, the selectable backgrounds (presets
// and uploads), the detected desktop apps and current settings, so the UI
// can render pickers. Apps that are not installed are still reported so
// the UI knows they must be hidden.
func (s *Service) HandleMeta(w http.ResponseWriter, r *http.Request) {
	apps := map[string]appInfo{
		store.KindComfyUI: {
			Installed: s.appInstalled(store.KindComfyUI),
			Label:     kindLabel(store.KindComfyUI),
		},
		store.KindOpencode: {
			Installed: s.appInstalled(store.KindOpencode),
			Label:     kindLabel(store.KindOpencode),
		},
		store.KindMdbook: {
			Installed: s.appInstalled(store.KindMdbook),
			Label:     kindLabel(store.KindMdbook),
		},
		store.KindVSCode: {
			Installed: s.appInstalled(store.KindVSCode),
			Label:     kindLabel(store.KindVSCode),
		},
		store.KindVSCodium: {
			Installed: s.appInstalled(store.KindVSCodium),
			Label:     kindLabel(store.KindVSCodium),
		},
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"icons":           assetNames("assets/icons", ".svg"),
		"backgrounds":     s.backgrounds(),
		"apps":            apps,
		"lang":            s.Config.Language,
		"compact":         s.Config.CompactCards,
		"pageBackground":  s.Config.PageBackground,
		"slogan":          s.sloganValue(),
		"username":        s.Username,
		"internalAddress": s.Config.InternalAddress,
	})
}

func assetNames(dir, ext string) []string {
	entries, err := fs.ReadDir(web.Files, dir)
	if err != nil {
		return []string{}
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ext) {
			names = append(names, strings.TrimSuffix(e.Name(), ext))
		}
	}
	return names
}
