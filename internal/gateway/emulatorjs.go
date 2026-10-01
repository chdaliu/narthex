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

// EmulatorPrefix is the same-origin path under which the running EmulatorJS
// server is proxied on the narthex dashboard listener. It lets the player be
// reached through the same origin — and the same reverse proxy or tunnel —
// as the dashboard, with no extra port to expose.
const EmulatorPrefix = "/emulator"

// emulatorTargetKey carries the resolved upstream address through the
// request context.
type emulatorTargetKey struct{}

// EmulatorHandler returns the same-origin reverse proxy for the EmulatorJS
// child server. Requests must carry a valid narthex session cookie; everyone
// else is redirected to the login page. target resolves the upstream loopback
// address on every request so start/stop is picked up live.
//
// The child serves the player page at "/" and the ROM/data under it, so only
// the prefix is stripped before forwarding.
func EmulatorHandler(cfg *store.Config, lang func() i18n.Lang, target func() (string, bool)) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			upstream, _ := pr.In.Context().Value(emulatorTargetKey{}).(string)
			pr.SetURL(&url.URL{Scheme: "http", Host: upstream})
			pr.Out.Host = upstream
			pr.SetXForwarded()
			p := strings.TrimPrefix(pr.Out.URL.Path, EmulatorPrefix)
			if p == "" {
				p = "/"
			}
			pr.Out.URL.Path = p
			pr.Out.URL.RawPath = ""
			// EmulatorJS needs no cookies; never leak the narthex session.
			pr.Out.Header.Del("Cookie")
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			io.WriteString(w, i18n.T(lang(), "gateway.emulatorStoppedTitle"))
		},
		// Flush immediately so large ROM/asset transfers are not buffered.
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
			writeStoppedPage(w, lang(), dashboardURL(r, cfg), "gateway.emulatorStoppedTitle", "gateway.emulatorStoppedBody")
			return
		}
		proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), emulatorTargetKey{}, upstream)))
	})
}
