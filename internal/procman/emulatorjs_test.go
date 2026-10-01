package procman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEmulatorCoreFor(t *testing.T) {
	cases := []struct {
		path   string
		core   string
		system string
		ok     bool
	}{
		{"game.nes", "fceumm", "NES", true},
		{"GAME.NES", "fceumm", "NES", true},
		{"mario.smc", "snes9x", "SNES", true},
		{"zelda.gba", "mgba", "Game Boy Advance", true},
		{"sonic.md", "genesis_plus_gx", "Mega Drive", true},
		{"disc.cue", "pcsx_rearmed", "PlayStation", true},
		{"mario.nes.zip", "fceumm", "NES", true},
		{"sonic.md.7z", "genesis_plus_gx", "Mega Drive", true},
		{"archive.zip", "", "", false},
		{"readme.txt", "", "", false},
		{"noext", "", "", false},
	}
	for _, c := range cases {
		core, system, ok := EmulatorCoreFor(c.path)
		if ok != c.ok || core != c.core || system != c.system {
			t.Fatalf("EmulatorCoreFor(%q) = (%q,%q,%v), want (%q,%q,%v)", c.path, core, system, ok, c.core, c.system, c.ok)
		}
	}
}

func TestDetectEmulatorGames(t *testing.T) {
	root := t.TempDir()
	// A nested recognized game, a hidden dir (skipped), a hidden file
	// (skipped) and an unknown extension (skipped).
	if err := os.MkdirAll(filepath.Join(root, "snes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(root, "mario.nes"):         "x",
		filepath.Join(root, "snes", "zelda.smc"): "x",
		filepath.Join(root, ".hidden", "x.gba"):  "x",
		filepath.Join(root, ".secret.nes"):       "x",
		filepath.Join(root, "notes.txt"):         "x",
		filepath.Join(root, "sonic.md.zip"):      "x",
	}
	for p, c := range files {
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	games := DetectEmulatorGames([]string{root})
	if len(games) != 3 {
		t.Fatalf("games = %+v, want 3", games)
	}
	byName := map[string]EmulatorGame{}
	for _, g := range games {
		byName[g.Name] = g
	}
	if g, ok := byName["mario"]; !ok || g.Core != "fceumm" {
		t.Fatalf("mario = %+v", byName["mario"])
	}
	if g, ok := byName["zelda"]; !ok || g.Core != "snes9x" {
		t.Fatalf("zelda = %+v", byName["zelda"])
	}
	// The archive name drops both extensions: "sonic.md.zip" → "sonic".
	if g, ok := byName["sonic"]; !ok || g.Core != "genesis_plus_gx" {
		t.Fatalf("sonic = %+v", byName["sonic"])
	}
}

// writeFakeEmulator creates a fake `emulator-serve`-style binary that serves
// HTTP on the port given via `--port`, so the emitted args can be checked.
func writeFakeEmulator(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "emulator")
	shim := `#!/bin/sh
if [ -n "$EMU_DUMP_FILE" ]; then
  printf 'ARGS|%s\n' "$*" > "$EMU_DUMP_FILE"
fi
port=5190
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

func TestEmulatorLifecycle(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; needed for the emulator shim")
	}
	bin := writeFakeEmulator(t)
	t.Setenv("NARTHEX_EMULATOR_BIN", bin)
	logDir := t.TempDir()
	dumpFile := filepath.Join(t.TempDir(), "dump.txt")
	t.Setenv("EMU_DUMP_FILE", dumpFile)

	romDir := t.TempDir()
	rom := filepath.Join(romDir, "mario.nes")
	if err := os.WriteFile(rom, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := &Manager{
		EmulatorHostname:   "127.0.0.1",
		EmulatorPortRange:  [2]int{5180, 5199},
		EmulatorCDNVersion: "stable",
		LogDir:             logDir,
	}
	pid, port, err := m.Start("emulatorjs", rom, "emu1")
	if err != nil {
		t.Fatal(err)
	}
	if pid <= 0 || port < 5180 || port > 5199 {
		t.Fatalf("unexpected pid=%d port=%d", pid, port)
	}
	if _, err := os.Stat(filepath.Join(logDir, "emu1.log")); err != nil {
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
	joined := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(raw)), "ARGS|"))
	for _, want := range []string{"emulator-serve", "--rom", rom, "--core", "fceumm", "--name", "mario", "--host", "127.0.0.1", "--cdn", "stable"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("child args %q missing %q", joined, want)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	st := m.Status("emulatorjs", pid, port)
	for (!st.Alive || !st.Healthy) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		st = m.Status("emulatorjs", pid, port)
	}
	if !st.Alive || !st.Healthy {
		t.Fatalf("emulator shim should become alive+healthy: %+v", st)
	}

	if err := m.Stop(pid); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for m.Status("emulatorjs", pid, port).Alive && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if m.Status("emulatorjs", pid, port).Alive {
		t.Fatal("emulator shim should be dead after stop")
	}
}

func TestEmulatorStartRejectsNonROM(t *testing.T) {
	t.Setenv("NARTHEX_EMULATOR_BIN", writeFakeEmulator(t))
	m := &Manager{
		EmulatorHostname:  "127.0.0.1",
		EmulatorPortRange: [2]int{5180, 5199},
		LogDir:            t.TempDir(),
	}
	txt := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(txt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Start("emulatorjs", txt, "x"); err == nil {
		t.Fatal("start with a non-ROM should error")
	}
}
