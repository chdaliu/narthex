package procman

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// startVSCode launches the VS Code web server from the `code` CLI.
func (m *Manager) startVSCode(dir, id string, port int) (int, error) {
	bin := DetectVSCode()
	if bin == "" {
		return 0, fmt.Errorf("VS Code CLI (code) not found in PATH")
	}
	return m.startCodeServe(bin, dir, id, port, m.VscodeHostname, m.VscodeConnectionToken)
}

// startVSCodium launches the VSCodium web server from the `codium` CLI.
func (m *Manager) startVSCodium(dir, id string, port int) (int, error) {
	bin := DetectVSCodium()
	if bin == "" {
		return 0, fmt.Errorf("VSCodium CLI (codium) not found in PATH")
	}
	return m.startCodeServe(bin, dir, id, port, m.VscodiumHostname, m.VscodiumConnectionToken)
}

// startCodeServe launches the shared VS Code-family web server:
// `<code|codium> serve-web --host <h> --port <p> --connection-token <t>
// --accept-server-license-terms --disable-telemetry`. The working
// directory dir is where the server starts; the web UI lets users pick a
// folder from the server's filesystem.
//
// The connection token is mandatory: the server binds a non-loopback
// address, and the token is what the browser asks for. The child PATH is
// shadowed with the no-open dir so the CLI can never pop a browser tab on
// the host (the "Open" button is the entry point).
func (m *Manager) startCodeServe(bin, dir, id string, port int, host, token string) (int, error) {
	noOpen, err := m.noOpenDir()
	if err != nil {
		return 0, err
	}
	env := prependPath(os.Environ(), noOpen)
	cmd := exec.Command(bin, "serve-web",
		"--host", host,
		"--port", strconv.Itoa(port),
		"--connection-token", token,
		"--accept-server-license-terms",
		"--disable-telemetry",
	)
	cmd.Dir = dir
	cmd.Env = env
	return m.spawn(cmd, id)
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
