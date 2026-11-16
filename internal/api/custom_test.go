package api_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"narthex/internal/api"
	"narthex/internal/store"
)

func TestCustomCardFlow(t *testing.T) {
	env := newTestEnv(t)
	cookie := loginCookie(t, env.base, testPassword)

	custom := map[string]any{
		"execMode":  "args",
		"exec":      "sh",
		"command":   "-c 'sleep 30'",
		"portMode":  "fixed",
		"port":      8123,
		"proxyMode": "path",
	}
	res, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]any{
		"kind":   "custom",
		"name":   "My App",
		"custom": custom,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create custom = %d %v", res.StatusCode, m)
	}
	id := cardID(t, m)

	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start custom = %d %v", res.StatusCode, m)
	}
	if m["url"] != "/custom/"+id+"/" {
		t.Fatalf("custom path url = %v", m["url"])
	}

	// Runtime fields are baked into the process: changes require a stop.
	custom["command"] = "-c 'sleep 31'"
	res, _ = doReq(t, "PATCH", env.base+"/api/cards/"+id, cookie, map[string]any{"custom": custom})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("patch running custom = %d, want 409", res.StatusCode)
	}

	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/stop", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stop custom = %d %v", res.StatusCode, m)
	}
	custom["proxyMode"] = "off"
	res, m = doReq(t, "PATCH", env.base+"/api/cards/"+id, cookie, map[string]any{"custom": custom})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("patch stopped custom = %d %v", res.StatusCode, m)
	}
	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK || m["url"] != "http://127.0.0.1:8000/" {
		t.Fatalf("custom direct url = %d %v", res.StatusCode, m["url"])
	}

	// Custom cards may be repeated.
	res, _ = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]any{
		"kind": "custom",
		"custom": map[string]any{
			"execMode": "shell",
			"command":  "sleep 30",
			"portMode": "none",
		},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("second custom card = %d", res.StatusCode)
	}
}

func TestBuiltinPresetIsLocked(t *testing.T) {
	env := newTestEnvFull(t, nil, func(s *api.Service) {
		opencodeInstalled(s)
	})
	cookie := loginCookie(t, env.base, testPassword)

	res, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]any{
		"kind":   "opencode",
		"custom": map[string]any{"execMode": "shell", "command": "should-not-apply"},
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("built-in create with custom = %d, want 400", res.StatusCode)
	}

	res, m = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]any{"kind": "opencode"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create opencode = %d %v", res.StatusCode, m)
	}
	id := cardID(t, m)
	if m["preset"] != "opencode" {
		t.Fatalf("preset = %v", m["preset"])
	}
	launch, _ := m["launch"].(map[string]any)
	if launch == nil || launch["exec"] != "opencode" {
		t.Fatalf("launch = %#v", m["launch"])
	}
	caps, _ := m["capabilities"].(map[string]any)
	if caps == nil || caps["editLaunch"] != false || caps["editProxy"] != true || caps["install"] != false {
		t.Fatalf("capabilities = %#v", m["capabilities"])
	}

	res, _ = doReq(t, "PATCH", env.base+"/api/cards/"+id, cookie, map[string]any{
		"custom": map[string]any{"execMode": "shell", "command": "should-not-apply"},
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("built-in patch launch = %d, want 400", res.StatusCode)
	}
	res, _ = doReq(t, "PATCH", env.base+"/api/cards/"+id, cookie, map[string]any{"name": "Renamed"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("built-in patch name = %d", res.StatusCode)
	}
	res, _ = doReq(t, "POST", env.base+"/api/cards/"+id+"/install", cookie, map[string]any{})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("built-in install = %d, want 400", res.StatusCode)
	}
}

