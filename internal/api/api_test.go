package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"narthex/internal/api"
	"narthex/internal/auth"
	"narthex/internal/i18n"
	"narthex/internal/procman"
	"narthex/internal/server"
	"narthex/internal/store"
)

// multipartBody builds a multipart form body with a single file field.
func multipartBody(t *testing.T, field, filename string, content []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(content)
	w.Close()
	return &buf, w.FormDataContentType()
}

const testPassword = "secret-123"

// fakeBackend records calls and simulates one managed instance.
type fakeBackend struct {
	mu      sync.Mutex
	started []string
	stopped []int
	alive   bool
	healthy bool
	mem     int64
	// urlPort is the port reported for running instances.
	urlPort int
}

func (f *fakeBackend) Start(kind, dir, id string) (int, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, kind+"|"+dir+"|"+id)
	f.alive = true
	f.healthy = true
	f.mem = 42000
	port := f.urlPort
	if port == 0 {
		port = 8000
	}
	return 1234, port, nil
}

func (f *fakeBackend) Stop(pid int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, pid)
	f.alive = false
	f.healthy = false
	return nil
}

func (f *fakeBackend) Status(kind string, pid, port int) procman.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return procman.Status{Alive: f.alive, Healthy: f.healthy, MemoryKB: f.mem}
}

func (f *fakeBackend) startCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.started...)
}

func (f *fakeBackend) stopCalls() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.stopped...)
}

type testEnv struct {
	base    string
	backend *fakeBackend
	root    string
}

// comfyInstalled configures both comfyui detection seams (the desktop
// app and a managed install) so comfyui is reported as installed.
func comfyInstalled(s *api.Service, dir string) {
	s.ComfyUIAppDir = func() string { return "/Applications/Comfy Desktop.app" }
	s.ComfyUIDirs = func() []string { return []string{dir} }
}

// opencodeInstalled configures the opencode detection seam so opencode
// is reported as installed.
func opencodeInstalled(s *api.Service) {
	s.OpencodeBin = func() string { return "/usr/bin/opencode" }
}

// mdbookInstalled configures the mdbook seams: the CLI is present, the
// configured project directory is booksDir and the real filesystem scan
// runs against it. bookDir is an existing recognized project; MdbookCreate
// actually creates a real book (book.toml) so the whole flow round-trips.
func mdbookInstalled(s *api.Service, booksDir, bookDir string) {
	s.MdbookBin = func() string { return "/usr/bin/mdbook" }
	s.MdbookProjects = procman.DetectMdbookProjects
	s.MdbookCreate = func(parent, name string) error {
		dir := filepath.Join(parent, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "book.toml"), []byte("[book]\n"), 0o600)
	}
	s.Config.Mdbook.Dirs = []string{booksDir}
	if bookDir != "" {
		_ = os.MkdirAll(bookDir, 0o755)
		_ = os.WriteFile(filepath.Join(bookDir, "book.toml"), []byte("[book]\n"), 0o600)
	}
}

// vscodeInstalled configures the vscode detection seam so the VS Code
// web server kind is reported as installed.
func vscodeInstalled(s *api.Service) {
	s.VscodeBin = func() string { return "/usr/bin/code" }
}

// vscodiumInstalled configures the vscodium detection seam so the
// VSCodium web server kind is reported as installed.
func vscodiumInstalled(s *api.Service) {
	s.VscodiumBin = func() string { return "/usr/bin/codium" }
}

// wettyInstalled configures the wetty detection seam so the WeTTY
// terminal-over-web kind is reported as installed.
func wettyInstalled(s *api.Service) {
	s.WettyBin = func() string { return "/usr/bin/wetty" }
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	return newTestEnvWith(t, nil)
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

func newTestEnvWith(t *testing.T, mutate func(*store.Config)) *testEnv {
	t.Helper()
	return newTestEnvFull(t, mutate, nil)
}

func newTestEnvFull(t *testing.T, mutateCfg func(*store.Config), mutateSvc func(*api.Service)) *testEnv {
	t.Helper()
	root := t.TempDir()
	cfg := &store.Config{
		Hostname:        "127.0.0.1",
		Port:            9090,
		SessionSecret:   store.RandomID(16),
		Language:        "en",
		SessionTTLHours: 720,
		PageBackground:  "bg-05",
	}
	if mutateCfg != nil {
		mutateCfg(cfg)
	}
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	cfg.PasswordHash = hash

	backend := &fakeBackend{}
	svc := api.NewService(t.TempDir(), cfg, &store.State{}, backend, "narthex")
	if mutateSvc != nil {
		mutateSvc(svc)
	}
	ts := httptest.NewServer(server.New(cfg, svc).Handler())
	t.Cleanup(ts.Close)
	return &testEnv{base: ts.URL, backend: backend, root: root}
}

func doReq(t *testing.T, method, url, cookie string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	json.NewDecoder(res.Body).Decode(&m)
	return res, m
}

func loginCookie(t *testing.T, base, password string) string {
	t.Helper()
	return loginCookieAs(t, base, "narthex", password)
}

func loginCookieAs(t *testing.T, base, username, password string) string {
	t.Helper()
	c := loginCookieObjAs(t, base, username, password)
	return c.Name + "=" + c.Value
}

func loginCookieObj(t *testing.T, base, password string) *http.Cookie {
	t.Helper()
	return loginCookieObjAs(t, base, "narthex", password)
}

func loginCookieObjAs(t *testing.T, base, username, password string) *http.Cookie {
	t.Helper()
	res, _ := doReq(t, "POST", base+"/api/login", "", map[string]string{"username": username, "password": password})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login failed: %d", res.StatusCode)
	}
	for _, c := range res.Cookies() {
		if c.Name == auth.SessionCookieName {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func cardID(t *testing.T, m map[string]any) string {
	t.Helper()
	id, _ := m["id"].(string)
	if id == "" {
		t.Fatalf("no card id in response: %v", m)
	}
	return id
}

func cardsList(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, _ := m["cards"].([]any)
	list := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if c, ok := r.(map[string]any); ok {
			list = append(list, c)
		}
	}
	return list
}

func TestAuthFlow(t *testing.T) {
	env := newTestEnv(t)

	res, _ := doReq(t, "GET", env.base+"/api/cards", "", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated cards = %d, want 401", res.StatusCode)
	}

	res, _ = doReq(t, "POST", env.base+"/api/login", "", map[string]string{"username": "narthex", "password": "wrong"})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", res.StatusCode)
	}

	// Wrong username is rejected with the same error (no enumeration).
	res, m := doReq(t, "POST", env.base+"/api/login", "", map[string]string{"username": "nobody", "password": testPassword})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong username = %d, want 401", res.StatusCode)
	}
	if m["error"] != i18n.T(i18n.EN, "err.credentialsWrong") {
		t.Fatalf("wrong username error = %v", m["error"])
	}

	c := loginCookieObj(t, env.base, testPassword)
	if c.MaxAge != 720*3600 {
		t.Fatalf("default session MaxAge = %d, want %d (30 days)", c.MaxAge, 720*3600)
	}
	cookie := c.Name + "=" + c.Value

	// Default session TTL: 720 hours = 30 days.
	res, m = doReq(t, "GET", env.base+"/api/session", cookie, nil)
	if res.StatusCode != http.StatusOK || m["authed"] != true {
		t.Fatalf("session should be authed: %d %v", res.StatusCode, m)
	}
	res, _ = doReq(t, "POST", env.base+"/api/logout", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("logout = %d", res.StatusCode)
	}
	cleared := false
	for _, c := range res.Cookies() {
		if c.Name == auth.SessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatalf("logout should clear the session cookie, got: %v", res.Header["Set-Cookie"])
	}
	// Note: sessions are stateless HMAC tokens, so the old token itself
	// remains valid until expiry (documented tradeoff); logout clears it
	// on the client side.

	res, _ = doReq(t, "GET", env.base+"/", "", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("index page = %d", res.StatusCode)
	}
}

