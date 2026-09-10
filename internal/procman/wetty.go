package procman

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// startWetty launches the WeTTY terminal-over-web server from the `wetty`
// CLI: `wetty --port <n> --host <host> [--ssh-host <h>] [--ssh-port <n>]
// [--ssh-user <u>]`. The working directory dir is the user's home.
//
// WeTTY has no HTTP-layer auth: the browser is prompted for the SSH account
// of the configured sshHost. Remote host/command URL parameters stay
// disabled (WeTTY's defaults), so the configured SSH target cannot be
// overridden from a URL.
func (m *Manager) startWetty(dir, id string, port int) (int, error) {
	bin := DetectWetty()
	if bin == "" {
		return 0, fmt.Errorf("wetty CLI not found in PATH")
	}
	args := []string{"--port", strconv.Itoa(port), "--host", m.WettyHostname}
	if m.WettySSHHost != "" {
		args = append(args, "--ssh-host", m.WettySSHHost)
	}
	if m.WettySSHPort > 0 {
		args = append(args, "--ssh-port", strconv.Itoa(m.WettySSHPort))
	}
	if m.WettySSHUser != "" {
		args = append(args, "--ssh-user", m.WettySSHUser)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	return m.spawn(cmd, id)
}

// DetectWetty returns the path to the wetty CLI, or "" when it is not
// installed. Resolution order: NARTHEX_WETTY_BIN (test/container override),
// then the wetty executable on PATH.
func DetectWetty() string {
	if v := os.Getenv("NARTHEX_WETTY_BIN"); v != "" {
		return v
	}
	if p, err := exec.LookPath("wetty"); err == nil {
		return p
	}
	return ""
}
