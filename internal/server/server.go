// Package server wires the HTTP routes, auth middleware and static files.
package server

import (
	"net/http"
	"path/filepath"
	"strings"

	"narthex/internal/api"
	"narthex/internal/auth"
	"narthex/internal/store"
	"narthex/web"
)

// Server is the HTTP front of narthex.
type Server struct {
	cfg        *store.Config
	svc        *api.Service
	uploadsDir string
}

// New creates a Server.
func New(cfg *store.Config, svc *api.Service) *Server {
	return &Server{cfg: cfg, svc: svc, uploadsDir: svc.UploadsDir()}
}

// Handler builds the root http.Handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/login", s.svc.HandleLogin)
	mux.HandleFunc("POST /api/logout", s.svc.HandleLogout)
	mux.HandleFunc("GET /api/session", s.svc.HandleSession)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/cards", s.svc.HandleListCards)
	protected.HandleFunc("POST /api/cards", s.svc.HandleCreateCard)
	protected.HandleFunc("PATCH /api/cards/{id}", s.svc.HandlePatchCard)
	protected.HandleFunc("DELETE /api/cards/{id}", s.svc.HandleDeleteCard)
	protected.HandleFunc("POST /api/cards/{id}/start", s.svc.HandleStartCard)
	protected.HandleFunc("POST /api/cards/{id}/stop", s.svc.HandleStopCard)
	protected.HandleFunc("GET /api/meta", s.svc.HandleMeta)
	protected.HandleFunc("GET /api/mdbook/projects", s.svc.HandleMdbookProjects)
	protected.HandleFunc("POST /api/mdbook/projects", s.svc.HandleMdbookCreate)
	protected.HandleFunc("POST /api/settings", s.svc.HandleSettings)
	protected.HandleFunc("POST /api/account", s.svc.HandleAccount)
	protected.HandleFunc("GET /api/autostart", s.svc.HandleAutostart)
	protected.HandleFunc("POST /api/autostart", s.svc.HandleAutostart)
	protected.HandleFunc("POST /api/uploads", s.svc.HandleUploadCreate)
	protected.HandleFunc("DELETE /api/uploads/{id}", s.svc.HandleUploadDelete)
	mux.Handle("/api/", s.authMiddleware(protected))

	mux.Handle("/uploads/", s.uploadsHandler())
	mux.Handle("/", staticHandler())
	return mux
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.Verify(s.cfg.SessionSecret, cookieValue(r)); !ok {
			api.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func cookieValue(r *http.Request) string {
	c, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func staticHandler() http.Handler {
	fs := http.FileServerFS(web.Files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fs.ServeHTTP(w, r)
	})
}

// uploadsHandler serves uploaded images from the uploads dir. Filenames
// are sanitized: no path separators, known extension only.
func (s *Server) uploadsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/uploads/")
		if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == "" || !api.UploadExt(ext[1:]) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, filepath.Join(s.uploadsDir, name))
	})
}
