package gateway_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"narthex/internal/auth"
	"narthex/internal/gateway"
	"narthex/internal/i18n"
)

func TestCustomPathProxy(t *testing.T) {
	var gotPath, gotQuery, gotCookie string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotCookie = r.Header.Get("Cookie")
		http.SetCookie(w, &http.Cookie{Name: "app", Value: "1", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: auth.SessionCookieName, Value: "evil", Path: "/"})
		w.Header().Set("Location", "/next")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()

	h := gateway.CustomPathHandler(testConfig(), func() i18n.Lang { return i18n.EN },
		func(id string) (string, bool) {
			if id != "abc" {
				return "", false
			}
			return hostOf(upstream), true
		})

	req := httptest.NewRequest("GET", "http://127.0.0.1:9090/custom/abc/foo?x=1", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionCookie()})
	req.AddCookie(&http.Cookie{Name: "keep", Value: "yes"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()

	if gotPath != "/foo" || gotQuery != "x=1" {
		t.Fatalf("upstream path = %q?%q", gotPath, gotQuery)
	}
	if strings.Contains(gotCookie, auth.SessionCookieName) || !strings.Contains(gotCookie, "keep=yes") {
		t.Fatalf("upstream cookie = %q", gotCookie)
	}
	if loc := res.Header.Get("Location"); loc != "/custom/abc/next" {
		t.Fatalf("Location = %q", loc)
	}
	cookies := res.Header.Values("Set-Cookie")
	joined := strings.Join(cookies, "\n")
	if strings.Contains(joined, auth.SessionCookieName) {
		t.Fatalf("session cookie leaked back: %q", joined)
	}
	if !strings.Contains(joined, "Path=/custom/abc/") {
		t.Fatalf("cookie path not rewritten: %q", joined)
	}
}

func TestCustomPathProxyAuthAndStoppedPage(t *testing.T) {
	h := gateway.CustomPathHandler(testConfig(), func() i18n.Lang { return i18n.EN },
		func(id string) (string, bool) { return "", false })

	res := doGetPath(t, h, "", "127.0.0.1:9090", "/custom/abc/")
	if res.StatusCode != http.StatusFound {
		t.Fatalf("unauthenticated status = %d", res.StatusCode)
	}
	res = doGetPath(t, h, sessionCookie(), "127.0.0.1:9090", "/custom/abc/")
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("stopped status = %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "http://127.0.0.1:9090/") {
		t.Fatalf("stopped page missing dashboard link: %s", body)
	}
}

func TestCustomDedicatedProxyStripsSessionCookie(t *testing.T) {
	var gotCookie string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	h := gateway.CustomHandler(testConfig(), func() i18n.Lang { return i18n.EN },
		func() (string, bool) { return hostOf(upstream), true })

	req := httptest.NewRequest("GET", "http://127.0.0.1:26200/", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionCookie()})
	req.AddCookie(&http.Cookie{Name: "keep", Value: "yes"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if strings.Contains(gotCookie, auth.SessionCookieName) || !strings.Contains(gotCookie, "keep=yes") {
		t.Fatalf("upstream cookie = %q", gotCookie)
	}
}
