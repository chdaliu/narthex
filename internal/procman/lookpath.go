package procman

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

// childPathEnv returns env with the existing well-known Homebrew bin
// directories appended to PATH (missing ones only). lookPath can resolve a
// CLI through that fallback while the directory is absent from PATH, but
// interpreters spawned by the CLI use PATH themselves: WeTTY is a Node
// script (`#!/usr/bin/env node`), so under launchd's minimal PATH it starts
// and immediately dies with "env: node: No such file or directory".
//
// Existing PATH entries keep their order and precedence (the no-open shim
// dir must stay first), and a PATH supplied by the caller is never
// replaced; only genuinely missing, existing directories are appended.
func childPathEnv(env []string) []string {
	pathIdx := -1
	present := map[string]bool{}
	for i, kv := range env {
		if !strings.HasPrefix(kv, "PATH=") {
			continue
		}
		pathIdx = i
		for _, dir := range filepath.SplitList(strings.TrimPrefix(kv, "PATH=")) {
			present[dir] = true
		}
	}
	var missing []string
	for _, dir := range brewBinDirs {
		if present[dir] {
			continue
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		present[dir] = true
		missing = append(missing, dir)
	}
	if len(missing) == 0 {
		return env
	}
	sep := string(os.PathListSeparator)
	if pathIdx < 0 {
		out := make([]string, 0, len(env)+1)
		out = append(out, env...)
		return append(out, "PATH="+strings.Join(missing, sep))
	}
	out := make([]string, len(env))
	copy(out, env)
	current := strings.TrimPrefix(out[pathIdx], "PATH=")
	if current == "" {
		out[pathIdx] = "PATH=" + strings.Join(missing, sep)
		return out
	}
	out[pathIdx] = "PATH=" + current + sep + strings.Join(missing, sep)
	return out
}