func TestCardsCRUD(t *testing.T) {
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		comfyInstalled(s, "/path/to/comfy/ComfyUI")
	})
	cookie := loginCookie(t, env.base, testPassword)

	res, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "comfyui"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create = %d %v", res.StatusCode, m)
	}
	id := cardID(t, m)
	if m["icon"] != "palette" || m["background"] != "bg-01" || m["name"] != "ComfyUI" {
		t.Fatalf("defaults wrong: %v", m)
	}
	if m["running"] != false {
		t.Fatalf("new card should be stopped: %v", m)
	}

	// A second card of the same kind is rejected (kind is the identity).
	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "comfyui"})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate kind = %d, want 409", res.StatusCode)
	}

	// Unknown kind is rejected.
	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "wechat"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown kind = %d, want 400", res.StatusCode)
	}

	res, m = doReq(t, "PATCH", env.base+"/api/cards/"+id, cookie, map[string]string{
		"name": "renamed", "icon": "rocket", "background": "bg-02",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("patch = %d %v", res.StatusCode, m)
	}
	if m["name"] != "renamed" || m["icon"] != "rocket" || m["background"] != "bg-02" {
		t.Fatalf("patch not applied: %v", m)
	}

	res, _ = doReq(t, "PATCH", env.base+"/api/cards/nope", cookie, map[string]string{"name": "x"})
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("patch unknown = %d, want 404", res.StatusCode)
	}

	res, m = doReq(t, "GET", env.base+"/api/cards", cookie, nil)
	if res.StatusCode != http.StatusOK || len(cardsList(t, m)) != 1 {
		t.Fatalf("list = %d %v", res.StatusCode, m)
	}

	res, _ = doReq(t, "DELETE", env.base+"/api/cards/"+id, cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("delete = %d", res.StatusCode)
	}
	res, m = doReq(t, "GET", env.base+"/api/cards", cookie, nil)
	if len(cardsList(t, m)) != 0 {
		t.Fatalf("card should be gone: %v", m)
	}

	res, _ = doReq(t, "DELETE", env.base+"/api/cards/"+id, cookie, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("delete unknown = %d, want 404", res.StatusCode)
	}
}

func TestCardLifecycle(t *testing.T) {
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		comfyInstalled(s, "/path/to/comfy/ComfyUI")
	})
	cookie := loginCookie(t, env.base, testPassword)

	res, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "comfyui"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create = %d %v", res.StatusCode, m)
	}
	id := cardID(t, m)

	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start = %d %v", res.StatusCode, m)
	}
	if m["running"] != true || m["healthy"] != true || m["memoryKB"] != float64(42000) {
		t.Fatalf("started view wrong: %v", m)
	}
	// The web URL points at the ComfyUI Desktop web port.
	if m["url"] != "http://127.0.0.1:8000/" {
		t.Fatalf("url wrong: %v", m)
	}
	if m["port"] != float64(8000) || m["pid"] != float64(1234) {
		t.Fatalf("pid/port wrong: %v", m)
	}
	calls := env.backend.startCalls()
	if len(calls) != 1 || calls[0] != "comfyui|/path/to/comfy/ComfyUI|"+id {
		t.Fatalf("start calls = %v", calls)
	}

	res, _ = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("second start = %d", res.StatusCode)
	}
	if len(env.backend.startCalls()) != 1 {
		t.Fatal("second start should be a no-op (already running)")
	}

	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/stop", cookie, nil)
	if res.StatusCode != http.StatusOK || m["running"] != false {
		t.Fatalf("stop = %d %v", res.StatusCode, m)
	}
	if stops := env.backend.stopCalls(); len(stops) != 1 || stops[0] != 1234 {
		t.Fatalf("stop calls = %v", stops)
	}

	res, _ = doReq(t, "DELETE", env.base+"/api/cards/"+id, cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("delete = %d", res.StatusCode)
	}
	if len(env.backend.stopCalls()) != 1 {
		t.Fatal("deleting a stopped card should not stop again")
	}
}

func TestCardRequiresInstalledApp(t *testing.T) {
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		s.ComfyUIAppDir = func() string { return "" }
		s.ComfyUIDirs = func() []string { return nil }
		s.OpencodeBin = func() string { return "" }
		s.MdbookBin = func() string { return "" }
		s.VscodeBin = func() string { return "" }
		s.VscodiumBin = func() string { return "" }
		s.WettyBin = func() string { return "" }
	})
	cookie := loginCookie(t, env.base, testPassword)

	// The app is not installed → creating a card of that kind is rejected.
	res, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "comfyui"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create comfyui without app = %d, want 400 (%v)", res.StatusCode, m)
	}
	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "opencode"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create opencode without app = %d, want 400 (%v)", res.StatusCode, m)
	}
	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "vscode"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create vscode without app = %d, want 400 (%v)", res.StatusCode, m)
	}
	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "vscodium"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create vscodium without app = %d, want 400 (%v)", res.StatusCode, m)
	}
	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "wetty"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create wetty without app = %d, want 400 (%v)", res.StatusCode, m)
	}

	// The meta reports the apps as not installed, so the UI hides them.
	res, m = doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	apps, _ := m["apps"].(map[string]any)
	comfy, _ := apps["comfyui"].(map[string]any)
	if comfy == nil || comfy["installed"] != false {
		t.Fatalf("meta apps.comfyui = %v", apps)
	}
	oc, _ := apps["opencode"].(map[string]any)
	if oc == nil || oc["installed"] != false {
		t.Fatalf("meta apps.opencode = %v", apps)
	}
	vc, _ := apps["vscode"].(map[string]any)
	if vc == nil || vc["installed"] != false {
		t.Fatalf("meta apps.vscode = %v", apps)
	}
	cd, _ := apps["vscodium"].(map[string]any)
	if cd == nil || cd["installed"] != false {
		t.Fatalf("meta apps.vscodium = %v", apps)
	}
	wt, _ := apps["wetty"].(map[string]any)
	if wt == nil || wt["installed"] != false {
		t.Fatalf("meta apps.wetty = %v", apps)
	}
}

func TestLoginRateLimit(t *testing.T) {
	env := newTestEnv(t)
	for i := 0; i < 5; i++ {
		res, _ := doReq(t, "POST", env.base+"/api/login", "", map[string]string{"username": "narthex", "password": "wrong"})
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, res.StatusCode)
		}
	}
	res, m := doReq(t, "POST", env.base+"/api/login", "", map[string]string{"username": "narthex", "password": "wrong"})
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("6th attempt = %d, want 429 (%v)", res.StatusCode, m)
	}
}

