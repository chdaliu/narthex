package gateway_test

import (
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"narthex/internal/auth"
	"narthex/internal/gateway"
	"narthex/internal/i18n"
	"narthex/internal/store"
)

func testConfig() *store.Config {
	return &store.Config{Port: 9090, SessionSecret: "s3cr3t"}
}

func sessionCookie() string {
	return auth.Sign("s3cr3t", "sess", time.Now().Add(time.Hour).Unix())
}

func hostOf(ts *httptest.Server) string {
	return strings.TrimPrefix(ts.URL, "http://")
}

func doGet(t *testing.T, h http.Handler, cookie, host string) *http.Response {
	t.Helper()
	return doGetPath(t, h, cookie, host, "/")
}

func doGetPath(t *testing.T, h http.Handler, cookie, host, path string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("GET", "http://"+host+path, nil)
	req.Host = host
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: cookie})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestGatewayRedirectsUnauthenticated(t *testing.T) {
	h := gateway.Handler(testConfig(), func() i18n.Lang { return i18n.EN },
		func() (gateway.Target, bool) { return gateway.Target{}, false })

	res := doGet(t, h, "", "192.168.1.9:5199")
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "http://192.168.1.9:9090/" {
		t.Fatalf("redirect = %q", loc)
	}
}

func TestGatewayRedirectHonorsInternalAddress(t *testing.T) {
	cfg := testConfig()
	cfg.InternalAddress = "10.0.0.5"
	h := gateway.Handler(cfg, func() i18n.Lang { return i18n.EN },
		func() (gateway.Target, bool) { return gateway.Target{}, false })

	res := doGet(t, h, "", "192.168.1.9:5199")
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "http://10.0.0.5:9090/" {
		t.Fatalf("redirect = %q", loc)
	}
}

func TestGatewayUnavailableWithoutTarget(t *testing.T) {
	h := gateway.Handler(testConfig(), func() i18n.Lang { return i18n.EN },
		func() (gateway.Target, bool) { return gateway.Target{}, false })

	res := doGet(t, h, sessionCookie(), "127.0.0.1:5199")
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content type = %q, want text/html", ct)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "http://127.0.0.1:9090/") {
		t.Fatalf("stopped page missing dashboard link: %s", body)
	}
}

