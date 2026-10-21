package procman

import (
	"os"
	"path/filepath"
	"testing"
)

// withoutBrewFallback hides the well-known Homebrew bin directories so
// that tests simulating "not installed" with a stub PATH stay immune to
// CLIs that are really installed there on the test machine.
func withoutBrewFallback(t *testing.T) {
	t.Helper()
	saved := brewBinDirs
	brewBinDirs = nil
	t.Cleanup(func() { brewBinDirs = saved })
}

// lookPath must fall back to the brewBinDirs seam when the process PATH
// (as launchd sets it for brew services / autostart) cannot resolve the
// CLI, and still return "" for names that exist nowhere.
func TestLookPathBrewFallback(t *testing.T) {
	dir := t.TempDir()
	name := "narthex-fake-cli"
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	saved := brewBinDirs
	brewBinDirs = []string{dir}
	defer func() { brewBinDirs = saved }()

	if got := lookPath(name); got != path {
		t.Fatalf("lookPath(%q) = %q, want %q", name, got, path)
	}
	if got := lookPath("narthex-missing-cli"); got != "" {
		t.Fatalf("lookPath(missing) = %q, want empty", got)
	}
}

// PATH resolution keeps precedence over the brew fallbacks.
func TestLookPathPrefersPATH(t *testing.T) {
	dir := t.TempDir()
	name := "narthex-fake-cli"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	fallback := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(fallback, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	saved := brewBinDirs
	brewBinDirs = []string{filepath.Dir(fallback)}
	defer func() { brewBinDirs = saved }()

	if got := lookPath(name); got != path {
		t.Fatalf("lookPath(%q) = %q, want %q", name, got, path)
	}
}