func TestLoginSuccessResetsQuota(t *testing.T) {
	env := newTestEnv(t)
	for i := 0; i < 4; i++ {
		res, _ := doReq(t, "POST", env.base+"/api/login", "", map[string]string{"username": "narthex", "password": "wrong"})
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, res.StatusCode)
		}
	}
	// A correct password on the 5th attempt must succeed and reset the
	// window: afterwards the full quota is available again.
	res, _ := doReq(t, "POST", env.base+"/api/login", "", map[string]string{"username": "narthex", "password": testPassword})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("correct password = %d, want 200", res.StatusCode)
	}
	for i := 0; i < 5; i++ {
		res, _ = doReq(t, "POST", env.base+"/api/login", "", map[string]string{"username": "narthex", "password": "wrong"})
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("after reset, wrong attempt %d = %d, want 401", i+1, res.StatusCode)
		}
	}
	res, _ = doReq(t, "POST", env.base+"/api/login", "", map[string]string{"username": "narthex", "password": "wrong"})
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("quota exhausted again = %d, want 429", res.StatusCode)
	}
}

func TestSessionTTLConfigured(t *testing.T) {
	env := newTestEnvWith(t, func(cfg *store.Config) {
		cfg.SessionTTLHours = 1
	})
	c := loginCookieObj(t, env.base, testPassword)
	if c.MaxAge != 3600 {
		t.Fatalf("configured session MaxAge = %d, want 3600", c.MaxAge)
	}
}

func TestSettingsLanguage(t *testing.T) {
	env := newTestEnv(t)
	cookie := loginCookie(t, env.base, testPassword)

	res, _ := doReq(t, "POST", env.base+"/api/settings", "", map[string]string{"language": "zh-TW"})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("settings without auth = %d, want 401", res.StatusCode)
	}

	res, _ = doReq(t, "POST", env.base+"/api/settings", cookie, map[string]string{"language": "fr"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid language = %d, want 400", res.StatusCode)
	}

	res, m := doReq(t, "POST", env.base+"/api/settings", cookie, map[string]string{"language": "zh-TW"})
	if res.StatusCode != http.StatusOK || m["language"] != "zh-TW" {
		t.Fatalf("settings = %d %v", res.StatusCode, m)
	}

	res, m = doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	if m["lang"] != "zh-TW" {
		t.Fatalf("meta lang = %v, want zh-TW", m["lang"])
	}

	// Error messages follow the configured language.
	_, m = doReq(t, "POST", env.base+"/api/login", "", map[string]string{"username": "narthex", "password": "wrong"})
	if m["error"] != i18n.T(i18n.ZHTW, "err.credentialsWrong") {
		t.Fatalf("error should be localized to zh-TW, got %v", m["error"])
	}

	// The default (uncustomized) slogan follows the language.
	res, m = doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	if m["slogan"] != i18n.T(i18n.ZHTW, "slogan.default") {
		t.Fatalf("slogan should follow language, got %v", m["slogan"])
	}

	res, _ = doReq(t, "POST", env.base+"/api/settings", cookie, map[string]string{"language": "en"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("switch back = %d", res.StatusCode)
	}
}

func TestIsLoopback(t *testing.T) {
	for _, h := range []string{"127.0.0.1", "localhost", "::1", "", "127.0.0.1:9090", "[::1]:9090"} {
		if !api.IsLoopback(h) {
			t.Errorf("IsLoopback(%q) should be true", h)
		}
	}
	for _, h := range []string{"0.0.0.0", "192.168.1.5", "example.com", "0.0.0.0:9090", "example.com:9090"} {
		if api.IsLoopback(h) {
			t.Errorf("IsLoopback(%q) should be false", h)
		}
	}
}

func TestCardKinds(t *testing.T) {
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		comfyInstalled(s, "/path/to/comfy/ComfyUI")
		opencodeInstalled(s)
	})
	cookie := loginCookie(t, env.base, testPassword)

	// The default kind is comfyui.
	res, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create = %d %v", res.StatusCode, m)
	}
	if m["kind"] != "comfyui" {
		t.Fatalf("default kind = %v, want comfyui", m["kind"])
	}
	if m["icon"] != "palette" {
		t.Fatalf("comfyui default icon = %v, want palette", m["icon"])
	}
	if m["name"] != "ComfyUI" {
		t.Fatalf("comfyui default name = %v, want ComfyUI", m["name"])
	}

	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "comfyui"})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate comfyui = %d, want 409 (%v)", res.StatusCode, m)
	}

	// Each kind can be added exactly once.
	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "opencode"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create opencode = %d %v", res.StatusCode, m)
	}
	if m["kind"] != "opencode" || m["icon"] != "terminal" || m["name"] != "OpenCode" {
		t.Fatalf("opencode defaults = %v", m)
	}
	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "opencode"})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate opencode = %d, want 409 (%v)", res.StatusCode, m)
	}

	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "wechat"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown kind = %d, want 400 (%v)", res.StatusCode, m)
	}
}

func TestOpencodeCardShowsCredentials(t *testing.T) {
	env := newTestEnvFull(t, func(cfg *store.Config) {
		cfg.Opencode.Password = "oc-secret"
	}, func(s *api.Service) {
		comfyInstalled(s, "/path/to/comfy/ComfyUI")
		opencodeInstalled(s)
		s.GatewayPort = 5199
	})
	cookie := loginCookie(t, env.base, testPassword)

	res, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "opencode"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create opencode = %d %v", res.StatusCode, m)
	}
	if m["username"] != store.OpencodeUsername || m["password"] != "oc-secret" {
		t.Fatalf("opencode card should expose the web credentials, got %v", m)
	}

	// The "Open" URL points at the narthex gateway (no embedded
	// credentials); the direct API URL names the opencode listen port.
	id := cardID(t, m)
	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start opencode = %d %v", res.StatusCode, m)
	}
	if m["url"] != "http://127.0.0.1:5199/" {
		t.Fatalf("opencode url should point at the gateway, got %v", m["url"])
	}
	if m["apiUrl"] != "http://127.0.0.1:8000/" {
		t.Fatalf("opencode apiUrl should name the direct endpoint, got %v", m["apiUrl"])
	}

	// The comfyui card must NOT expose credentials.
	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "comfyui"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create comfyui = %d %v", res.StatusCode, m)
	}
	if _, ok := m["password"]; ok {
		t.Fatalf("comfyui card must not expose a password: %v", m)
	}
	if _, ok := m["username"]; ok {
		t.Fatalf("comfyui card must not expose a username: %v", m)
	}
}

func TestMetaApps(t *testing.T) {
	// ComfyUI Desktop and the opencode CLI installed.
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		comfyInstalled(s, "/path/to/comfy/ComfyUI")
		opencodeInstalled(s)
	})
	cookie := loginCookie(t, env.base, testPassword)

	res, m := doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("meta = %d", res.StatusCode)
	}
	apps, _ := m["apps"].(map[string]any)
	if apps == nil {
		t.Fatalf("meta should include apps: %v", m)
	}
	comfy, _ := apps["comfyui"].(map[string]any)
	if comfy == nil || comfy["installed"] != true || comfy["label"] != "ComfyUI" {
		t.Fatalf("meta apps.comfyui = %v", apps)
	}
	oc, _ := apps["opencode"].(map[string]any)
	if oc == nil || oc["installed"] != true || oc["label"] != "OpenCode" {
		t.Fatalf("meta apps.opencode = %v", apps)
	}
}

