package procman

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// startWetty launches the WeTTY terminal-over-web server from the `wetty`
// CLI: `wetty --port <n> --host <host> [--ssh-host <h>] [--ssh-port <n>]
// [--ssh-user <u>] [--allowed-origin <o>]...`. The working directory dir
// is the user's home.
//
// WeTTY has no HTTP-layer auth: the browser is prompted for the SSH account
// of the configured sshHost. Remote host/command URL parameters stay
// disabled (WeTTY's defaults), so the configured SSH target cannot be
// overridden from a URL.
//
// Since WeTTY 4 the server refuses to start without at least one trusted
// origin (--allowed-origin, repeatable, exact match, no wildcards). The
// page lives on wetty's own port, so the origin is `http://<browser host>`
// + this instance's port. hosts carries the browser-side hosts the card is
// opened from (request host and configured internal address); the loopback
// variants are always allowed so local access works out of the box.
func (m *Manager) startWetty(dir, id string, port int, hosts []string) (int, error) {
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
	for _, origin := range wettyOrigins(port, hosts) {
		args = append(args, "--allowed-origin", origin)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	return m.spawn(cmd, id)
}

// wettyOrigins builds the deduplicated origin list passed to WeTTY 4's
// repeatable --allowed-origin flag. hosts are normalized to bare hostnames
// first (request hosts and internal addresses may carry a scheme and the
// dashboard's own port); callers always hand at least the loopback hosts.
func wettyOrigins(port int, hosts []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(host string) {
		if host = strings.Trim(strings.TrimSpace(normalizeHost(host)), "[]"); host == "" {
			return
		}
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = strings.Trim(h, "[]")
		}
		origin := "http://" + net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
		if !seen[origin] {
			seen[origin] = true
			out = append(out, origin)
		}
	}
	// Local access always works ("localhost" and the loopback address).
	add("localhost")
	add("127.0.0.1")
	for _, host := range hosts {
		add(host)
	}
	return out
}

// normalizeHost strips any scheme, userinfo and trailing slash from a
// configured address, leaving the host[:port] part.
func normalizeHost(addr string) string {
	addr = strings.TrimSpace(addr)
	if i := strings.Index(addr, "://"); i >= 0 {
		addr = addr[i+3:]
	}
	if i := strings.Index(addr, "@"); i >= 0 {
		addr = addr[i+1:]
	}
	if i := strings.IndexAny(addr, "/?#"); i >= 0 {
		addr = addr[:i]
	}
	return addr
}

// DetectWetty returns the path to the wetty CLI, or "" when it is not
// installed. Resolution order: NARTHEX_WETTY_BIN (test/container override),
// then the wetty executable on PATH or in a well-known Homebrew bin
// directory (see lookPath).
func DetectWetty() string {
	if v := os.Getenv("NARTHEX_WETTY_BIN"); v != "" {
		return v
	}
	return lookPath("wetty")
}
