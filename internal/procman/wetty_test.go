package procman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFakeWetty creates a fake `wetty` CLI that serves HTTP on the port
// given via `--port`. When WETTY_DUMP_FILE is set it also dumps the
// arguments so tests can assert the SSH target flags.
func writeFakeWetty(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "wetty")
	shim := `#!/bin/sh
if [ -n "$WETTY_DUMP_FILE" ]; then
  printf 'ARGS|%s\n' "$*" > "$WETTY_DUMP_FILE"
fi
port=4900
prev=
for a in "$@"; do
  if [ "$prev" = "--port" ]; then port=$a; fi
  prev=$a
done
exec python3 -m http.server "$port" --bind 127.0.0.1
`
	if err := os.WriteFile(bin, []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestDetectWetty(t *testing.T) {
	// The env override wins.
	t.Setenv("NARTHEX_WETTY_BIN", "/opt/fake/bin/wetty")
	if got := DetectWetty(); got != "/opt/fake/bin/wetty" {
		t.Fatalf("DetectWetty with override = %q", got)
	}

	// Without the override, the executable on PATH is found.
	t.Setenv("NARTHEX_WETTY_BIN", "")
	dir := t.TempDir()
	bin := filepath.Join(dir, "wetty")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if got := DetectWetty(); got != bin {
		t.Fatalf("DetectWetty from PATH = %q, want %q", got, bin)
	}
}

func TestWettyLifecycle(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; needed for the wetty shim")
	}
	bin := writeFakeWetty(t)
	t.Setenv("NARTHEX_WETTY_BIN", bin)
	logDir := t.TempDir()
	dumpFile := filepath.Join(t.TempDir(), "dump.txt")
	t.Setenv("WETTY_DUMP_FILE", dumpFile)
	m := &Manager{
		WettyHostname:  "127.0.0.1",
		WettyPortRange: [2]int{4980, 4999},
		WettySSHHost:   "localhost",
		WettySSHPort:   2222,
		WettySSHUser:   "narthex",
		LogDir:         logDir,
	}

	pid, port, err := m.Start("wetty", t.TempDir(), "wt1")
	if err != nil {
		t.Fatal(err)
	}
	if pid <= 0 || port < 4980 || port > 4999 {
		t.Fatalf("unexpected pid=%d port=%d", pid, port)
	}
	if _, err := os.Stat(filepath.Join(logDir, "wt1.log")); err != nil {
		t.Fatalf("log file missing: %v", err)
	}

	// The child receives the listen address and the configured SSH target.
	var raw []byte
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if b, err := os.ReadFile(dumpFile); err == nil {
			raw = b
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(raw) == 0 {
		t.Fatal("child dump missing")
	}
	joined := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(raw)), "ARGS|"))
	for _, want := range []string{"--port", "--host", "127.0.0.1", "--ssh-host", "localhost", "--ssh-port", "2222", "--ssh-user", "narthex"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("child args %q missing %q", joined, want)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	st := m.Status("wetty", pid, port)
	for (!st.Alive || !st.Healthy) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		st = m.Status("wetty", pid, port)
	}
	if !st.Alive || !st.Healthy {
		t.Fatalf("wetty shim should become alive+healthy: %+v", st)
	}

	if err := m.Stop(pid); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for m.Status("wetty", pid, port).Alive && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if m.Status("wetty", pid, port).Alive {
		t.Fatal("wetty shim should be dead after stop")
	}
}

func TestWettyStartError(t *testing.T) {
	// No wetty on PATH and no override → start fails.
	t.Setenv("NARTHEX_WETTY_BIN", "")
	t.Setenv("PATH", t.TempDir())
	m := &Manager{
		WettyHostname:  "127.0.0.1",
		WettyPortRange: [2]int{4980, 4999},
		LogDir:         t.TempDir(),
	}
	if _, _, err := m.Start("wetty", t.TempDir(), "x"); err == nil {
		t.Fatal("start without a wetty binary should error")
	}
}
