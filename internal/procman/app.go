package procman

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// detectAppBundle returns the first candidate .app bundle whose main
// binary exists: the $envKey override, /Applications/<appName>, then
// ~/Applications/<appName>. Returns "" when none is installed.
func detectAppBundle(envKey, appName, binName string) string {
	var out []string
	if v := os.Getenv(envKey); v != "" {
		out = append(out, v)
	}
	out = append(out, filepath.Join("/Applications", appName))
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, "Applications", appName))
	}
	for _, cand := range out {
		if _, err := os.Stat(filepath.Join(cand, "Contents", "MacOS", binName)); err != nil {
			continue
		}
		return cand
	}
	return ""
}

// appProcessRunning reports whether any process whose command line
// contains the app bundle's binary directory is alive. This covers the
// single-instance case where a freshly spawned app hands off to an
// already-running instance and the recorded pid dies.
var appProcessRunning = realAppProcessRunning

func realAppProcessRunning(appDir string) bool {
	if appDir == "" {
		return false
	}
	pattern := filepath.Join(appDir, "Contents", "MacOS")
	out, err := exec.Command("pgrep", "-f", pattern).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
