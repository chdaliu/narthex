package procman

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// startVSCode launches the VS Code web server from the `code` CLI.
func (m *Manager) startVSCode(dir, id string, port int) (int, error) {
	bin := DetectVSCode()
	if bin == "" {
		return 0, fmt.Errorf("VS Code CLI (code) not found in PATH")
	}
	return m.startCodeServe(bin, dir, id, port, m.VscodeHostname, m.VscodeConnectionToken, m.VscodeDataDir, m.VscodeMachineSettingsFile)
}

// startVSCodium launches the VSCodium web server from the `codium` CLI.
func (m *Manager) startVSCodium(dir, id string, port int) (int, error) {
	bin := DetectVSCodium()
	if bin == "" {
		return 0, fmt.Errorf("VSCodium CLI (codium) not found in PATH")
	}
	return m.startCodeServe(bin, dir, id, port, m.VscodiumHostname, m.VscodiumConnectionToken, m.VscodiumDataDir, m.VscodiumMachineSettingsFile)
}

// startCodeServe launches the shared VS Code-family web server:
// `<code|codium> serve-web --host <h> --port <p> --connection-token <t>
// --server-data-dir <d> --accept-server-license-terms --disable-telemetry`.
// The working directory dir is where the server starts; the web UI lets
// users pick a folder from the server's filesystem.
//
// The connection token is mandatory: the server binds a non-loopback
// address, and the token is what the browser asks for. The child PATH is
// shadowed with the no-open dir so the CLI can never pop a browser tab on
// the host (the "Open" button is the entry point).
//
// serverDataDir pins the server-side data directory: browser user settings
// are stored in-browser and can be evicted (e.g. Safari's ITP), so settings
// that must persist go to the server-side Machine settings via Remote
// Settings (Preferences → Open Remote Settings). When machineSettingsFile
// is set, its keys are seeded into that Machine settings file first.
func (m *Manager) startCodeServe(bin, dir, id string, port int, host, token, serverDataDir, machineSettingsFile string) (int, error) {
	noOpen, err := m.noOpenDir()
	if err != nil {
		return 0, err
	}
	if serverDataDir != "" && machineSettingsFile != "" {
		if err := seedMachineSettings(serverDataDir, machineSettingsFile); err != nil {
			return 0, err
		}
	}
	env := prependPath(os.Environ(), noOpen)
	args := []string{"serve-web",
		"--host", host,
		"--port", strconv.Itoa(port),
		"--connection-token", token,
	}
	if serverDataDir != "" {
		if err := os.MkdirAll(serverDataDir, 0o700); err != nil {
			return 0, fmt.Errorf("create server data dir: %w", err)
		}
		args = append(args, "--server-data-dir", filepath.Clean(serverDataDir))
	}
	args = append(args, "--accept-server-license-terms", "--disable-telemetry")
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	return m.spawn(cmd, id)
}

// seedMachineSettings merges the JSON object at sourcePath into the
// server-side Machine settings file (<serverDataDir>/data/Machine/
// settings.json). Only keys missing from the target are added, so settings
// edited later via Remote Settings are preserved. A missing or invalid
// source file is an error: the file is explicitly configured.
func seedMachineSettings(serverDataDir, sourcePath string) error {
	srcBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read machine settings %s: %w", sourcePath, err)
	}
	var src map[string]json.RawMessage
	if err := json.Unmarshal(srcBytes, &src); err != nil {
		return fmt.Errorf("parse machine settings %s: %w", sourcePath, err)
	}
	if len(src) == 0 {
		return nil
	}
	target := filepath.Join(serverDataDir, "data", "Machine", "settings.json")
	existing := map[string]json.RawMessage{}
	if targetBytes, err := os.ReadFile(target); err == nil {
		if err := json.Unmarshal(targetBytes, &existing); err != nil {
			return fmt.Errorf("parse existing machine settings %s: %w", target, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read machine settings %s: %w", target, err)
	}
	added := false
	for k, v := range src {
		if _, ok := existing[k]; !ok {
			existing[k] = v
			added = true
		}
	}
	if !added {
		return nil
	}
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create machine settings dir: %w", err)
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("write machine settings: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("write machine settings: %w", err)
	}
	return nil
}

// DetectVSCode returns the path to the VS Code CLI (`code`), or "" when it
// is not installed. Resolution order: NARTHEX_VSCODE_BIN (test/container
// override), then the `code` executable on PATH.
func DetectVSCode() string {
	if v := os.Getenv("NARTHEX_VSCODE_BIN"); v != "" {
		return v
	}
	if p, err := exec.LookPath("code"); err == nil {
		return p
	}
	return ""
}

// DetectVSCodium returns the path to the VSCodium CLI (`codium`), or ""
// when it is not installed. Resolution order: NARTHEX_VSCODIUM_BIN
// (test/container override), then the `codium` executable on PATH.
func DetectVSCodium() string {
	if v := os.Getenv("NARTHEX_VSCODIUM_BIN"); v != "" {
		return v
	}
	if p, err := exec.LookPath("codium"); err == nil {
		return p
	}
	return ""
}
