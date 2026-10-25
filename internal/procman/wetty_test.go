package procman

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"narthex/internal/store"
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
		WettySSHMode:   store.WettySSHSystem,
		WettySSHHost:   "localhost",
		WettySSHPort:   2222,
		WettySSHUser:   "narthex",
		LogDir:         logDir,
	}

	pid, port, err := m.Start("wetty", t.TempDir(), "wt1", []string{"192.168.1.20", "http://10.0.0.5:8080", "127.0.0.1"}, false)
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
	for _, want := range []string{"--port", "--host", "127.0.0.1", "--ssh-host", "localhost", "--ssh-port", "2222", "--ssh-user", "narthex",
		// WeTTY >= 4 trusted origins: loopback variants plus the browser
		// hosts (schemes and dashboard ports stripped, deduplicated).
		"--allowed-origin", "http://localhost:" + strconv.Itoa(port),
		"http://127.0.0.1:" + strconv.Itoa(port),
		"http://192.168.1.20:" + strconv.Itoa(port),
		"http://10.0.0.5:" + strconv.Itoa(port),
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("child args %q missing %q", joined, want)
		}
	}
	if strings.Count(joined, "--allowed-origin") != 4 {
		t.Fatalf("child args %q should carry exactly 4 origins", joined)
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
	withoutBrewFallback(t)
	// No wetty on PATH and no override → start fails.
	t.Setenv("NARTHEX_WETTY_BIN", "")
	t.Setenv("PATH", t.TempDir())
	m := &Manager{
		WettyHostname:  "127.0.0.1",
		WettyPortRange: [2]int{4980, 4999},
		LogDir:         t.TempDir(),
	}
	if _, _, err := m.Start("wetty", t.TempDir(), "x", nil, false); err == nil {
		t.Fatal("start without a wetty binary should error")
	}
}

func TestWettyOrigins(t *testing.T) {
	got := wettyOrigins(4900, []string{
		"localhost",
		"http://10.0.0.5:8080/",
		"QNAP.local",
		"[::1]",
		"127.0.0.1:5000",
		"",
	})
	want := []string{
		"http://localhost:4900",
		"http://127.0.0.1:4900",
		"http://10.0.0.5:4900",
		"http://QNAP.local:4900",
		"http://[::1]:4900",
	}
	if len(got) != len(want) {
		t.Fatalf("origins = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("origins = %v, want %v", got, want)
		}
	}
}

// writeFakeSSHD creates a fake sshd that serves HTTP on the port given via
// `-p`. It deliberately does not exec, so the shell keeps the generated
// config path on its command line (the manager verifies it before killing a
// pid from the state file).
func writeFakeSSHD(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sshd")
	shim := `#!/bin/sh
if [ -n "$NARTHEX_SSHD_ARGS_FILE" ]; then
  printf '%s\n' "$*" > "$NARTHEX_SSHD_ARGS_FILE"
fi
port=5300
prev=
for a in "$@"; do
  if [ "$prev" = "-p" ]; then port=$a; fi
  prev=$a
done
python3 -m http.server "$port" --bind 127.0.0.1
`
	if err := os.WriteFile(bin, []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// waitForFile polls for a non-empty file written by a spawned shim.
func waitForFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return b
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
	return nil
}

func testUsername(t *testing.T) string {
	t.Helper()
	name, err := currentUsername()
	if err != nil {
		t.Skip(err)
	}
	return name
}

// managedWettyManager wires a fake wetty plus a fake sshd and returns the
// manager together with the args dump paths.
func managedWettyManager(t *testing.T) (*Manager, string, string) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; needed for the wetty/sshd shims")
	}
	t.Setenv("NARTHEX_WETTY_BIN", writeFakeWetty(t))
	t.Setenv("NARTHEX_SSHD_BIN", writeFakeSSHD(t))
	wettyDump := filepath.Join(t.TempDir(), "wetty-args.txt")
	sshdDump := filepath.Join(t.TempDir(), "sshd-args.txt")
	t.Setenv("WETTY_DUMP_FILE", wettyDump)
	t.Setenv("NARTHEX_SSHD_ARGS_FILE", sshdDump)
	m := &Manager{
		WettyHostname:     "127.0.0.1",
		WettyPortRange:    [2]int{4980, 4999},
		WettySSHMode:      store.WettySSHManaged,
		WettySSHHost:      "localhost",
		WettySSHPortRange: [2]int{5390, 5399},
		WettySSHDDir:      t.TempDir(),
		LogDir:            t.TempDir(),
	}
	return m, wettyDump, sshdDump
}

