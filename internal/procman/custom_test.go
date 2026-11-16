package procman

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"narthex/internal/store"
)

func TestResolveExecutable(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "myapp")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := ResolveExecutable("myapp", nil)
	if err != nil || got != bin {
		t.Fatalf("ResolveExecutable(myapp) = %q, %v; want %q", got, err, bin)
	}
	got, err = ResolveExecutable(bin, nil)
	if err != nil || got != bin {
		t.Fatalf("ResolveExecutable(abs) = %q, %v", got, err)
	}
	if _, err := ResolveExecutable(filepath.Join(dir, "missing"), nil); err == nil {
		t.Fatal("missing program should error")
	}
}

func TestCustomStartArgsAndAutoPort(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	m := &Manager{
		CustomHostname:  "127.0.0.1",
		CustomPortRange: [2]int{5650, 5670},
		LogDir:          t.TempDir(),
	}
	card := store.Card{
		ID:   "custom1",
		Kind: store.KindCustom,
		Custom: &store.CustomCard{
			ExecMode:  store.CustomExecArgs,
			Exec:      "python3",
			Command:   "-m http.server {port} --bind 127.0.0.1",
			PortMode:  store.CustomPortAuto,
			ProxyMode: store.CustomProxyOff,
		},
	}
	pid, port, err := m.Start(StartSpec{Card: card})
	if err != nil {
		t.Fatal(err)
	}
	if port < 5650 || port > 5670 {
		t.Fatalf("port = %d outside range", port)
	}
	waitHealthy(t, m, pid, port)
	if err := m.Stop(pid); err != nil {
		t.Fatal(err)
	}
}

func TestCustomStartShellMode(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	port := freeTestPort(t)
	m := &Manager{
		CustomHostname:  "127.0.0.1",
		CustomPortRange: [2]int{5671, 5690},
		LogDir:          t.TempDir(),
	}
	card := store.Card{
		ID:   "custom2",
		Kind: store.KindCustom,
		Custom: &store.CustomCard{
			ExecMode:  store.CustomExecShell,
			Command:   fmt.Sprintf("python3 -m http.server %d --bind 127.0.0.1", port),
			PortMode:  store.CustomPortFixed,
			Port:      port,
			ProxyMode: store.CustomProxyOff,
		},
	}
	pid, got, err := m.Start(StartSpec{Card: card})
	if err != nil {
		t.Fatal(err)
	}
	if got != port {
		t.Fatalf("port = %d, want %d", got, port)
	}
	waitHealthy(t, m, pid, got)
	if err := m.Stop(pid); err != nil {
		t.Fatal(err)
	}
}

func TestCustomFixedPortInUse(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	m := &Manager{CustomHostname: "127.0.0.1", LogDir: t.TempDir()}
	card := store.Card{
		ID:   "custom3",
		Kind: store.KindCustom,
		Custom: &store.CustomCard{
			ExecMode: store.CustomExecShell,
			Command:  "sleep 30",
			PortMode: store.CustomPortFixed,
			Port:     port,
		},
	}
	if _, _, err := m.Start(StartSpec{Card: card}); err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("expected port-in-use error, got %v", err)
	}
}

func TestRunInstall(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "install.log")
	marker := filepath.Join(dir, "installed")
	cmd := fmt.Sprintf("echo hello && touch %s", shellQuote(marker))
	if err := RunInstall(context.Background(), cmd, dir, logPath, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("install marker missing: %v", err)
	}
	b, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(b), "hello") {
		t.Fatalf("install log = %q, %v", b, err)
	}
}

func waitHealthy(t *testing.T, m *Manager, pid, port int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st := m.Status(store.KindCustom, pid, port)
		if st.Alive && st.Healthy {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("custom instance %d/%d did not become healthy", pid, port)
}

func freeTestPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
