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

// CustomPrefix is the same-origin path under which a custom card in path
// proxy mode is served: /custom/<cardID>/.
const CustomPrefix = "/custom"

type customTargetKey struct{}
type customPrefixKey struct{}

// CustomPathHandler returns the same-origin reverse proxy for custom cards in
// path proxy mode. The card ID is taken from the first path segment after
// /custom/ and target resolves its running loopback address per request.
func CustomPathHandler(cfg *store.Config, lang func() i18n.Lang, target func(id string) (string, bool)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, CustomPrefix+"/")
		id, tail, _ := strings.Cut(rest, "/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		prefix := CustomPrefix + "/" + id
		if !strings.HasPrefix(r.URL.Path, prefix+"/") {
			// Canonicalize /custom/<id> to /custom/<id>/ so relative links
			// resolve under the card prefix.
			http.Redirect(w, r, prefix+"/"+tail, http.StatusFound)
			return
		}
		if _, ok := auth.Verify(cfg.SessionSecret, cookieValue(r)); !ok {
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, dashboardURL(r, cfg), http.StatusFound)
			return
		}
		upstream, ok := target(id)
		if !ok || upstream == "" {
			writeStoppedPage(w, lang(), dashboardURL(r, cfg), "gateway.customStoppedTitle", "gateway.customStoppedBody")
			return
		}
		ctx := context.WithValue(r.Context(), customTargetKey{}, upstream)
		ctx = context.WithValue(ctx, customPrefixKey{}, prefix)
		serveCustomProxy(w, r.WithContext(ctx), lang, prefix != "")
	})
}

// CustomHandler returns the dedicated-listener reverse proxy for one custom
// card in gateway proxy mode. target resolves the running card's loopback
// address per request.
func CustomHandler(cfg *store.Config, lang func() i18n.Lang, target func() (string, bool)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.Verify(cfg.SessionSecret, cookieValue(r)); !ok {
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, dashboardURL(r, cfg), http.StatusFound)
			return
		}
		upstream, ok := target()
		if !ok || upstream == "" {
			writeStoppedPage(w, lang(), dashboardURL(r, cfg), "gateway.customStoppedTitle", "gateway.customStoppedBody")
			return
		}
		ctx := context.WithValue(r.Context(), customTargetKey{}, upstream)
		ctx = context.WithValue(ctx, customPrefixKey{}, "")
		serveCustomProxy(w, r.WithContext(ctx), lang, false)
	})
}

// serveCustomProxy builds the shared reverse proxy. pathMode enables the
// cookie-path and Location rewrites needed under /custom/<id>/.
func serveCustomProxy(w http.ResponseWriter, r *http.Request, lang func() i18n.Lang, pathMode bool) {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			upstream, _ := pr.In.Context().Value(customTargetKey{}).(string)
			prefix, _ := pr.In.Context().Value(customPrefixKey{}).(string)
			pr.SetURL(&url.URL{Scheme: "http", Host: upstream})
			pr.Out.Host = upstream
			pr.SetXForwarded()
			if pathMode && prefix != "" {
				p := strings.TrimPrefix(pr.Out.URL.Path, prefix)
				if p == "" {
					p = "/"
				}
				pr.Out.URL.Path = p
				pr.Out.URL.RawPath = ""
				pr.Out.Header.Set("X-Forwarded-Prefix", prefix)
			}
			// Never leak the narthex session cookie upstream, but preserve
			// the custom app's own cookies.
			if c := filterCookieHeader(pr.In); c != "" {
				pr.Out.Header.Set("Cookie", c)
			} else {
				pr.Out.Header.Del("Cookie")
			}
		},
		ModifyResponse: func(res *http.Response) error {
			prefix, _ := res.Request.Context().Value(customPrefixKey{}).(string)
			upstream, _ := res.Request.Context().Value(customTargetKey{}).(string)
			rewriteCustomCookies(res, prefix)
			if pathMode && prefix != "" {
				if loc := res.Header.Get("Location"); loc != "" {
					res.Header.Set("Location", customLocation(loc, prefix, upstream))
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			io.WriteString(w, i18n.T(lang(), "gateway.customStoppedTitle"))
		},
		FlushInterval: -1,
	}
	proxy.ServeHTTP(w, r)
}

// filterCookieHeader returns the request's Cookie header without the narthex
// session cookie.
func filterCookieHeader(r *http.Request) string {
	cookies := r.Cookies()
	kept := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c.Name == auth.SessionCookieName {
			continue
		}
		kept = append(kept, c.Name+"="+c.Value)
	}
	return strings.Join(kept, "; ")
}

// rewriteCustomCookies drops any upstream cookie trying to impersonate the
// narthex session and, in path mode, scopes the app's cookies under the card
// prefix. Cookies ignore ports, so gateway mode cannot path-isolate them.
func rewriteCustomCookies(res *http.Response, prefix string) {
	raw := res.Header.Values("Set-Cookie")
	if len(raw) == 0 {
		return
	}
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		c, err := http.ParseSetCookie(line)
		if err != nil {
			continue
		}
		if c.Name == auth.SessionCookieName {
			continue
		}
		c.Domain = ""
		if prefix != "" {
			switch {
			case c.Path == "" || c.Path == "/":
				c.Path = prefix + "/"
			case strings.HasPrefix(c.Path, prefix+"/") || c.Path == prefix:
				// Already scoped.
			default:
				c.Path = prefix + c.Path
			}
		}
		out = append(out, c.String())
	}
	res.Header.Del("Set-Cookie")
	for _, c := range out {
		res.Header.Add("Set-Cookie", c)
	}
}

// customLocation rewrites an upstream redirect to the card prefix.
func customLocation(loc, prefix, upstream string) string {
	if strings.HasPrefix(loc, prefix+"/") || loc == prefix {
		return loc
	}
	if strings.HasPrefix(loc, "/") {
		return prefix + loc
	}
	u, err := url.Parse(loc)
	if err != nil || u.Host == "" {
		return loc
	}
	if u.Host != upstream {
		return loc
	}
	p := prefix + u.Path
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	return p
}