func TestMetaAppReasons(t *testing.T) {
	t.Run("mdbook cli without dirs", func(t *testing.T) {
		env := newTestEnvFull(t, nil, func(s *api.Service) {
			s.MdbookBin = func() string { return "/usr/bin/mdbook" }
			s.Config.Mdbook.Dirs = []string{}
		})
		cookie := loginCookie(t, env.base, testPassword)
		_, m := doReq(t, "GET", env.base+"/api/meta", cookie, nil)
		apps, _ := m["apps"].(map[string]any)
		mb, _ := apps["mdbook"].(map[string]any)
		if mb["installed"] != false || mb["reason"] != "add.reason.mdbookNoDirs" {
			t.Fatalf("mdbook reason = %v", mb)
		}
	})

	t.Run("comfy desktop without an instance", func(t *testing.T) {
		env := newTestEnvFull(t, nil, func(s *api.Service) {
			s.ComfyUIAppDir = func() string { return "/Applications/Comfy Desktop.app" }
			s.ComfyUIDirs = func() []string { return nil }
		})
		cookie := loginCookie(t, env.base, testPassword)
		_, m := doReq(t, "GET", env.base+"/api/meta", cookie, nil)
		apps, _ := m["apps"].(map[string]any)
		cf, _ := apps["comfyui"].(map[string]any)
		if cf["installed"] != false || cf["reason"] != "add.reason.comfyNoInstall" {
			t.Fatalf("comfyui reason = %v", cf)
		}
	})

	t.Run("undetected kinds report notInstalled", func(t *testing.T) {
		env := newTestEnvFull(t, nil, func(s *api.Service) {
			s.ComfyUIAppDir = func() string { return "" }
			s.ComfyUIDirs = func() []string { return nil }
			s.OpencodeBin = func() string { return "" }
			s.MdbookBin = func() string { return "" }
			s.VscodeBin = func() string { return "" }
			s.VscodiumBin = func() string { return "" }
			s.WettyBin = func() string { return "" }
		})
		cookie := loginCookie(t, env.base, testPassword)
		_, m := doReq(t, "GET", env.base+"/api/meta", cookie, nil)
		apps, _ := m["apps"].(map[string]any)
		for _, k := range []string{"comfyui", "opencode", "mdbook", "vscode", "vscodium", "wetty"} {
			a, _ := apps[k].(map[string]any)
			if a["installed"] != false || a["reason"] != "add.reason.notInstalled" {
				t.Fatalf("%s reason = %v", k, a)
			}
		}
	})

	t.Run("installed kinds omit the reason", func(t *testing.T) {
		env := newTestEnvFull(t, nil, func(s *api.Service) {
			comfyInstalled(s, "/path/to/comfy/ComfyUI")
			opencodeInstalled(s)
		})
		cookie := loginCookie(t, env.base, testPassword)
		_, m := doReq(t, "GET", env.base+"/api/meta", cookie, nil)
		apps, _ := m["apps"].(map[string]any)
		cf, _ := apps["comfyui"].(map[string]any)
		if _, ok := cf["reason"]; ok {
			t.Fatalf("installed comfyui should omit reason, got %v", cf)
		}
	})
}

func TestSettingsPageBackground(t *testing.T) {
	env := newTestEnv(t)
	cookie := loginCookie(t, env.base, testPassword)

	res, m := doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"pageBackground": "bg-01"})
	if res.StatusCode != http.StatusOK || m["pageBackground"] != "bg-01" {
		t.Fatalf("settings = %d %v", res.StatusCode, m)
	}
	res, m = doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	if m["pageBackground"] != "bg-01" {
		t.Fatalf("meta after settings = %v", m)
	}
	res, _ = doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"pageBackground": "no-such-bg"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid pageBackground = %d, want 400", res.StatusCode)
	}
}

func TestUploadLifecycle(t *testing.T) {
	env := newTestEnv(t)

	// Unauthenticated upload is rejected.
	res, _ := doReq(t, "POST", env.base+"/api/uploads", "", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("upload without auth = %d, want 401", res.StatusCode)
	}

	// 1x1 transparent PNG.
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
		0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
	cookie := loginCookie(t, env.base, testPassword)
	body, contentType := multipartBody(t, "file", "test.png", png)
	req, err := http.NewRequest("POST", env.base+"/api/uploads", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Cookie", cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var up map[string]any
	json.NewDecoder(resp.Body).Decode(&up)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || up["id"] == nil || up["url"] == nil {
		t.Fatalf("upload = %d %v", resp.StatusCode, up)
	}
	id := up["id"].(string)
	url := up["url"].(string)

	// Unsupported type rejected.
	body, contentType = multipartBody(t, "file", "test.txt", []byte("hello"))
	req, err = http.NewRequest("POST", env.base+"/api/uploads", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Cookie", cookie)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad type upload = %d, want 400", resp.StatusCode)
	}

	// The upload appears in meta and can be chosen as page background.
	res, m := doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	found := false
	for _, b := range m["backgrounds"].([]any) {
		bm := b.(map[string]any)
		if bm["id"] == id && bm["preset"] == false {
			found = true
		}
	}
	if !found {
		t.Fatalf("upload missing from meta: %v", m["backgrounds"])
	}
	res, _ = doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"pageBackground": id})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("pageBackground upload = %d", res.StatusCode)
	}

	// The image is served.
	resp, err = http.Get(env.base + url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", url, resp.StatusCode)
	}

	// Traversal blocked.
	resp, err = http.Get(env.base + "/uploads/../config.json")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("path traversal should not be served")
	}

	// Delete.
	res, _ = doReq(t, "DELETE", env.base+"/api/uploads/"+id, cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("delete upload = %d", res.StatusCode)
	}
	res, _ = doReq(t, "DELETE", env.base+"/api/uploads/"+id, cookie, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("delete again = %d, want 404", res.StatusCode)
	}
}

func TestSettingsSlogan(t *testing.T) {
	env := newTestEnv(t)
	cookie := loginCookie(t, env.base, testPassword)

	res, m := doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"slogan": "hello world"})
	if res.StatusCode != http.StatusOK || m["slogan"] != "hello world" {
		t.Fatalf("set slogan = %d %v", res.StatusCode, m)
	}
	res, m = doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	if m["slogan"] != "hello world" {
		t.Fatalf("meta slogan = %v", m["slogan"])
	}

	res, _ = doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"slogan": ""})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("clear slogan = %d", res.StatusCode)
	}
	res, m = doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	if m["slogan"] != "" {
		t.Fatalf("slogan should be empty after clearing: %v", m["slogan"])
	}

	res, _ = doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"slogan": strings.Repeat("x", 81)})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("overlong slogan = %d, want 400", res.StatusCode)
	}

	// A customized slogan no longer follows language switches.
	res, _ = doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"slogan": "custom", "language": "zh-TW"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set custom + switch lang = %d", res.StatusCode)
	}
	res, m = doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	if m["slogan"] != "custom" {
		t.Fatalf("custom slogan should not follow language: %v", m["slogan"])
	}
}

