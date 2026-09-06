package procman

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// writeFakeApp creates a fake .app bundle whose main binary runs the
// given shell script. Returns the bundle directory.
func writeFakeApp(t *testing.T, name, binName, script string) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), name)
	binDir := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(binDir, binName)
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return app
}

// writeComfyUIShim writes a minimal ComfyUI install: a main.py that
// serves HTTP on the port given via --port, mirroring ComfyUI's CLI.
func writeComfyUIShim(t *testing.T, dir string) {
	t.Helper()
	mainPy := `#!/usr/bin/env python3
import http.server, sys

port = 8188
prev = None
for a in sys.argv[1:]:
    if prev == "--port":
        port = int(a)
    prev = a
http.server.ThreadingHTTPServer(("127.0.0.1", port), http.server.SimpleHTTPRequestHandler).serve_forever()
`
	if err := os.WriteFile(filepath.Join(dir, "main.py"), []byte(mainPy), 0o755); err != nil {
		t.Fatal(err)
	}
}

const sleepShim = `#!/bin/sh
sleep 300
`

func TestDetectComfyUIApp(t *testing.T) {
	// A fake bundle via the env override is detected (and wins over any
	// real install on this machine).
	app := writeFakeApp(t, "Comfy Desktop.app", "Comfy Desktop", sleepShim)
	t.Setenv("NARTHEX_COMFY_APP_DIR", app)
	if got := DetectComfyUIApp(); got != app {
		t.Fatalf("DetectComfyUIApp = %q, want %q", got, app)
	}

	// An override without the binary is ignored: the result is either a
	// real install or empty, never the override.
	override := t.TempDir()
	t.Setenv("NARTHEX_COMFY_APP_DIR", override)
	if got := DetectComfyUIApp(); got == override {
		t.Fatalf("override without binary should not be detected, got %q", got)
	}
}

func TestFreePort(t *testing.T) {
	m := &Manager{Hostname: "127.0.0.1", PortRange: [2]int{4150, 4159}}
	p, err := m.FreePort()
	if err != nil {
		t.Fatal(err)
	}
	if p < 4150 || p > 4159 {
		t.Fatalf("port %d outside range", p)
	}
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
	if err != nil {
		t.Fatalf("port %d not actually free: %v", p, err)
	}
	l.Close()

	bad := &Manager{Hostname: "127.0.0.1", PortRange: [2]int{0, 0}}
	if _, err := bad.FreePort(); err == nil {
		t.Fatal("invalid range should error")
	}
}

func TestHealthy(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	if !Healthy("127.0.0.1", port) {
		t.Fatal("listener should be healthy")
	}
	l.Close()
	if Healthy("127.0.0.1", port) {
		t.Fatal("closed port should be unhealthy")
	}
	if Healthy("127.0.0.1", 0) {
		t.Fatal("port 0 should be unhealthy")
	}
}

func TestMemoryKB(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	defer cmd.Process.Kill()
	kb, ok := MemoryKB(cmd.Process.Pid)
	if !ok || kb <= 0 {
		t.Fatalf("expected positive RSS, got %d ok=%v", kb, ok)
	}
	if _, ok := MemoryKB(0); ok {
		t.Fatal("pid 0 should not report memory")
	}
}

func TestStopNonExistent(t *testing.T) {
	if err := Stop(999999); err != nil {
		t.Fatalf("stopping a dead pid should not error: %v", err)
	}
	if Alive(999999) {
		t.Fatal("nonexistent pid should not be alive")
	}
}

func TestComfyUILifecycle(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; needed for the ComfyUI shim")
	}
	install := t.TempDir()
	writeComfyUIShim(t, install)
	logDir := t.TempDir()
	m := &Manager{
		Hostname:  "127.0.0.1",
		PortRange: [2]int{4190, 4199},
		LogDir:    logDir,
	}

	pid, port, err := m.Start("comfyui", install, "cf1")
	if err != nil {
		t.Fatal(err)
	}
	if pid <= 0 || port < 4190 || port > 4199 {
		t.Fatalf("unexpected pid=%d port=%d", pid, port)
	}
	deadline := time.Now().Add(5 * time.Second)
	st := m.Status("comfyui", pid, port)
	for (!st.Alive || !st.Healthy) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		st = m.Status("comfyui", pid, port)
	}
	if !st.Alive || !st.Healthy {
		t.Fatalf("ComfyUI shim should become alive+healthy: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(logDir, "cf1.log")); err != nil {
		t.Fatalf("log file missing: %v", err)
	}

	if err := m.Stop(pid); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for m.Status("comfyui", pid, port).Alive && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if m.Status("comfyui", pid, port).Alive {
		t.Fatal("ComfyUI shim should be dead after stop")
	}
}

func TestComfyUIErrors(t *testing.T) {
	m := &Manager{Hostname: "127.0.0.1", PortRange: [2]int{4190, 4199}, LogDir: t.TempDir()}

	// Directory without main.py.
	empty := t.TempDir()
	if _, _, err := m.Start("comfyui", empty, "x"); err == nil {
		t.Fatal("start in a dir without main.py should error")
	}

	// Unknown kind.
	if _, _, err := m.Start("wechat", t.TempDir(), "x"); err == nil {
		t.Fatal("unknown kind should error")
	}
}

func TestDetectComfyUIInstalls(t *testing.T) {
	desktop := t.TempDir()

	// Missing installations.json → nothing detected.
	if got := detectComfyUIInstalls(desktop); len(got) != 0 {
		t.Fatalf("missing installations.json should detect nothing, got %v", got)
	}

	// An installed install with a main.py is detected; one without
	// main.py and one still downloading are skipped; duplicates collapse.
	base := t.TempDir()
	good := filepath.Join(base, "inst1", "ComfyUI")
	noMain := filepath.Join(base, "inst2", "ComfyUI")
	pending := filepath.Join(base, "inst3", "ComfyUI")
	for _, d := range []string{good, noMain, pending} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(good, "main.py"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	installs := []map[string]string{
		{"installPath": filepath.Join(base, "inst1"), "status": "installed"},
		{"installPath": filepath.Join(base, "inst2"), "status": "installed"},
		{"installPath": filepath.Join(base, "inst3"), "status": "downloading"},
		{"installPath": filepath.Join(base, "inst1"), "status": "installed"},
	}
	b, err := json.Marshal(installs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(desktop, "installations.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	got := detectComfyUIInstalls(desktop)
	if len(got) != 1 || got[0] != good {
		t.Fatalf("detected dirs = %v, want [%s]", got, good)
	}
}
