package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"narthex/internal/auth"
	"narthex/internal/i18n"
	"narthex/internal/store"
)

// MdbookPrefix is the same-origin path under which the running mdBook
// server is proxied on the narthex dashboard listener. It lets the book be
// reached through the same origin — and the same reverse proxy or tunnel —
// as the dashboard, with no extra port to expose.
const MdbookPrefix = "/mdbook"

// mdbookTargetKey carries the resolved upstream address through the
// request context.
type mdbookTargetKey struct{}

// MdbookHandler returns the same-origin reverse proxy for the mdBook
// server. Requests must carry a valid narthex session cookie; everyone
// else is redirected to the login page. target resolves the upstream
// loopback address on every request so start/stop is picked up live.
//
// mdBook serves its book from the site root using relative links and a
// root-absolute live-reload websocket (`/__livereload`), both of which the
// dashboard mux routes here: the prefix is stripped before forwarding and
// the websocket path passes through untouched.
func MdbookHandler(cfg *store.Config, lang func() i18n.Lang, target func() (string, bool)) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			upstream, _ := pr.In.Context().Value(mdbookTargetKey{}).(string)
			pr.SetURL(&url.URL{Scheme: "http", Host: upstream})
			pr.Out.Host = upstream
			pr.SetXForwarded()
			p := strings.TrimPrefix(pr.Out.URL.Path, MdbookPrefix)
			if p == "" {
				p = "/"
			}
			pr.Out.URL.Path = p
			pr.Out.URL.RawPath = ""
			// mdBook needs no cookies; never leak the narthex session.
			pr.Out.Header.Del("Cookie")
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			io.WriteString(w, i18n.T(lang(), "gateway.mdbookStoppedTitle"))
		},
		// Flush immediately so the live-reload websocket and large assets
		// are not buffered.
		FlushInterval: -1,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.Verify(cfg.SessionSecret, cookieValue(r)); !ok {
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, dashboardURL(r, cfg), http.StatusFound)
			return
		}
		upstream, ok := target()
		if !ok || upstream == "" {
			writeStoppedPage(w, lang(), dashboardURL(r, cfg), "gateway.mdbookStoppedTitle", "gateway.mdbookStoppedBody")
			return
		}
		proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), mdbookTargetKey{}, upstream)))
	})
}
