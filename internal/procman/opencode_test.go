package procman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFakeOpencode creates a fake `opencode` CLI that mimics `opencode
// web`: it serves HTTP on the port given via `--port`, mirroring the
// web UI the real CLI hosts. When OC_ENV_FILE is set it also dumps the
// basic-auth credentials and PATH so tests can assert the child env.
func writeFakeOpencode(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "opencode")
	shim := `#!/bin/sh
if [ -n "$OC_ENV_FILE" ]; then
  printf '%s|%s|%s\n' "$OPENCODE_SERVER_USERNAME" "$OPENCODE_SERVER_PASSWORD" "$PATH" > "$OC_ENV_FILE"
fi
port=4300
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

func TestDetectOpencode(t *testing.T) {
	// The env override wins.
	t.Setenv("NARTHEX_OPENCODE_BIN", "/opt/fake/bin/opencode")
	if got := DetectOpencode(); got != "/opt/fake/bin/opencode" {
		t.Fatalf("DetectOpencode with override = %q", got)
	}

	// Without the override, the executable on PATH is found.
	t.Setenv("NARTHEX_OPENCODE_BIN", "")
	dir := t.TempDir()
	bin := filepath.Join(dir, "opencode")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if got := DetectOpencode(); got != bin {
		t.Fatalf("DetectOpencode from PATH = %q, want %q", got, bin)
	}
}

func TestOpencodeLifecycle(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; needed for the opencode shim")
	}
	bin := writeFakeOpencode(t)
	t.Setenv("NARTHEX_OPENCODE_BIN", bin)
	logDir := t.TempDir()
	envFile := filepath.Join(t.TempDir(), "env.txt")
	t.Setenv("OC_ENV_FILE", envFile)
	m := &Manager{
		OpencodeHostname:  "127.0.0.1",
		OpencodePortRange: [2]int{4380, 4399},
		OpencodePassword:  "secret-pass",
		LogDir:            logDir,
	}

	pid, port, err := m.Start("opencode", t.TempDir(), "oc1")
	if err != nil {
		t.Fatal(err)
	}
	if pid <= 0 || port < 4380 || port > 4399 {
		t.Fatalf("unexpected pid=%d port=%d", pid, port)
	}
	if _, err := os.Stat(filepath.Join(logDir, "oc1.log")); err != nil {
		t.Fatalf("log file missing: %v", err)
	}

	// The child receives the basic-auth credentials and a PATH that has
	// the no-open dir first (so `opencode web` never opens a browser).
	var raw []byte
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if b, err := os.ReadFile(envFile); err == nil {
			raw = b
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(raw) == 0 {
		t.Fatal("env dump missing")
	}
	parts := strings.Split(strings.TrimSpace(string(raw)), "|")
	if len(parts) != 3 {
		t.Fatalf("env dump = %q", raw)
	}
	if parts[0] != "opencode" || parts[1] != "secret-pass" {
		t.Fatalf("credentials = %q %q", parts[0], parts[1])
	}
	noOpen := filepath.Join(logDir, "no-open")
	if !strings.HasPrefix(parts[2], noOpen+string(os.PathListSeparator)) {
		t.Fatalf("PATH should start with the no-open dir: %q", parts[2])
	}
	for _, name := range []string{"open", "xdg-open"} {
		if _, err := os.Stat(filepath.Join(noOpen, name)); err != nil {
			t.Fatalf("no-open script %s missing: %v", name, err)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	st := m.Status("opencode", pid, port)
	for (!st.Alive || !st.Healthy) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		st = m.Status("opencode", pid, port)
	}
	if !st.Alive || !st.Healthy {
		t.Fatalf("opencode shim should become alive+healthy: %+v", st)
	}

	if err := m.Stop(pid); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for m.Status("opencode", pid, port).Alive && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if m.Status("opencode", pid, port).Alive {
		t.Fatal("opencode shim should be dead after stop")
	}
}

func TestOpencodeErrors(t *testing.T) {
	// No opencode on PATH and no override → start fails.
	t.Setenv("NARTHEX_OPENCODE_BIN", "")
	t.Setenv("PATH", t.TempDir())
	m := &Manager{
		OpencodeHostname:  "127.0.0.1",
		OpencodePortRange: [2]int{4380, 4399},
		OpencodePassword:  "secret",
		LogDir:            t.TempDir(),
	}
	if _, _, err := m.Start("opencode", t.TempDir(), "x"); err == nil {
		t.Fatal("start without an opencode binary should error")
	}
}
