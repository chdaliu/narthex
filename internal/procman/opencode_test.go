package procman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeFakeOpencode creates a fake `opencode` CLI that mimics `opencode
// serve`/`opencode web`: it answers `--version` (reporting v2, so the
// manager picks the `serve` subcommand) and serves HTTP on the port given
// via `--port`. When OC_ENV_FILE is set it also dumps the subcommand, the
// basic-auth credentials and PATH so tests can assert the child env.
func writeFakeOpencode(t *testing.T) string {
	t.Helper()
	return writeFakeOpencodeVersion(t, "opencode v2.0.25")
}

// writeFakeOpencodeVersion is writeFakeOpencode with a custom `--version`
// output.
func writeFakeOpencodeVersion(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "opencode")
	shim := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '%s\n' '` + version + `'
  exit 0
fi
if [ -n "$OC_ENV_FILE" ]; then
  printf '%s|%s|%s|%s\n' "$1" "$OPENCODE_SERVER_USERNAME" "$OPENCODE_SERVER_PASSWORD" "$PATH" > "$OC_ENV_FILE"
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

// writeVersionScript creates an executable shim that prints the given
// string for any invocation (used as a fake CLI for version probing).
func writeVersionScript(t *testing.T, output string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "opencode")
	script := "#!/bin/sh\n"
	if output != "" {
		script += "printf '%s\\n' '" + output + "'\n"
	}
	script += "exit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestOpencodeArgs(t *testing.T) {
	cases := []struct {
		name    string
		version string
		exit    int
		want    string
	}{
		// opencode v2 removed `web`; the web UI is served by `serve`.
		{"v2 uses serve", "opencode v2.0.25", 0, "serve"},
		// opencode v1 still serves the web UI from `web`.
		{"v1 keeps web", "1.18.21", 0, "web"},
		// Unparseable or failing version probes keep the historical v1
		// invocation instead of breaking v1 installs.
		{"unparseable falls back to web", "not-a-version", 0, "web"},
		{"failing probe falls back to web", "", 1, "web"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := writeVersionScript(t, tc.version, tc.exit)
			got := opencodeArgs(bin, 4321, "0.0.0.0")
			want := []string{tc.want, "--port", "4321", "--hostname", "0.0.0.0"}
			if strings.Join(got, " ") != strings.Join(want, " ") {
				t.Fatalf("opencodeArgs = %v, want %v", got, want)
			}
		})
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

	pid, port, err := m.Start("opencode", t.TempDir(), "oc1", nil)
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
	// the no-open dir first (so `opencode web` never opens a browser); on
	// a v2 CLI the manager probes the version and starts `serve`.
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
	if len(parts) != 4 {
		t.Fatalf("env dump = %q", raw)
	}
	if parts[0] != "serve" {
		t.Fatalf("v2 CLI should start `opencode serve`, got %q", parts[0])
	}
	if parts[1] != "opencode" || parts[2] != "secret-pass" {
		t.Fatalf("credentials = %q %q", parts[1], parts[2])
	}
	noOpen := filepath.Join(logDir, "no-open")
	if !strings.HasPrefix(parts[3], noOpen+string(os.PathListSeparator)) {
		t.Fatalf("PATH should start with the no-open dir: %q", parts[3])
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
	withoutBrewFallback(t)
	// No opencode on PATH and no override → start fails.
	t.Setenv("NARTHEX_OPENCODE_BIN", "")
	t.Setenv("PATH", t.TempDir())
	m := &Manager{
		OpencodeHostname:  "127.0.0.1",
		OpencodePortRange: [2]int{4380, 4399},
		OpencodePassword:  "secret",
		LogDir:            t.TempDir(),
	}
	if _, _, err := m.Start("opencode", t.TempDir(), "x", nil); err == nil {
		t.Fatal("start without an opencode binary should error")
	}
}