func TestAccountEndpoint(t *testing.T) {
	env := newTestEnv(t)
	cookie := loginCookie(t, env.base, testPassword)

	// Wrong current password → 401.
	res, m := doReq(t, "POST", env.base+"/api/account", cookie, map[string]any{
		"currentPassword": "wrong",
		"username":        "newuser",
	})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong current password = %d %v", res.StatusCode, m)
	}

	// Change username only.
	res, m = doReq(t, "POST", env.base+"/api/account", cookie, map[string]any{
		"currentPassword": testPassword,
		"username":        "newuser",
	})
	if res.StatusCode != http.StatusOK || m["username"] != "newuser" {
		t.Fatalf("change username = %d %v", res.StatusCode, m)
	}
	// Old session was rotated away; a fresh cookie was issued.
	res, _ = doReq(t, "GET", env.base+"/api/cards", cookie, nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old session should be invalid after account change, got %d", res.StatusCode)
	}
	// New username logs in with the same password.
	if got := loginCookieObjAs(t, env.base, "newuser", testPassword).Name; got == "" {
		t.Fatal("login with new username failed")
	}

	// Change password only.
	cookie = loginCookieAs(t, env.base, "newuser", testPassword)
	res, m = doReq(t, "POST", env.base+"/api/account", cookie, map[string]any{
		"currentPassword": testPassword,
		"newPassword":     "newpass-123",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("change password = %d %v", res.StatusCode, m)
	}
	if got := loginCookieObjAs(t, env.base, "newuser", "newpass-123").Name; got == "" {
		t.Fatal("login with new password failed")
	}
	res, _ = doReq(t, "POST", env.base+"/api/login", "", map[string]string{"username": "newuser", "password": testPassword})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password should fail, got %d", res.StatusCode)
	}

	// Validation: empty username means "keep the current one".
	cookie = loginCookieAs(t, env.base, "newuser", "newpass-123")
	res, _ = doReq(t, "POST", env.base+"/api/account", cookie, map[string]any{
		"currentPassword": "newpass-123",
		"username":        "",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("empty username should be ignored, got %d", res.StatusCode)
	}
	if got := loginCookieObjAs(t, env.base, "newuser", "newpass-123").Name; got == "" {
		t.Fatal("username should be unchanged after empty update")
	}
	cookie = loginCookieAs(t, env.base, "newuser", "newpass-123")
	res, _ = doReq(t, "POST", env.base+"/api/account", cookie, map[string]any{
		"currentPassword": "newpass-123",
		"newPassword":     "123",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("short password should be 400, got %d", res.StatusCode)
	}
}

func TestStartResolvesInstallAtStartTime(t *testing.T) {
	// The app is installed: the card can be created and started even
	// though no path is involved — the install directory is resolved by
	// the backend at start time.
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		comfyInstalled(s, "/path/to/comfy/ComfyUI")
		opencodeInstalled(s)
	})
	cookie := loginCookie(t, env.base, testPassword)

	_, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "comfyui"})
	id := cardID(t, m)
	res, m := doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start comfyui = %d %v", res.StatusCode, m)
	}
	if calls := env.backend.startCalls(); len(calls) != 1 || calls[0] != "comfyui|/path/to/comfy/ComfyUI|"+id {
		t.Fatalf("comfyui start calls = %v", calls)
	}
	// Stop so the fake backend is free for the next card.
	if res, _ := doReq(t, "POST", env.base+"/api/cards/"+id+"/stop", cookie, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("stop comfyui = %d", res.StatusCode)
	}

	// The opencode card's working directory is the user's home.
	home, _ := os.UserHomeDir()
	_, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "opencode"})
	id = cardID(t, m)
	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start opencode = %d %v", res.StatusCode, m)
	}
	if calls := env.backend.startCalls(); len(calls) != 2 || calls[1] != "opencode|"+home+"|"+id {
		t.Fatalf("opencode start calls = %v", calls)
	}
}

func TestMeta(t *testing.T) {
	env := newTestEnv(t)
	cookie := loginCookie(t, env.base, testPassword)

	res, m := doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("meta = %d", res.StatusCode)
	}
	if m["lang"] != "en" {
		t.Fatalf("default meta lang = %v, want en", m["lang"])
	}
	if m["pageBackground"] != "bg-05" {
		t.Fatalf("default pageBackground = %v, want bg-05", m["pageBackground"])
	}
	if m["slogan"] != "From nothing, to nothing." {
		t.Fatalf("default en slogan = %v", m["slogan"])
	}
	icons := m["icons"].([]any)
	if len(icons) == 0 {
		t.Fatalf("icons should not be empty: %v", m)
	}
	backgrounds := m["backgrounds"].([]any)
	if len(backgrounds) == 0 {
		t.Fatalf("backgrounds should not be empty: %v", m)
	}
	first, ok := backgrounds[0].(map[string]any)
	if !ok || first["id"] == nil || first["url"] == nil || first["preset"] != true {
		t.Fatalf("background structure unexpected: %v", backgrounds[0])
	}
	foundRocket := false
	for _, i := range icons {
		if i == "rocket" {
			foundRocket = true
		}
	}
	if !foundRocket {
		t.Fatalf("rocket icon missing from %v", icons)
	}
	if m["apps"] == nil {
		t.Fatalf("meta should include apps: %v", m)
	}
}

func TestInternalAddress(t *testing.T) {
	env := newTestEnvFull(t, func(cfg *store.Config) {
		cfg.ComfyUI.Hostname = "0.0.0.0"
	}, func(s *api.Service) {
		comfyInstalled(s, "/path/to/comfy/ComfyUI")
	})
	cookie := loginCookie(t, env.base, testPassword)

	// Helper: GET /api/cards with a custom Host so the visitor address
	// differs from the loopback test server.
	getCards := func(host string) map[string]any {
		req, err := http.NewRequest("GET", env.base+"/api/cards", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Cookie", cookie)
		req.Host = host
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		json.NewDecoder(res.Body).Decode(&m)
		return m
	}

	_, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "comfyui"})
	id := cardID(t, m)
	res, m := doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start = %d %v", res.StatusCode, m)
	}

	// No internal address configured: the URL mirrors the visitor's host.
	url := getCards("192.168.1.5:9090")["cards"].([]any)[0].(map[string]any)["url"]
	if url != "http://192.168.1.5:8000/" {
		t.Fatalf("url without internal address = %v", url)
	}

	// Configure an internal address: it replaces the visitor's host.
	res, m = doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"internalAddress": "10.0.0.5"})
	if res.StatusCode != http.StatusOK || m["internalAddress"] != "10.0.0.5" {
		t.Fatalf("set internal address = %d %v", res.StatusCode, m)
	}
	res, m = doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	if m["internalAddress"] != "10.0.0.5" {
		t.Fatalf("meta internalAddress = %v", m["internalAddress"])
	}
	url = getCards("192.168.1.5:9090")["cards"].([]any)[0].(map[string]any)["url"]
	if url != "http://10.0.0.5:8000/" {
		t.Fatalf("url with internal address = %v", url)
	}

	// An address that carries its own port is used verbatim.
	res, _ = doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"internalAddress": "10.0.0.5:8080"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set internal address with port = %d", res.StatusCode)
	}
	url = getCards("192.168.1.5:9090")["cards"].([]any)[0].(map[string]any)["url"]
	if url != "http://10.0.0.5:8080/" {
		t.Fatalf("url with internal address port = %v", url)
	}

	// Clearing the address falls back to the visitor's host.
	res, _ = doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"internalAddress": ""})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("clear internal address = %d", res.StatusCode)
	}
	url = getCards("192.168.1.5:9090")["cards"].([]any)[0].(map[string]any)["url"]
	if url != "http://192.168.1.5:8000/" {
		t.Fatalf("url after clearing internal address = %v", url)
	}
}

