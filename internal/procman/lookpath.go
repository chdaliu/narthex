package procman

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// brewBinDirs lists well-known Homebrew bin directories at init time so
// that lookPath can find CLIs installed there. launchd-launched serve
// runs with PATH=/usr/bin:/bin:/usr/sbin:/sbin (brew services and the
// built-in autostart job alike), so Homebrew binaries are invisible to
// exec.LookPath and every app would report as not installed.
var brewBinDirs = buildBrewBinDirs()

func buildBrewBinDirs() []string {
	dirs := []string{"/opt/homebrew/bin", "/usr/local/bin"}
	if runtime.GOOS != "darwin" {
		dirs = append(dirs, "/home/linuxbrew/.linuxbrew/bin")
		if home, err := os.UserHomeDir(); err == nil {
			dirs = append(dirs, filepath.Join(home, ".linuxbrew", "bin"))
		}
	}
	return dirs
}

// lookPath resolves name like exec.LookPath and, when the PATH lookup
// fails, probes well-known Homebrew bin directories. Every serve
// instance launched by launchd (brew services / autostart) needs this
// fallback to keep detecting the managed apps at all.
func lookPath(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, dir := range brewBinDirs {
		p := filepath.Join(dir, name)
		if f, err := exec.LookPath(p); err == nil {
			return f
		}
	}
	return ""
}