func readManagedState(t *testing.T, m *Manager) managedSSHDState {
	t.Helper()
	b, err := os.ReadFile(m.managedSSHDStatePath())
	if err != nil {
		t.Fatalf("read managed sshd state: %v", err)
	}
	var st managedSSHDState
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatalf("parse managed sshd state: %v", err)
	}
	return st
}

func waitDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for Alive(pid) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if Alive(pid) {
		t.Fatalf("pid %d should be dead", pid)
	}
}

// The default managed mode starts a private sshd on loopback, points WeTTY
// at it, and stops it together with the card.
func TestWettyManagedSSHD(t *testing.T) {
	m, wettyDump, sshdDump := managedWettyManager(t)
	pid, port, err := m.Start("wetty", t.TempDir(), "wtm", []string{"192.168.1.20"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if pid <= 0 || port < 4980 || port > 4999 {
		t.Fatalf("unexpected pid=%d port=%d", pid, port)
	}
	st := readManagedState(t, m)
	if st.SSHPID <= 0 || st.SSHPort < 5390 || st.SSHPort > 5399 {
		t.Fatalf("unexpected managed sshd state: %+v", st)
	}
	if st.WettyPID != pid {
		t.Fatalf("state wetty pid = %d, want %d", st.WettyPID, pid)
	}

	info, err := os.Stat(filepath.Join(m.WettySSHDDir, "host_ed25519"))
	if err != nil {
		t.Fatalf("host key missing: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("host key permissions = %o, want 600", perm)
	}

	sshdArgs := string(waitForFile(t, sshdDump))
	if !strings.Contains(sshdArgs, "-f "+m.managedSSHDConfigPath()) {
		t.Fatalf("sshd args %q missing the generated config", sshdArgs)
	}
	if !strings.Contains(sshdArgs, "-p "+strconv.Itoa(st.SSHPort)) {
		t.Fatalf("sshd args %q missing -p %d", sshdArgs, st.SSHPort)
	}

	wettyArgs := string(waitForFile(t, wettyDump))
	for _, want := range []string{
		"--ssh-host 127.0.0.1",
		"--ssh-port " + strconv.Itoa(st.SSHPort),
		"--ssh-user " + testUsername(t),
	} {
		if !strings.Contains(wettyArgs, want) {
			t.Fatalf("wetty args %q missing %q", wettyArgs, want)
		}
	}
	if !Healthy("127.0.0.1", st.SSHPort) {
		t.Fatalf("managed sshd port %d should accept connections", st.SSHPort)
	}

	if err := m.Stop(pid); err != nil {
		t.Fatal(err)
	}
	waitDead(t, st.SSHPID)
	if Healthy("127.0.0.1", st.SSHPort) {
		t.Fatalf("managed sshd port %d should be closed after stop", st.SSHPort)
	}
	if _, err := os.Stat(m.managedSSHDStatePath()); !os.IsNotExist(err) {
		t.Fatalf("managed sshd state should be removed, err=%v", err)
	}
}

// A wetty crash (not an explicit stop) must take the private sshd down too.
func TestWettyManagedSSHDDiesWithWetty(t *testing.T) {
	m, _, _ := managedWettyManager(t)
	pid, _, err := m.Start("wetty", t.TempDir(), "wtc", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	st := readManagedState(t, m)
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitDead(t, st.SSHPID)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(m.managedSSHDStatePath()); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("managed sshd state should be removed after the wetty crash")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// system mode keeps the old behavior: no private sshd, the configured SSH
// target is passed to WeTTY.
func TestWettySystemModeSkipsManagedSSHD(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; needed for the wetty shim")
	}
	t.Setenv("NARTHEX_WETTY_BIN", writeFakeWetty(t))
	marker := filepath.Join(t.TempDir(), "sshd-ran")
	fakeSSHD := filepath.Join(t.TempDir(), "sshd")
	if err := os.WriteFile(fakeSSHD, []byte("#!/bin/sh\ntouch \"$NARTHEX_SSHD_MARKER\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NARTHEX_SSHD_BIN", fakeSSHD)
	t.Setenv("NARTHEX_SSHD_MARKER", marker)
	wettyDump := filepath.Join(t.TempDir(), "wetty-args.txt")
	t.Setenv("WETTY_DUMP_FILE", wettyDump)
	m := &Manager{
		WettyHostname:  "127.0.0.1",
		WettyPortRange: [2]int{4980, 4999},
		WettySSHMode:   store.WettySSHSystem,
		WettySSHHost:   "ssh.example",
		WettySSHPort:   2222,
		WettySSHUser:   "bob",
		WettySSHDDir:   t.TempDir(),
		LogDir:         t.TempDir(),
	}
	pid, _, err := m.Start("wetty", t.TempDir(), "wts", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop(pid)

	wettyArgs := string(waitForFile(t, wettyDump))
	for _, want := range []string{"--ssh-host ssh.example", "--ssh-port 2222", "--ssh-user bob"} {
		if !strings.Contains(wettyArgs, want) {
			t.Fatalf("wetty args %q missing %q", wettyArgs, want)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("managed sshd must not run in system mode, err=%v", err)
	}
	if _, err := os.Stat(m.managedSSHDStatePath()); !os.IsNotExist(err) {
		t.Fatalf("system mode should not write managed sshd state, err=%v", err)
	}
}

func TestManagedSSHDConfig(t *testing.T) {
	cfg := managedSSHDConfig("alice", 5300, "/tmp/wetty-sshd")
	for _, want := range []string{
		"Port 5300",
		"ListenAddress 127.0.0.1",
		"AllowUsers alice",
		"PasswordAuthentication yes",
		"PubkeyAuthentication no",
		"PermitRootLogin no",
		"UsePAM yes",
		`HostKey "/tmp/wetty-sshd/host_ed25519"`,
		`PidFile "/tmp/wetty-sshd/sshd.pid"`,
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("managed sshd config missing %q:\n%s", want, cfg)
		}
	}
}

func TestWettyLocalTarget(t *testing.T) {
	cases := map[string]bool{
		"":                       true,
		"localhost":              true,
		"LOCALHOST":              true,
		"127.0.0.1":              true,
		"127.0.0.1:2222":         true,
		"http://localhost:9090":  true,
		"::1":                    true,
		"[::1]:22":               true,
		"192.168.1.20":           false,
		"10.0.0.5:22":            false,
		"ssh.example":            false,
		"http://ssh.example:220": false,
	}
	for host, want := range cases {
		if got := wettyLocalTarget(host); got != want {
			t.Errorf("wettyLocalTarget(%q) = %v, want %v", host, got, want)
		}
	}
}

// The managed sshd only accepts the narthex account; another sshUser must
// fail fast instead of producing an unreachable login.
func TestWettyManagedRejectsOtherUser(t *testing.T) {
	name := testUsername(t)
	t.Setenv("NARTHEX_WETTY_BIN", writeFakeWetty(t))
	m := &Manager{
		WettyHostname:     "127.0.0.1",
		WettyPortRange:    [2]int{4980, 4999},
		WettySSHMode:      store.WettySSHManaged,
		WettySSHHost:      "localhost",
		WettySSHPortRange: [2]int{5390, 5399},
		WettySSHUser:      name + "-other",
		WettySSHDDir:      t.TempDir(),
		LogDir:            t.TempDir(),
	}
	if _, _, err := m.Start("wetty", t.TempDir(), "wtu", nil, false); err == nil || !strings.Contains(err.Error(), "cannot log in") {
		t.Fatalf("start with another sshUser should fail, got %v", err)
	}
}

// The generated config must be accepted by the real sshd: it listens on the
// picked loopback port and announces itself, then disappears with the card.
// macOS-only: other platforms may refuse a non-root sshd, and the release
// target is macOS.
func TestWettyManagedRealSSHD(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("managed sshd integration is verified on macOS")
	}
	const realSSHD = "/usr/sbin/sshd"
	if _, err := os.Stat(realSSHD); err != nil {
		t.Skip("sshd not available")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; needed for the wetty shim")
	}
	t.Setenv("NARTHEX_WETTY_BIN", writeFakeWetty(t))
	t.Setenv("NARTHEX_SSHD_BIN", realSSHD)
	m := &Manager{
		WettyHostname:     "127.0.0.1",
		WettyPortRange:    [2]int{4980, 4999},
		WettySSHMode:      store.WettySSHManaged,
		WettySSHHost:      "localhost",
		WettySSHPortRange: [2]int{5390, 5399},
		WettySSHDDir:      t.TempDir(),
		LogDir:            t.TempDir(),
	}
	pid, _, err := m.Start("wetty", t.TempDir(), "wtr", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	st := readManagedState(t, m)

	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(st.SSHPort)), 2*time.Second)
	if err != nil {
		t.Fatalf("managed sshd should accept connections: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, _ := conn.Read(buf)
	conn.Close()
	if !strings.HasPrefix(string(buf[:n]), "SSH-2.0-") {
		t.Fatalf("managed sshd banner = %q", buf[:n])
	}

	if err := m.Stop(pid); err != nil {
		t.Fatal(err)
	}
	waitDead(t, st.SSHPID)
}