func TestOpencodeURL(t *testing.T) {
	env := newTestEnvFull(t, func(cfg *store.Config) {
		cfg.Opencode.Hostname = "0.0.0.0"
		cfg.Opencode.Password = "oc-secret"
	}, func(s *api.Service) {
		opencodeInstalled(s)
		s.GatewayPort = 5199
	})
	cookie := loginCookie(t, env.base, testPassword)

	_, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "opencode"})
	id := cardID(t, m)
	res, m := doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start opencode = %d %v", res.StatusCode, m)
	}

	getCards := func(host string) map[string]any {
		req, err := http.NewRequest("GET", env.base+"/api/cards", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Cookie", cookie)
		req.Host = host
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		json.NewDecoder(res.Body).Decode(&m)
		return m
	}

	// A non-loopback listen address mirrors the visitor's host; the
	// gateway port replaces the opencode listen port.
	card := getCards("192.168.1.5:9090")["cards"].([]any)[0].(map[string]any)
	if card["url"] != "http://192.168.1.5:5199/" {
		t.Fatalf("opencode url = %v", card["url"])
	}
	if card["apiUrl"] != "http://192.168.1.5:8000/" {
		t.Fatalf("opencode apiUrl = %v", card["apiUrl"])
	}

	// The internal network address replaces the visitor's host.
	res, _ = doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"internalAddress": "10.0.0.5"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set internal address = %d", res.StatusCode)
	}
	card = getCards("192.168.1.5:9090")["cards"].([]any)[0].(map[string]any)
	if card["url"] != "http://10.0.0.5:5199/" {
		t.Fatalf("opencode url with internal address = %v", card["url"])
	}
}

// TestOpencodeGatewayLifecycle checks that the reverse-proxy listener is
// started with the opencode card and torn down when it stops or is
// deleted, leaving no lingering socket.
func TestOpencodeGatewayLifecycle(t *testing.T) {
	gwWant := freePort(t)
	var svcRef *api.Service
	env := newTestEnvFull(t, func(cfg *store.Config) {
		cfg.Opencode.Password = "oc-secret"
		cfg.Opencode.GatewayPort = gwWant
	}, func(s *api.Service) {
		opencodeInstalled(s)
		svcRef = s
		s.GatewayHandler = func() http.Handler { return http.NotFoundHandler() }
	})
	cookie := loginCookie(t, env.base, testPassword)

	res, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "opencode"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create opencode = %d %v", res.StatusCode, m)
	}
	id := cardID(t, m)
	if svcRef.GatewayPort != 0 {
		t.Fatalf("gateway running before start: %d", svcRef.GatewayPort)
	}

	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start opencode = %d %v", res.StatusCode, m)
	}
	gwPort := svcRef.GatewayPort
	if gwPort == 0 {
		t.Fatalf("gateway not started with the card")
	}
	if gwPort != gwWant {
		t.Fatalf("gateway port = %d, want preferred %d", gwPort, gwWant)
	}
	if want := fmt.Sprintf("http://127.0.0.1:%d/", gwPort); m["url"] != want {
		t.Fatalf("card url = %v, want %v", m["url"], want)
	}

	res, _ = doReq(t, "POST", env.base+"/api/cards/"+id+"/stop", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stop opencode = %d", res.StatusCode)
	}
	if svcRef.GatewayPort != 0 {
		t.Fatalf("gateway still running after stop: %d", svcRef.GatewayPort)
	}

	// Start then delete must also stop the gateway, and a restart must reuse
	// the preferred port so the "Open" URL stays stable.
	res, _ = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK || svcRef.GatewayPort != gwWant {
		t.Fatalf("restart opencode = %d, gateway = %d, want %d", res.StatusCode, svcRef.GatewayPort, gwWant)
	}
	res, _ = doReq(t, "DELETE", env.base+"/api/cards/"+id, cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("delete opencode = %d", res.StatusCode)
	}
	if svcRef.GatewayPort != 0 {
		t.Fatalf("gateway still running after delete: %d", svcRef.GatewayPort)
	}
}

// TestSettingsGatewayPort checks validation and persistence of the fixed
// opencode gateway port exposed by /api/settings and /api/meta.
func TestSettingsGatewayPort(t *testing.T) {
	env := newTestEnvFull(t, func(cfg *store.Config) {
		cfg.Opencode.GatewayPort = store.DefaultOpencodeGatewayPort
	}, nil)
	cookie := loginCookie(t, env.base, testPassword)

	// Out-of-range values and the dashboard port are rejected.
	for _, bad := range []int{0, 70000, 9090} {
		res, _ := doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"gatewayPort": bad})
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("gatewayPort %d = %d, want 400", bad, res.StatusCode)
		}
	}

	want := freePort(t)
	res, m := doReq(t, "POST", env.base+"/api/settings", cookie, map[string]any{"gatewayPort": want})
	if res.StatusCode != http.StatusOK || m["gatewayPort"] != float64(want) {
		t.Fatalf("set gatewayPort = %d %v", res.StatusCode, m)
	}
	res, m = doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	if m["gatewayPort"] != float64(want) {
		t.Fatalf("meta gatewayPort = %v, want %d", m["gatewayPort"], want)
	}
}

func TestAutostartAPI(t *testing.T) {
	env := newTestEnv(t)
	cookie := loginCookie(t, env.base, testPassword)

	t.Run("unauthenticated rejected", func(t *testing.T) {
		res, _ := doReq(t, "GET", env.base+"/api/autostart", "", nil)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET without auth = %d, want 401", res.StatusCode)
		}
	})

	t.Run("GET status reports supported flag", func(t *testing.T) {
		res, m := doReq(t, "GET", env.base+"/api/autostart", cookie, nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		supported, ok := m["supported"].(bool)
		if !ok {
			t.Fatalf("supported flag missing or wrong type: %v", m)
		}
		// On macOS the feature is supported; on other platforms it isn't.
		if runtime.GOOS == "darwin" && !supported {
			t.Errorf("darwin should report supported=true, got %v", m)
		}
		if runtime.GOOS != "darwin" && supported {
			t.Errorf("non-darwin should report supported=false, got %v", m)
		}
		// A fresh dev machine / CI runner has no narthex plist installed.
		if installed, _ := m["installed"].(bool); installed {
			t.Errorf("expected not installed on a clean test env, got %v", m)
		}
	})

	t.Run("POST unknown action is rejected", func(t *testing.T) {
		res, m := doReq(t, "POST", env.base+"/api/autostart", cookie, map[string]string{"action": "bogus"})
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("bogus action = %d, want 400", res.StatusCode)
		}
		if m["error"] == nil {
			t.Fatalf("expected error message, got %v", m)
		}
	})

	t.Run("POST invalid JSON is rejected", func(t *testing.T) {
		req, _ := http.NewRequest("POST", env.base+"/api/autostart", bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid JSON = %d, want 400", res.StatusCode)
		}
	})
}

