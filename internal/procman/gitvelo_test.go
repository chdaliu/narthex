package procman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFakeGitvelo creates a fake `gitvelo` CLI that mimics `gitvelo serve`:
// it records its arguments to $GV_ARGS_FILE and serves HTTP on the port
// given via --port.
func writeFakeGitvelo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "gitvelo")
	shim := `#!/bin/sh
if [ -n "$GV_ARGS_FILE" ]; then
  printf '%s\n' "$@" > "$GV_ARGS_FILE"
fi
port=5400
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

// readFakeArgs waits for the fake CLI's argument dump and returns its lines.
func readFakeArgs(t *testing.T, path string) []string {
	t.Helper()
	var raw []byte
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			raw = b
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(raw) == 0 {
		t.Fatal("argument dump missing")
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func TestDetectGitvelo(t *testing.T) {
	// The env override wins.
	t.Setenv("NARTHEX_GITVELO_BIN", "/opt/fake/bin/gitvelo")
	if got := DetectGitvelo(); got != "/opt/fake/bin/gitvelo" {
		t.Fatalf("DetectGitvelo with override = %q", got)
	}

	// Without the override, the executable on PATH is found.
	t.Setenv("NARTHEX_GITVELO_BIN", "")
	dir := t.TempDir()
	bin := filepath.Join(dir, "gitvelo")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if got := DetectGitvelo(); got != bin {
		t.Fatalf("DetectGitvelo from PATH = %q, want %q", got, bin)
	}
}

func TestGitveloLifecycle(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; needed for the gitvelo shim")
	}
	bin := writeFakeGitvelo(t)
	t.Setenv("NARTHEX_GITVELO_BIN", bin)
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("GV_ARGS_FILE", argsFile)
	logDir := t.TempDir()

	// Proxy mode: --no-auth on loopback, regardless of the configured host.
	m := &Manager{
		GitveloHostname:  "0.0.0.0",
		GitveloPortRange: [2]int{5480, 5499},
		LogDir:           logDir,
	}
	pid, port, err := m.Start("gitvelo", t.TempDir(), "gv1", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if pid <= 0 || port < 5480 || port > 5499 {
		t.Fatalf("unexpected pid=%d port=%d", pid, port)
	}
	if _, err := os.Stat(filepath.Join(logDir, "gv1.log")); err != nil {
		t.Fatalf("log file missing: %v", err)
	}
	args := readFakeArgs(t, argsFile)
	joined := strings.Join(args, " ")
	for _, want := range []string{"serve", "--no-auth", "--host 127.0.0.1", "--port"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("proxy-mode args = %q, want %q", joined, want)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	st := m.Status("gitvelo", pid, port)
	for (!st.Alive || !st.Healthy) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		st = m.Status("gitvelo", pid, port)
	}
	if !st.Alive || !st.Healthy {
		t.Fatalf("gitvelo shim should become alive+healthy: %+v", st)
	}
	if err := m.Stop(pid); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for m.Status("gitvelo", pid, port).Alive && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if m.Status("gitvelo", pid, port).Alive {
		t.Fatal("gitvelo shim should be dead after stop")
	}

	// Direct mode: no --no-auth and the configured listen address.
	os.Remove(argsFile)
	m = &Manager{
		GitveloHostname:  "0.0.0.0",
		GitveloPortRange: [2]int{5480, 5499},
		LogDir:           t.TempDir(),
	}
	pid, port, err = m.Start("gitvelo", t.TempDir(), "gv2", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop(pid)
	args = readFakeArgs(t, argsFile)
	joined = strings.Join(args, " ")
	if strings.Contains(joined, "--no-auth") {
		t.Fatalf("direct-mode args must not contain --no-auth: %q", joined)
	}
	if !strings.Contains(joined, "--host 0.0.0.0") {
		t.Fatalf("direct-mode args = %q, want --host 0.0.0.0", joined)
	}
}

func TestGitveloErrors(t *testing.T) {
	withoutBrewFallback(t)
	t.Setenv("NARTHEX_GITVELO_BIN", "")
	t.Setenv("PATH", t.TempDir())
	m := &Manager{
		GitveloHostname:  "127.0.0.1",
		GitveloPortRange: [2]int{5480, 5499},
		LogDir:           t.TempDir(),
	}
	if _, _, err := m.Start("gitvelo", t.TempDir(), "x", nil, true); err == nil {
		t.Fatal("start without a gitvelo binary should error")
	}
}
