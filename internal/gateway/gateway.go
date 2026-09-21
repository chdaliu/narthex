// Package gateway fronts a local opencode web server behind narthex.
//
// opencode protects every route with HTTP basic auth and ships its SPA
// assets with a bare `crossorigin` attribute, so browsers drop the
// Authorization header for those subresources and render a blank page
// (or loop on the native auth prompt). The gateway serves opencode on its
// own listener, injects the basic-auth credentials server-side and
// authenticates the visitor with the narthex session cookie instead — a
// same-origin proxy that neither prompts nor breaks assets. The upstream
// opencode endpoint stays bound and password-protected for API clients
// (`opencode attach`, SDK) that connect to it directly.
package gateway

import (
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"

	"narthex/internal/auth"
	"narthex/internal/i18n"
	"narthex/internal/store"
)

// Server is the lifecycle of the gateway listener. It exists only while
// the opencode card runs: starting it binds the preferred port (falling
// back to a kernel-assigned ephemeral port when that one is occupied),
// stopping it closes the listener (and any live connections) so no socket
// or goroutine lingers.
type Server struct {
	mu   sync.Mutex
	srv  *http.Server
	ln   net.Listener
	port int
}

// Start binds hostname:preferredPort and serves handler, so the opencode
// "Open" URL keeps the same port across restarts. When preferredPort is 0
// or already in use, the kernel assigns a free ephemeral port instead. It
// is a no-op when already running, returning the current port.
func (s *Server) Start(hostname string, preferredPort int, handler http.Handler) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		return s.port, nil
	}
	ln, err := listen(hostname, preferredPort)
	if err != nil {
		return 0, err
	}
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		ln.Close()
		return 0, http.ErrNotSupported
	}
	srv := &http.Server{Handler: handler}
	s.srv, s.ln, s.port = srv, ln, tcp.Port
	go func() { _ = srv.Serve(ln) }()
	return s.port, nil
}

// listen binds hostname:preferredPort, or hostname:0 when preferredPort is
// 0 or unavailable.
func listen(hostname string, preferredPort int) (net.Listener, error) {
	if preferredPort > 0 {
		if ln, err := net.Listen("tcp", net.JoinHostPort(hostname, strconv.Itoa(preferredPort))); err == nil {
			return ln, nil
		}
	}
	return net.Listen("tcp", net.JoinHostPort(hostname, "0"))
}

// Stop closes the listener and all active connections; it is a no-op when
// not running.
func (s *Server) Stop() {
	s.mu.Lock()
	srv, ln := s.srv, s.ln
	s.srv, s.ln, s.port = nil, nil, 0
	s.mu.Unlock()
	// Close the listener first so the port is released immediately, even
	// if Serve has not started tracking it yet.
	if ln != nil {
		ln.Close()
	}
	if srv != nil {
		srv.Close()
	}
}

// Port returns the bound port, or 0 when not running.
func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// Target is the opencode upstream the gateway forwards to.
type Target struct {
	// Addr is the loopback host:port of the opencode server.
	Addr string
	// Username and Password are the basic-auth credentials injected into
	// every upstream request.
	Username string
	Password string
}

type targetKey struct{}

// Handler returns the HTTP handler of the opencode gateway. Requests must
// carry a valid narthex session cookie; unauthenticated visitors are
// redirected to the narthex login page. target resolves the upstream on
// every request so start/stop of the opencode card is picked up live.
func Handler(cfg *store.Config, lang func() i18n.Lang, target func() (Target, bool)) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			t, _ := pr.In.Context().Value(targetKey{}).(Target)
			pr.SetURL(&url.URL{Scheme: "http", Host: t.Addr})
			pr.Out.Host = t.Addr
			pr.SetXForwarded()
			// Replace any client credentials with the server-side ones and
			// never leak the narthex session cookie upstream.
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Set("Authorization", basicAuthHeader(t.Username, t.Password))
			pr.Out.Header.Del("Cookie")
		},
		ModifyResponse: func(res *http.Response) error {
			// opencode answers 401 with a challenge the browser would
			// otherwise turn into a native prompt; the gateway always
			// injects credentials, so the challenge is never useful.
			res.Header.Del("WWW-Authenticate")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			io.WriteString(w, i18n.T(lang(), "err.gatewayUnavailable"))
		},
		// Flush immediately so Server-Sent Events (/event) stream.
		FlushInterval: -1,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.Verify(cfg.SessionSecret, cookieValue(r)); !ok {
			http.Redirect(w, r, loginURL(r, cfg), http.StatusFound)
			return
		}
		t, ok := target()
		if !ok || t.Addr == "" {
			http.Error(w, i18n.T(lang(), "err.gatewayUnavailable"), http.StatusServiceUnavailable)
			return
		}
		proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), targetKey{}, t)))
	})
}

func basicAuthHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func cookieValue(r *http.Request) string {
	c, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// loginURL points the browser back at the narthex login page (same host,
// narthex's own port).
func loginURL(r *http.Request, cfg *store.Config) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(cfg.Port)) + "/"
}