func TestSessionBootID(t *testing.T) {
	env := newTestEnv(t)
	res, m := doReq(t, "GET", env.base+"/api/session", "", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("session = %d, want 200", res.StatusCode)
	}
	if id, _ := m["bootId"].(string); id == "" {
		t.Fatalf("bootId missing from session response: %v", m)
	}
	// The login page shares the dashboard background, so the public
	// session response must carry it (with a resolved URL).
	if m["pageBackground"] != "bg-05" {
		t.Fatalf("pageBackground = %v", m["pageBackground"])
	}
	if m["pageBackgroundUrl"] != "/assets/backgrounds/bg-05.jpg" {
		t.Fatalf("pageBackgroundUrl = %v", m["pageBackgroundUrl"])
	}
}

func TestRestartAPI(t *testing.T) {
	t.Run("unauthenticated rejected", func(t *testing.T) {
		env := newTestEnv(t)
		res, _ := doReq(t, "POST", env.base+"/api/restart", "", nil)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("POST without auth = %d, want 401", res.StatusCode)
		}
	})

	t.Run("unavailable without a restart action", func(t *testing.T) {
		env := newTestEnv(t) // Restart is nil by default in tests.
		cookie := loginCookie(t, env.base, testPassword)
		res, m := doReq(t, "POST", env.base+"/api/restart", cookie, map[string]string{})
		if res.StatusCode != http.StatusNotImplemented {
			t.Fatalf("restart without action = %d, want 501", res.StatusCode)
		}
		if m["error"] == nil {
			t.Fatalf("expected error message, got %v", m)
		}
	})

	t.Run("scheduled restart is invoked", func(t *testing.T) {
		called := make(chan struct{}, 1)
		env := newTestEnvFull(t, nil, func(s *api.Service) {
			s.RestartDelay = time.Millisecond
			s.Restart = func() {
				select {
				case called <- struct{}{}:
				default:
				}
			}
		})
		cookie := loginCookie(t, env.base, testPassword)
		res, m := doReq(t, "POST", env.base+"/api/restart", cookie, map[string]string{})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("restart = %d, want 200", res.StatusCode)
		}
		if ok, _ := m["ok"].(bool); !ok {
			t.Fatalf("expected ok=true, got %v", m)
		}
		select {
		case <-called:
		case <-time.After(2 * time.Second):
			t.Fatal("restart action was not invoked")
		}
	})
}

func TestMdbookCardFlow(t *testing.T) {
	root := t.TempDir()
	booksDir := filepath.Join(root, "books")
	bookDir := filepath.Join(booksDir, "guide")
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		mdbookInstalled(s, booksDir, bookDir)
	})
	cookie := loginCookie(t, env.base, testPassword)

	// A mdbook card requires a recognized project directory.
	res, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "mdbook"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create mdbook without dir = %d, want 400 (%v)", res.StatusCode, m)
	}
	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]any{"kind": "mdbook", "dir": filepath.Join(root, "other")})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create mdbook with unknown dir = %d, want 400 (%v)", res.StatusCode, m)
	}

	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]any{"kind": "mdbook", "dir": bookDir})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create mdbook = %d %v", res.StatusCode, m)
	}
	if m["kind"] != "mdbook" || m["dir"] != bookDir || m["name"] != "MdBook" || m["icon"] != "book-open" {
		t.Fatalf("mdbook defaults = %v", m)
	}
	id := cardID(t, m)

	res, _ = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]any{"kind": "mdbook", "dir": bookDir})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate mdbook = %d, want 409", res.StatusCode)
	}

	// Start passes the stored project directory to the backend.
	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start mdbook = %d %v", res.StatusCode, m)
	}
	calls := env.backend.startCalls()
	if len(calls) != 1 || calls[0] != "mdbook|"+bookDir+"|"+id {
		t.Fatalf("start calls = %v", calls)
	}

	res, _ = doReq(t, "POST", env.base+"/api/cards/"+id+"/stop", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stop mdbook = %d", res.StatusCode)
	}
	res, _ = doReq(t, "DELETE", env.base+"/api/cards/"+id, cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("delete mdbook = %d", res.StatusCode)
	}
}

func TestMdbookHiddenWithoutDirs(t *testing.T) {
	// The CLI is installed but no mdbook.dirs configured: the kind is
	// hidden entirely (appInstalled reports false) and cards are rejected.
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		s.MdbookBin = func() string { return "/usr/bin/mdbook" }
		s.Config.Mdbook.Dirs = []string{}
	})
	cookie := loginCookie(t, env.base, testPassword)
	res, m := doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	apps, _ := m["apps"].(map[string]any)
	mb, _ := apps["mdbook"].(map[string]any)
	if mb == nil || mb["installed"] != false {
		t.Fatalf("meta apps.mdbook without dirs = %v", apps)
	}
	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "mdbook"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create mdbook without dirs = %d, want 400 (%v)", res.StatusCode, m)
	}
}

