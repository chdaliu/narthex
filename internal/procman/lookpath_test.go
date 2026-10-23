package procman

import (
	"os"
	"path/filepath"
	"strings"
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

// childPathEnv appends only the existing Homebrew dirs that PATH does not
// already contain, keeping the caller's order (the no-open shim must stay
// first).
func TestChildPathEnv(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	saved := brewBinDirs
	brewBinDirs = []string{dirA, missing, dirB}
	defer func() { brewBinDirs = saved }()

	env := []string{"HOME=/tmp", "PATH=/shim" + string(os.PathListSeparator) + dirA}
	got := childPathEnv(env)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "HOME=/tmp") {
		t.Fatalf("childPathEnv dropped unrelated entries: %v", got)
	}
	wantPath := "PATH=/shim" + string(os.PathListSeparator) + dirA + string(os.PathListSeparator) + dirB
	if got[1] != wantPath {
		t.Fatalf("PATH = %q, want %q", got[1], wantPath)
	}
	if strings.Contains(joined, missing) {
		t.Fatalf("childPathEnv appended a missing dir: %q", joined)
	}

	// Calling it again is a no-op.
	again := childPathEnv(got)
	for i := range again {
		if again[i] != got[i] {
			t.Fatalf("childPathEnv is not idempotent: %v -> %v", got, again)
		}
	}

	// No PATH entry at all: one is added.
	got = childPathEnv([]string{"HOME=/tmp"})
	if len(got) != 2 || !strings.HasPrefix(got[1], "PATH="+dirA) {
		t.Fatalf("childPathEnv without PATH = %v", got)
	}
}

func TestChildPathEnvNoBrewDirs(t *testing.T) {
	saved := brewBinDirs
	brewBinDirs = nil
	defer func() { brewBinDirs = saved }()
	env := []string{"PATH=/usr/bin:/bin"}
	got := childPathEnv(env)
	if len(got) != 1 || got[0] != env[0] {
		t.Fatalf("childPathEnv should not change env without brew dirs: %v", got)
	}
}
