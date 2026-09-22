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
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"narthex/internal/auth"
	"narthex/internal/i18n"
	"narthex/internal/store"
)

// Server is the lifecycle of the gateway listener. Starting it binds the
// preferred port (falling back to a kernel-assigned ephemeral port when
// that one is occupied), stopping it closes the listener (and any live
// connections) so no socket or goroutine lingers. The api layer keeps the
// listener bound for the whole serve lifetime so a stale opencode tab
// always gets an HTTP answer (login redirect or stopped page) instead of a
// connection-refused blank page.
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
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, dashboardURL(r, cfg), http.StatusFound)
			return
		}
		t, ok := target()
		if !ok || t.Addr == "" {
			writeStoppedPage(w, lang(), dashboardURL(r, cfg), "gateway.stoppedTitle", "gateway.stoppedBody")
			return
		}
		proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), targetKey{}, t)))
	})
}

// writeStoppedPage answers an authenticated visitor when the proxied
// upstream is gone (stopped, crashed or not started yet) with a small page
// that bounces back to the narthex dashboard, so a stale tab never lands on
// a blank connection-refused page. titleKey/bodyKey select the localized
// copy for the app (opencode or mdBook).
func writeStoppedPage(w http.ResponseWriter, lang i18n.Lang, dash, titleKey, bodyKey string) {
	title := i18n.T(lang, titleKey)
	body := i18n.T(lang, bodyKey)
	link := i18n.T(lang, "gateway.backToDashboard")
	page := "<!doctype html><html lang=\"" + html.EscapeString(string(lang)) + "\">" +
		"<head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
		"<meta http-equiv=\"refresh\" content=\"3;url=" + html.EscapeString(dash) + "\">" +
		"<title>" + html.EscapeString(title) + "</title></head>" +
		"<body style=\"margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;" +
		"background:#0b0d12;color:#e7e9ee;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif\">" +
		"<main style=\"text-align:center;padding:24px\"><h1 style=\"font-size:18px;font-weight:600\">" +
		html.EscapeString(title) + "</h1><p style=\"color:#9aa0ac\">" + html.EscapeString(body) + "</p>" +
		"<p><a style=\"color:#7aa2ff\" href=\"" + html.EscapeString(dash) + "\">" +
		html.EscapeString(link) + "</a></p></main></body></html>"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	io.WriteString(w, page)
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

// dashboardURL points the browser back at the narthex dashboard (and its
// login page). It honors the configured internal address, else the request
// host, following the same rules as the card "Open" URLs.
func dashboardURL(r *http.Request, cfg *store.Config) string {
	if addr := strings.TrimSpace(cfg.InternalAddress); addr != "" {
		addr = strings.TrimPrefix(addr, "http://")
		addr = strings.TrimPrefix(addr, "https://")
		addr = strings.TrimSuffix(addr, "/")
		if _, _, err := net.SplitHostPort(addr); err == nil {
			return "http://" + addr + "/"
		}
		return "http://" + net.JoinHostPort(addr, strconv.Itoa(cfg.Port)) + "/"
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(cfg.Port)) + "/"
}
