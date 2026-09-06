package procman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFakeCodeServe creates a fake `code`/`codium` CLI (named binName)
// that mimics `serve-web`: it serves HTTP on the port given via `--port`.
// When VSCODE_DUMP_FILE is set it also dumps the arguments and PATH so
// tests can assert the child env (connection token, license flag, no-open
// shadow).
func writeFakeCodeServe(t *testing.T, binName string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), binName)
	shim := `#!/bin/sh
if [ -n "$VSCODE_DUMP_FILE" ]; then
  printf 'ARGS|%s\nPATH|%s\n' "$*" "$PATH" > "$VSCODE_DUMP_FILE"
fi
if [ "$1" = "serve-web" ]; then
  port=4700
  prev=
  for a in "$@"; do
    if [ "$prev" = "--port" ]; then port=$a; fi
    prev=$a
  done
  exec python3 -m http.server "$port" --bind 127.0.0.1
fi
exit 0
`
	if err := os.WriteFile(bin, []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestDetectVSCode(t *testing.T) {
	t.Setenv("NARTHEX_VSCODE_BIN", "/opt/fake/bin/code")
	if got := DetectVSCode(); got != "/opt/fake/bin/code" {
		t.Fatalf("DetectVSCode with override = %q", got)
	}
	t.Setenv("NARTHEX_VSCODE_BIN", "")
	dir := t.TempDir()
	codeBin := filepath.Join(dir, "code")
	if err := os.WriteFile(codeBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if got := DetectVSCode(); got != codeBin {
		t.Fatalf("DetectVSCode from PATH (code) = %q, want %q", got, codeBin)
	}

	// A PATH with only codium must NOT satisfy the vscode kind.
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "codium"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir2)
	if got := DetectVSCode(); got != "" {
		t.Fatalf("DetectVSCode with only codium = %q, want empty", got)
	}
}

func TestDetectVSCodium(t *testing.T) {
	t.Setenv("NARTHEX_VSCODIUM_BIN", "/opt/fake/bin/codium")
	if got := DetectVSCodium(); got != "/opt/fake/bin/codium" {
		t.Fatalf("DetectVSCodium with override = %q", got)
	}
	t.Setenv("NARTHEX_VSCODIUM_BIN", "")
	dir := t.TempDir()
	codiumBin := filepath.Join(dir, "codium")
	if err := os.WriteFile(codiumBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if got := DetectVSCodium(); got != codiumBin {
		t.Fatalf("DetectVSCodium from PATH (codium) = %q, want %q", got, codiumBin)
	}

	// A PATH with only code must NOT satisfy the vscodium kind.
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "code"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir2)
	if got := DetectVSCodium(); got != "" {
		t.Fatalf("DetectVSCodium with only code = %q, want empty", got)
	}
}

func TestVSCodeFamilyLifecycle(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; needed for the code/codium shim")
	}
	cases := []struct {
		name      string
		kind      string
		binName   string
		envKey    string
		hostname  string
		portRange [2]int
		token     string
	}{
		{"vscode", "vscode", "code", "NARTHEX_VSCODE_BIN", "127.0.0.1", [2]int{4780, 4799}, "tok-vscode"},
		{"vscodium", "vscodium", "codium", "NARTHEX_VSCODIUM_BIN", "127.0.0.1", [2]int{4880, 4899}, "tok-vscodium"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := writeFakeCodeServe(t, tc.binName)
			t.Setenv(tc.envKey, bin)
			logDir := t.TempDir()
			dumpFile := filepath.Join(t.TempDir(), "dump.txt")
			t.Setenv("VSCODE_DUMP_FILE", dumpFile)
			m := &Manager{LogDir: logDir}
			if tc.kind == "vscode" {
				m.VscodeHostname = tc.hostname
				m.VscodePortRange = tc.portRange
				m.VscodeConnectionToken = tc.token
			} else {
				m.VscodiumHostname = tc.hostname
				m.VscodiumPortRange = tc.portRange
				m.VscodiumConnectionToken = tc.token
			}

			pid, port, err := m.Start(tc.kind, t.TempDir(), "vc1")
			if err != nil {
				t.Fatal(err)
			}
			if pid <= 0 || port < tc.portRange[0] || port > tc.portRange[1] {
				t.Fatalf("unexpected pid=%d port=%d", pid, port)
			}
			if _, err := os.Stat(filepath.Join(logDir, "vc1.log")); err != nil {
				t.Fatalf("log file missing: %v", err)
			}

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
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			if len(lines) != 2 || !strings.HasPrefix(lines[0], "ARGS|") {
				t.Fatalf("child dump = %q", raw)
			}
			joined := strings.Join(strings.Split(strings.TrimPrefix(lines[0], "ARGS|"), " "), " ")
			for _, want := range []string{"serve-web", "--host", tc.hostname, "--connection-token", tc.token, "--accept-server-license-terms", "--disable-telemetry"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("child args %q missing %q", lines[0], want)
				}
			}
			pathLine := strings.TrimPrefix(lines[1], "PATH|")
			noOpen := filepath.Join(logDir, "no-open")
			if !strings.HasPrefix(pathLine, noOpen+string(os.PathListSeparator)) {
				t.Fatalf("PATH should start with the no-open dir: %q", pathLine)
			}

			deadline := time.Now().Add(5 * time.Second)
			st := m.Status(tc.kind, pid, port)
			for (!st.Alive || !st.Healthy) && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
				st = m.Status(tc.kind, pid, port)
			}
			if !st.Alive || !st.Healthy {
				t.Fatalf("shim should become alive+healthy: %+v", st)
			}

			if err := m.Stop(pid); err != nil {
				t.Fatal(err)
			}
			deadline = time.Now().Add(3 * time.Second)
			for m.Status(tc.kind, pid, port).Alive && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
			}
			if m.Status(tc.kind, pid, port).Alive {
				t.Fatal("shim should be dead after stop")
			}
		})
	}
}

func TestVSCodeFamilyStartError(t *testing.T) {
	for _, tc := range []struct {
		kind   string
		envKey string
	}{
		{"vscode", "NARTHEX_VSCODE_BIN"},
		{"vscodium", "NARTHEX_VSCODIUM_BIN"},
	} {
		t.Setenv(tc.envKey, "")
		t.Setenv("PATH", t.TempDir())
		m := &Manager{
			LogDir: t.TempDir(),
		}
		if _, _, err := m.Start(tc.kind, t.TempDir(), "x"); err == nil {
			t.Fatalf("start %s without a binary should error", tc.kind)
		}
	}
}
