package emulatorserver_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"narthex/internal/emulatorserver"
)

func TestPlayerPageCDN(t *testing.T) {
	h := emulatorserver.Handler(emulatorserver.Options{
		ROM:        "/tmp/mario.nes",
		Core:       "fceumm",
		Name:       "Mario",
		CDNVersion: "stable",
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`EJS_core = "fceumm"`,
		`EJS_gameUrl = "rom"`,
		`EJS_pathtodata = "https://cdn.emulatorjs.org/stable/data/"`,
		`src="https://cdn.emulatorjs.org/stable/data/loader.js"`,
		"<title>Mario</title>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("page missing %q:\n%s", want, body)
		}
	}
}

func TestPlayerPageLocalData(t *testing.T) {
	h := emulatorserver.Handler(emulatorserver.Options{
		ROM:     "/tmp/mario.nes",
		Core:    "fceumm",
		Name:    "Mario",
		DataDir: "/data/emulatorjs",
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `EJS_pathtodata = "data/"`) {
		t.Fatalf("local data page should use relative data path:\n%s", body)
	}
	if strings.Contains(body, "cdn.emulatorjs.org") {
		t.Fatalf("local data page must not use the CDN:\n%s", body)
	}
}

func TestRomServed(t *testing.T) {
	dir := t.TempDir()
	rom := filepath.Join(dir, "mario.nes")
	if err := os.WriteFile(rom, []byte("ROMDATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := emulatorserver.Handler(emulatorserver.Options{ROM: rom, Core: "fceumm", Name: "Mario"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/rom", nil))
	b, _ := io.ReadAll(rec.Result().Body)
	if string(b) != "ROMDATA" {
		t.Fatalf("rom body = %q, want ROMDATA", b)
	}
}

func TestLocalDataServed(t *testing.T) {
	data := t.TempDir()
	if err := os.WriteFile(filepath.Join(data, "loader.js"), []byte("loader"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := emulatorserver.Handler(emulatorserver.Options{ROM: "/tmp/x.nes", Core: "fceumm", Name: "x", DataDir: data})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/data/loader.js", nil))
	b, _ := io.ReadAll(rec.Result().Body)
	if string(b) != "loader" {
		t.Fatalf("data body = %q, want loader", b)
	}
	// Without a local dir, /data/ is not mounted.
	h2 := emulatorserver.Handler(emulatorserver.Options{ROM: "/tmp/x.nes", Core: "fceumm", Name: "x"})
	rec2 := httptest.NewRecorder()
	h2.ServeHTTP(rec2, httptest.NewRequest("GET", "/data/loader.js", nil))
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("no data dir should 404 /data/, got %d", rec2.Code)
	}
}