func TestCrossOriginMutationRejected(t *testing.T) {
	env := newTestEnv(t)
	cookie := loginCookie(t, env.base, testPassword)
	body := `{"kind":"custom","custom":{"execMode":"shell","command":"sleep 30","portMode":"none"}}`
	req, _ := http.NewRequest("POST", env.base+"/api/cards", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Origin", "http://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin mutation = %d, want 403", res.StatusCode)
	}

	req, _ = http.NewRequest("POST", env.base+"/api/cards", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Origin", env.base)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("same-origin mutation = %d, want 200", res.StatusCode)
	}
}

func TestCustomValidation(t *testing.T) {
	env := newTestEnv(t)
	cookie := loginCookie(t, env.base, testPassword)
	res, _ := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]any{
		"kind":   "custom",
		"custom": map[string]any{"execMode": "args", "portMode": "auto"},
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("custom without exec = %d, want 400", res.StatusCode)
	}
	res, _ = doReq(t, "POST", env.base+"/api/cards", cookie, map[string]any{
		"kind": "custom",
		"custom": map[string]any{
			"execMode":       "shell",
			"command":        "sleep 30",
			"portMode":       "none",
			"proxyMode":      "path",
			"installEnabled": true,
			"installCommand": "",
			"gatewayPort":    0,
			"installAuto":    false,
		},
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("proxy without port / empty install = %d, want 400", res.StatusCode)
	}
}

func TestCustomInstall(t *testing.T) {
	env := newTestEnv(t)
	cookie := loginCookie(t, env.base, testPassword)
	res, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]any{
		"kind": "custom",
		"custom": map[string]any{
			"execMode":       "shell",
			"command":        "sleep 30",
			"portMode":       "none",
			"installEnabled": true,
			"installCommand": "echo installed",
		},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create install card = %d %v", res.StatusCode, m)
	}
	id := cardID(t, m)
	res, _ = doReq(t, "POST", env.base+"/api/cards/"+id+"/install", cookie, map[string]any{})
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("install = %d", res.StatusCode)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res, st := doReq(t, "GET", env.base+"/api/cards/"+id+"/install", cookie, nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("install status = %d", res.StatusCode)
		}
		if running, _ := st["running"].(bool); !running {
			if out, _ := st["output"].(string); !strings.Contains(out, "installed") {
				t.Fatalf("install output = %q", out)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("install did not finish")
}

func TestCustomAutoInstallThenStart(t *testing.T) {
	binDir := t.TempDir()
	env := newTestEnvFull(t, func(cfg *store.Config) {
		cfg.Custom.PathDirs = []string{binDir}
	}, func(s *api.Service) {})
	cookie := loginCookie(t, env.base, testPassword)
	installCmd := fmt.Sprintf("cp /bin/sleep %s && chmod +x %s",
		filepath.Join(binDir, "narthex-custom-autotest"),
		filepath.Join(binDir, "narthex-custom-autotest"))
	res, m := doReq(t, "POST", env.base+"/api/cards", cookie, map[string]any{
		"kind": "custom",
		"custom": map[string]any{
			"execMode":       "args",
			"exec":           "narthex-custom-autotest",
			"command":        "30",
			"portMode":       "none",
			"installEnabled": true,
			"installAuto":    true,
			"installCommand": installCmd,
		},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create auto-install card = %d %v", res.StatusCode, m)
	}
	id := cardID(t, m)
	res, m = doReq(t, "POST", env.base+"/api/cards/"+id+"/start", cookie, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start auto-install card = %d %v", res.StatusCode, m)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, list := doReq(t, "GET", env.base+"/api/cards", cookie, nil)
		cards, _ := list["cards"].([]any)
		for _, raw := range cards {
			c, _ := raw.(map[string]any)
			if c["id"] != id {
				continue
			}
			installing, _ := c["installing"].(bool)
			running, _ := c["running"].(bool)
			if !installing && running {
				if _, err := os.Stat(filepath.Join(binDir, "narthex-custom-autotest")); err != nil {
					t.Fatalf("install did not create the program: %v", err)
				}
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("auto install did not start the card")
}