func TestMdbookProjectsEndpoints(t *testing.T) {
	root := t.TempDir()
	booksDir := filepath.Join(root, "books")
	bookDir := filepath.Join(booksDir, "guide")
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		mdbookInstalled(s, booksDir, bookDir)
	})
	cookie := loginCookie(t, env.base, testPassword)

	// GET lists the configured dirs and recognized projects.
	res, m := doReq(t, "GET", env.base+"/api/mdbook/projects", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET projects = %d", res.StatusCode)
	}
	dirs, _ := m["dirs"].([]any)
	if len(dirs) != 1 || dirs[0] != booksDir {
		t.Fatalf("projects dirs = %v", m["dirs"])
	}
	projects, _ := m["projects"].([]any)
	if len(projects) != 1 {
		t.Fatalf("projects = %v", m["projects"])
	}

	// Create a new book inside a configured dir.
	res, m = doReq(t, "POST", env.base+"/api/mdbook/projects", cookie, map[string]string{"dir": booksDir, "name": "newbook"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create project = %d %v", res.StatusCode, m)
	}
	if m["name"] != "newbook" || m["path"] != filepath.Join(booksDir, "newbook") {
		t.Fatalf("created project = %v", m)
	}
	// The new project is now recognized, so a card can point at it.
	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]any{"kind": "mdbook", "dir": filepath.Join(booksDir, "newbook")})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("card on created project = %d %v", res.StatusCode, m)
	}

	// Validation: parent outside the configured dirs.
	res, _ = doReq(t, "POST", env.base+"/api/mdbook/projects", cookie, map[string]string{"dir": root, "name": "x"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create outside dirs = %d, want 400", res.StatusCode)
	}
	// Validation: bad name.
	res, _ = doReq(t, "POST", env.base+"/api/mdbook/projects", cookie, map[string]string{"dir": booksDir, "name": "../evil"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create with bad name = %d, want 400", res.StatusCode)
	}
	// Validation: existing target.
	res, _ = doReq(t, "POST", env.base+"/api/mdbook/projects", cookie, map[string]string{"dir": booksDir, "name": "guide"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create existing = %d, want 400", res.StatusCode)
	}
}

func TestMdbookCreateWithoutDirs(t *testing.T) {
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		s.MdbookBin = func() string { return "/usr/bin/mdbook" }
		s.Config.Mdbook.Dirs = []string{}
	})
	cookie := loginCookie(t, env.base, testPassword)
	res, _ := doReq(t, "POST", env.base+"/api/mdbook/projects", cookie, map[string]string{"dir": "/books", "name": "x"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("create without dirs = %d, want 400", res.StatusCode)
	}
}

func TestVscodeCard(t *testing.T) {
	env := newTestEnvFull(t, func(cfg *store.Config) {
		cfg.Vscode.ConnectionToken = "vc-token"
	}, func(s *api.Service) {
		vscodeInstalled(s)
	})
	cookie := loginCookie(t, env.base, testPassword)

	res, m := doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	apps, _ := m["apps"].(map[string]any)
	vc, _ := apps["vscode"].(map[string]any)
	if vc == nil || vc["installed"] != true || vc["label"] != "VS Code" {
		t.Fatalf("meta apps.vscode = %v", apps)
	}

	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "vscode"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create vscode = %d %v", res.StatusCode, m)
	}
	if m["kind"] != "vscode" || m["name"] != "VS Code" || m["icon"] != "code" {
		t.Fatalf("vscode defaults = %v", m)
	}
	if m["password"] != "vc-token" {
		t.Fatalf("vscode card should expose the connection token, got %v", m)
	}
	if _, ok := m["username"]; ok {
		t.Fatalf("vscode card must not expose a username: %v", m)
	}
	id := cardID(t, m)

	// Start runs in the user's home like opencode.
	home, _ := os.UserHomeDir()
	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start vscode = %d %v", res.StatusCode, m)
	}
	calls := env.backend.startCalls()
	if len(calls) != 1 || calls[0] != "vscode|"+home+"|"+id {
		t.Fatalf("vscode start calls = %v", calls)
	}
	// The "Open" URL carries the connection token (the server answers 403
	// without it).
	if m["url"] != "http://127.0.0.1:8000/?tkn=vc-token" {
		t.Fatalf("vscode url = %v, want token in url", m["url"])
	}

	res, _ = doReq(t, "POST", env.base+"/api/cards/"+id+"/stop", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stop vscode = %d", res.StatusCode)
	}
}

func TestVscodiumCard(t *testing.T) {
	env := newTestEnvFull(t, func(cfg *store.Config) {
		cfg.Vscodium.ConnectionToken = "cd-token"
	}, func(s *api.Service) {
		vscodiumInstalled(s)
	})
	cookie := loginCookie(t, env.base, testPassword)

	res, m := doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	apps, _ := m["apps"].(map[string]any)
	cd, _ := apps["vscodium"].(map[string]any)
	if cd == nil || cd["installed"] != true || cd["label"] != "VSCodium" {
		t.Fatalf("meta apps.vscodium = %v", apps)
	}

	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "vscodium"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create vscodium = %d %v", res.StatusCode, m)
	}
	if m["kind"] != "vscodium" || m["name"] != "VSCodium" || m["icon"] != "code" {
		t.Fatalf("vscodium defaults = %v", m)
	}
	if m["password"] != "cd-token" {
		t.Fatalf("vscodium card should expose the connection token, got %v", m)
	}
	if _, ok := m["username"]; ok {
		t.Fatalf("vscodium card must not expose a username: %v", m)
	}
	id := cardID(t, m)

	home, _ := os.UserHomeDir()
	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start vscodium = %d %v", res.StatusCode, m)
	}
	calls := env.backend.startCalls()
	if len(calls) != 1 || calls[0] != "vscodium|"+home+"|"+id {
		t.Fatalf("vscodium start calls = %v", calls)
	}
	// The "Open" URL carries the connection token (the server answers 403
	// without it).
	if m["url"] != "http://127.0.0.1:8000/?tkn=cd-token" {
		t.Fatalf("vscodium url = %v, want token in url", m["url"])
	}

	res, _ = doReq(t, "POST", env.base+"/api/cards/"+id+"/stop", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stop vscodium = %d", res.StatusCode)
	}
}

func TestWettyCard(t *testing.T) {
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		wettyInstalled(s)
	})
	cookie := loginCookie(t, env.base, testPassword)

	res, m := doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	apps, _ := m["apps"].(map[string]any)
	wt, _ := apps["wetty"].(map[string]any)
	if wt == nil || wt["installed"] != true || wt["label"] != "WeTTY" {
		t.Fatalf("meta apps.wetty = %v", apps)
	}

	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]string{"kind": "wetty"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create wetty = %d %v", res.StatusCode, m)
	}
	if m["kind"] != "wetty" || m["name"] != "WeTTY" || m["icon"] != "terminal" {
		t.Fatalf("wetty defaults = %v", m)
	}
	// WeTTY has no HTTP-layer credentials to surface on the card.
	if _, ok := m["username"]; ok {
		t.Fatalf("wetty card must not expose a username: %v", m)
	}
	if _, ok := m["password"]; ok {
		t.Fatalf("wetty card must not expose a password: %v", m)
	}
	id := cardID(t, m)

	// Start runs in the user's home like opencode.
	home, _ := os.UserHomeDir()
	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start wetty = %d %v", res.StatusCode, m)
	}
	calls := env.backend.startCalls()
	if len(calls) != 1 || calls[0] != "wetty|"+home+"|"+id {
		t.Fatalf("wetty start calls = %v", calls)
	}
	if m["url"] != "http://127.0.0.1:8000/" {
		t.Fatalf("wetty url = %v", m["url"])
	}

	res, _ = doReq(t, "POST", env.base+"/api/cards/"+id+"/stop", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stop wetty = %d", res.StatusCode)
	}
}

func TestMetaAppsIncludesNewKinds(t *testing.T) {
	root := t.TempDir()
	booksDir := filepath.Join(root, "books")
	bookDir := filepath.Join(booksDir, "guide")
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		comfyInstalled(s, "/path/to/comfy/ComfyUI")
		opencodeInstalled(s)
		mdbookInstalled(s, booksDir, bookDir)
		vscodeInstalled(s)
		vscodiumInstalled(s)
		wettyInstalled(s)
	})
	cookie := loginCookie(t, env.base, testPassword)
	res, m := doReq(t, "GET", env.base+"/api/meta", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("meta = %d", res.StatusCode)
	}
	apps, _ := m["apps"].(map[string]any)
	for kind, want := range map[string]bool{
		"comfyui": true, "opencode": true, "mdbook": true, "vscode": true, "vscodium": true, "wetty": true,
	} {
		a, _ := apps[kind].(map[string]any)
		if a == nil || a["installed"] != want {
			t.Fatalf("meta apps.%s = %v", kind, apps)
		}
	}
}
