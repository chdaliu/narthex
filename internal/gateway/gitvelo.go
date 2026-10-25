package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"

	"narthex/internal/auth"
	"narthex/internal/i18n"
	"narthex/internal/store"
)

// gitveloTargetKey carries the resolved upstream address through the
// request context.
type gitveloTargetKey struct{}

// GitveloHandler returns the gitvelo reverse proxy. It runs on a dedicated
// listener (like the opencode gateway) because GitVelo serves its SPA from
// root-absolute paths, which would collide with the dashboard mux under a
// prefix.
//
// The upstream GitVelo server runs with --no-auth on loopback (see
// procman.startGitvelo): GitVelo's login is a cookie session that narthex
// cannot inject, so the gateway is the authentication boundary instead.
// Requests must carry a valid narthex session cookie; everyone else is
// redirected to the narthex login page. The narthex cookie is never
// forwarded upstream. target resolves the upstream loopback address on
// every request so start/stop is picked up live, and reports false while
// the card runs in direct (own-login) mode so the gateway never exposes it.
func GitveloHandler(cfg *store.Config, lang func() i18n.Lang, target func() (string, bool)) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			upstream, _ := pr.In.Context().Value(gitveloTargetKey{}).(string)
			pr.SetURL(&url.URL{Scheme: "http", Host: upstream})
			pr.Out.Host = upstream
			pr.SetXForwarded()
			// GitVelo needs no cookies in --no-auth mode; never leak the
			// narthex session.
			pr.Out.Header.Del("Cookie")
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			io.WriteString(w, i18n.T(lang(), "err.gitveloGatewayUnavailable"))
		},
		// Flush immediately so large assets and JSON-RPC responses stream.
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
			writeStoppedPage(w, lang(), dashboardURL(r, cfg), "gateway.gitveloStoppedTitle", "gateway.gitveloStoppedBody")
			return
		}
		proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), gitveloTargetKey{}, upstream)))
	})
}