func TestGatewayInjectsCredentialsAndStripsCookie(t *testing.T) {
	var gotAuth, gotCookie string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("WWW-Authenticate", `Basic realm="opencode"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("upstream body"))
	}))
	defer upstream.Close()

	h := gateway.Handler(testConfig(), func() i18n.Lang { return i18n.EN },
		func() (gateway.Target, bool) {
			return gateway.Target{Addr: hostOf(upstream), Username: "opencode", Password: "pw"}, true
		})

	res := doGet(t, h, sessionCookie(), "127.0.0.1:5199")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 passthrough", res.StatusCode)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("opencode:pw"))
	if gotAuth != want {
		t.Fatalf("upstream Authorization = %q, want %q", gotAuth, want)
	}
	if gotCookie != "" {
		t.Fatalf("narthex session cookie leaked upstream: %q", gotCookie)
	}
	if res.Header.Get("WWW-Authenticate") != "" {
		t.Fatalf("WWW-Authenticate must be stripped, got %q", res.Header.Get("WWW-Authenticate"))
	}
}

func TestGatewayBadGatewayWhenUpstreamDown(t *testing.T) {
	// Reserve a port and close it so the upstream connection is refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	h := gateway.Handler(testConfig(), func() i18n.Lang { return i18n.EN },
		func() (gateway.Target, bool) {
			return gateway.Target{Addr: addr, Username: "opencode", Password: "pw"}, true
		})

	res := doGet(t, h, sessionCookie(), "127.0.0.1:5199")
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", res.StatusCode)
	}
}

func TestMdbookRedirectsUnauthenticated(t *testing.T) {
	h := gateway.MdbookHandler(testConfig(), func() i18n.Lang { return i18n.EN },
		func() (string, bool) { return "", false })

	res := doGetPath(t, h, "", "192.168.1.9:9090", "/mdbook/")
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "http://192.168.1.9:9090/" {
		t.Fatalf("redirect = %q", loc)
	}
}

func TestMdbookStoppedPage(t *testing.T) {
	h := gateway.MdbookHandler(testConfig(), func() i18n.Lang { return i18n.EN },
		func() (string, bool) { return "", false })

	res := doGetPath(t, h, sessionCookie(), "127.0.0.1:9090", "/mdbook/")
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "http://127.0.0.1:9090/") {
		t.Fatalf("stopped page missing dashboard link: %s", body)
	}
}

func TestMdbookProxiesAndStripsPrefix(t *testing.T) {
	var gotPath, gotCookie string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCookie = r.Header.Get("Cookie")
		w.Write([]byte("book"))
	}))
	defer upstream.Close()

	h := gateway.MdbookHandler(testConfig(), func() i18n.Lang { return i18n.EN },
		func() (string, bool) { return hostOf(upstream), true })

	res := doGetPath(t, h, sessionCookie(), "127.0.0.1:9090", "/mdbook/chapter_1.html")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if gotPath != "/chapter_1.html" {
		t.Fatalf("upstream path = %q, want /chapter_1.html", gotPath)
	}
	if gotCookie != "" {
		t.Fatalf("narthex session cookie leaked upstream: %q", gotCookie)
	}
}

func TestMdbookPrefixRootMapsToUpstreamRoot(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	}))
	defer upstream.Close()

	h := gateway.MdbookHandler(testConfig(), func() i18n.Lang { return i18n.EN },
		func() (string, bool) { return hostOf(upstream), true })

	if res := doGetPath(t, h, sessionCookie(), "127.0.0.1:9090", "/mdbook/"); res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if gotPath != "/" {
		t.Fatalf("upstream path = %q, want /", gotPath)
	}
}

func TestMdbookLivereloadPathPassesThrough(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	}))
	defer upstream.Close()

	h := gateway.MdbookHandler(testConfig(), func() i18n.Lang { return i18n.EN },
		func() (string, bool) { return hostOf(upstream), true })

	if res := doGetPath(t, h, sessionCookie(), "127.0.0.1:9090", "/__livereload"); res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if gotPath != "/__livereload" {
		t.Fatalf("upstream path = %q, want /__livereload", gotPath)
	}
}

func TestServerLifecycle(t *testing.T) {
	var s gateway.Server
	if p := s.Port(); p != 0 {
		t.Fatalf("idle port = %d, want 0", p)
	}

	// A preferred free port is honored, so the opencode "Open" URL stays
	// stable across restarts.
	pref := freePort(t)
	p, err := s.Start("127.0.0.1", pref, http.NotFoundHandler())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if p != pref || s.Port() != p {
		t.Fatalf("port = %d, server = %d, want %d", p, s.Port(), pref)
	}

	// Starting again is a no-op that keeps the same port.
	p2, err := s.Start("127.0.0.1", pref, http.NotFoundHandler())
	if err != nil || p2 != p {
		t.Fatalf("second start = %d, %v; want %d", p2, err, p)
	}

	s.Stop()
	if s.Port() != 0 {
		t.Fatalf("port after stop = %d, want 0", s.Port())
	}
	// The listener must be released.
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
	if err != nil {
		t.Fatalf("port still bound after stop: %v", err)
	}
	ln.Close()
}

// TestServerPreferredPortFallback checks that an occupied preferred port
// makes the gateway fall back to an ephemeral one instead of failing.
func TestServerPreferredPortFallback(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port

	var s gateway.Server
	p, err := s.Start("127.0.0.1", busyPort, http.NotFoundHandler())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Stop()
	if p <= 0 || p == busyPort {
		t.Fatalf("fallback port = %d, want a free port != %d", p, busyPort)
	}
}

// freePort returns a currently free loopback port.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return p
}
