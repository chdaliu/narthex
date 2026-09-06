package procman

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
)

// startComfyUI launches a ComfyUI server from its install directory:
// <python> main.py --port <port> --listen <host>. The python interpreter
// is resolved from the install layout (a .venv, a ComfyUI Desktop
// standalone environment, then python3/python from PATH). The install
// directory is a ComfyUI Desktop managed install (its code lives in
// <installPath>/ComfyUI); models and input/output stay the ones the
// desktop app manages.
func (m *Manager) startComfyUI(dir, id string, port int) (int, error) {
	if _, err := os.Stat(filepath.Join(dir, "main.py")); err != nil {
		return 0, fmt.Errorf("no main.py in %s (pick the ComfyUI install directory)", dir)
	}
	python, err := m.comfyUIPython(dir)
	if err != nil {
		return 0, err
	}
	args := []string{"main.py", "--port", strconv.Itoa(port), "--listen", m.Hostname}
	cmd := exec.Command(python, args...)
	cmd.Dir = dir
	return m.spawn(cmd, id)
}

// comfyUIPython finds a python interpreter for the ComfyUI install in
// dir: a local .venv, a ComfyUI Desktop standalone environment, then a
// generic python3/python from PATH.
func (m *Manager) comfyUIPython(dir string) (string, error) {
	for _, cand := range []string{
		filepath.Join(dir, ".venv", "bin", "python3"),
		filepath.Join(dir, "standalone-env", "bin", "python3"),
		filepath.Join(dir, "standalone-env", "bin", "python"),
	} {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, nil
		}
	}
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no python interpreter found for %s (looked in .venv, standalone-env, PATH)", dir)
}

// comfyDesktopSupportDir returns the data directory of a ComfyUI Desktop
// installation: ~/Library/Application Support/Comfy Desktop on macOS,
// ~/.config/Comfy Desktop elsewhere (mirroring the XDG convention). The
// NARTHEX_COMFY_DESKTOP_DIR environment variable overrides it (tests,
// containers).
func comfyDesktopSupportDir() string {
	if d := os.Getenv("NARTHEX_COMFY_DESKTOP_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if os.Getenv("XDG_CONFIG_HOME") != "" {
		return filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "Comfy Desktop")
	}
	if _, err := os.Stat("/System/Library/CoreServices"); err == nil {
		return filepath.Join(home, "Library", "Application Support", "Comfy Desktop")
	}
	return filepath.Join(home, ".config", "Comfy Desktop")
}

// DetectComfyUIInstalls returns the code directories (containing main.py)
// of the ComfyUI Desktop managed installs recorded in the desktop app's
// installations.json.
func DetectComfyUIInstalls() []string {
	return detectComfyUIInstalls(comfyDesktopSupportDir())
}

func detectComfyUIInstalls(desktopDir string) []string {
	if desktopDir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(desktopDir, "installations.json"))
	if err != nil {
		return nil
	}
	var inst []struct {
		InstallPath string `json:"installPath"`
		Status      string `json:"status"`
	}
	if json.Unmarshal(b, &inst) != nil {
		return nil
	}
	seen := map[string]bool{}
	var dirs []string
	for _, in := range inst {
		if in.Status != "" && in.Status != "installed" {
			continue
		}
		if in.InstallPath == "" {
			continue
		}
		codeDir := filepath.Clean(filepath.Join(in.InstallPath, "ComfyUI"))
		if _, err := os.Stat(filepath.Join(codeDir, "main.py")); err != nil {
			continue
		}
		if !seen[codeDir] {
			seen[codeDir] = true
			dirs = append(dirs, codeDir)
		}
	}
	sort.Strings(dirs)
	return dirs
}

// detectComfyUIApp is a seam for tests.
var detectComfyUIApp = DetectComfyUIApp

// DetectComfyUIApp returns the Comfy Desktop.app bundle directory, or ""
// when the desktop app is not installed. Resolution order:
// NARTHEX_COMFY_APP_DIR (test/container override), then
// /Applications/Comfy Desktop.app and ~/Applications/Comfy Desktop.app.
func DetectComfyUIApp() string {
	return detectAppBundle("NARTHEX_COMFY_APP_DIR", "Comfy Desktop.app", "Comfy Desktop")
}
