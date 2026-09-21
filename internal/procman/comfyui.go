package procman

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// startComfyUI launches a ComfyUI server from its install directory:
// <python> main.py --port <port> --listen <host>. The python interpreter
// is resolved from the install layout (a .venv, a ComfyUI Desktop
// standalone environment, then python3/python from PATH). The install
// directory is a ComfyUI Desktop managed install (its code lives in
// <installPath>/ComfyUI); the desktop's shared models, input/output
// directories and extra launch args are passed through so the server
// sees the same storage as the desktop app (comfyLaunchArgs).
func (m *Manager) startComfyUI(dir, id string, port int) (int, error) {
	if _, err := os.Stat(filepath.Join(dir, "main.py")); err != nil {
		return 0, fmt.Errorf("no main.py in %s (pick the ComfyUI install directory)", dir)
	}
	python, err := m.comfyUIPython(dir)
	if err != nil {
		return 0, err
	}
	args := []string{"main.py", "--port", strconv.Itoa(port), "--listen", m.Hostname}
	args = append(args, comfyLaunchArgs(dir)...)
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
	seen := map[string]bool{}
	var dirs []string
	for _, in := range readComfyInstallRecords(desktopDir) {
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

// comfyInstallRecord is the subset of a ComfyUI Desktop installations.json
// record needed at launch time. The *bool fields distinguish "unset"
// (default true) from an explicit false.
type comfyInstallRecord struct {
	ID              string `json:"id"`
	InstallPath     string `json:"installPath"`
	Status          string `json:"status"`
	LaunchArgs      string `json:"launchArgs"`
	UseSharedInput  *bool  `json:"useSharedInput"`
	UseSharedOutput *bool  `json:"useSharedOutput"`
	InputDir        string `json:"inputDir"`
	OutputDir       string `json:"outputDir"`
}

// readComfyInstallRecords parses installations.json from the desktop
// support dir (empty slice when missing or malformed).
func readComfyInstallRecords(desktopDir string) []comfyInstallRecord {
	if desktopDir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(desktopDir, "installations.json"))
	if err != nil {
		return nil
	}
	var inst []comfyInstallRecord
	if json.Unmarshal(b, &inst) != nil {
		return nil
	}
	return inst
}

// comfyInstallRecordForDir finds the record whose installPath matches the
// parent of the code directory (the code dir is <installPath>/ComfyUI).
func comfyInstallRecordForDir(desktopDir, codeDir string) (comfyInstallRecord, bool) {
	installPath := filepath.Clean(filepath.Dir(codeDir))
	for _, in := range readComfyInstallRecords(desktopDir) {
		if in.InstallPath != "" && filepath.Clean(in.InstallPath) == installPath {
			return in, true
		}
	}
	return comfyInstallRecord{}, false
}

// comfyExtraModelConfigPath returns the extra_model_paths YAML ComfyUI
// Desktop would pass for the instance: the per-install file when present,
// otherwise the shared model paths file. Empty when neither exists.
func comfyExtraModelConfigPath(desktopDir, instID string) string {
	var candidates []string
	if instID != "" {
		candidates = append(candidates, filepath.Join(desktopDir, "instance-model-paths", instID+".yaml"))
	}
	candidates = append(candidates, filepath.Join(desktopDir, "shared_model_paths.yaml"))
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// comfySettings is the subset of ComfyUI Desktop settings.json used for
// the shared input/output directories.
type comfySettings struct {
	InputDir  string `json:"inputDir"`
	OutputDir string `json:"outputDir"`
}

// comfyLaunchArgs returns the launch arguments ComfyUI Desktop would pass
// to main.py for the given code directory: the shared model paths config
// (--extra-model-paths-config), the shared or per-install input/output
// directories (--input-directory/--output-directory) and the instance's
// extra launch args (e.g. --enable-manager). A code directory without a
// matching installations.json record gets no desktop args; individual
// missing pieces (no model config, no settings) are skipped.
func comfyLaunchArgs(codeDir string) []string {
	desktopDir := comfyDesktopSupportDir()
	if desktopDir == "" {
		return nil
	}
	rec, found := comfyInstallRecordForDir(desktopDir, codeDir)
	if !found {
		// Not a desktop-managed install: no desktop storage to mirror.
		return nil
	}
	var args []string

	if p := comfyExtraModelConfigPath(desktopDir, rec.ID); p != "" {
		args = append(args, "--extra-model-paths-config", p)
	}

	input, output := comfySettingsDirs(desktopDir)
	if rec.UseSharedInput != nil && !*rec.UseSharedInput {
		input = rec.InputDir
	}
	if rec.UseSharedOutput != nil && !*rec.UseSharedOutput {
		output = rec.OutputDir
	}
	if input != "" {
		os.MkdirAll(input, 0o755)
		args = append(args, "--input-directory", input)
	}
	if output != "" {
		os.MkdirAll(output, 0o755)
		args = append(args, "--output-directory", output)
	}

	if rec.LaunchArgs != "" {
		args = append(args, strings.Fields(rec.LaunchArgs)...)
	}
	return args
}

// comfySettingsDirs reads the shared input/output directories from the
// desktop settings.json (empty when missing or malformed).
func comfySettingsDirs(desktopDir string) (input, output string) {
	b, err := os.ReadFile(filepath.Join(desktopDir, "settings.json"))
	if err != nil {
		return "", ""
	}
	var s comfySettings
	if json.Unmarshal(b, &s) != nil {
		return "", ""
	}
	return s.InputDir, s.OutputDir
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
