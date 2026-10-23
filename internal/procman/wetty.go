package procman

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"narthex/internal/store"
)

// defaultWettySSHPortRange is the fallback port range for the managed sshd
// when the manager was not configured with one.
var defaultWettySSHPortRange = [2]int{5300, 5399}

// managedSSHDState records the sidecar sshd spawned for a WeTTY card so it
// can be reaped across narthex restarts (the in-process exit callback is
// gone after a restart). It lives next to the generated sshd config.
type managedSSHDState struct {
	WettyPID int `json:"wettyPid"`
	SSHPID   int `json:"sshdPid"`
	SSHPort  int `json:"sshPort"`
}

// startWetty launches the WeTTY terminal-over-web server from the `wetty`
// CLI: `wetty --port <n> --host <host> [--ssh-host <h>] [--ssh-port <n>]
// [--ssh-user <u>] [--allowed-origin <o>]...`. The working directory dir
// is the user's home.
//
// WeTTY has no HTTP-layer auth: the browser is prompted for an SSH password
// (the narthex OS account in managed mode, the configured sshHost account in
// system mode). Remote host/command URL parameters stay disabled (WeTTY's
// defaults), so the configured SSH target cannot be overridden from a URL.
//
// In the default managed mode (see wettyManagedSSH) narthex spawns a private
// sshd bound to 127.0.0.1 for the card's lifetime, so the host does not need
// the system SSH server / Remote Login enabled. The sshd uses PAM password
// auth, only accepts the narthex user, and dies with the card. Pointing
// sshHost at another machine (or setting sshMode=system) keeps the previous
// behavior and connects to that SSH server directly.
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

	sshHost := m.WettySSHHost
	sshPort := m.WettySSHPort
	sshUser := m.WettySSHUser
	managed := m.wettyManagedSSH()

	var sshdPid int
	var err error
	if managed {
		username, userErr := currentUsername()
		if userErr != nil {
			return 0, userErr
		}
		if sshUser != "" && sshUser != username {
			return 0, fmt.Errorf("wetty.sshUser %q cannot log in through the managed sshd (only %q); set wetty.sshMode to %q to use another account", sshUser, username, store.WettySSHSystem)
		}
		sshUser = username
		// A previous serve (or a SIGKILLed narthex) can leave the private
		// sshd behind; reap it before starting a new one.
		m.cleanupManagedSSHD()
		sshdPid, sshPort, err = m.startManagedSSHD(username, id)
		if err != nil {
			return 0, err
		}
		// WeTTY talks to the private sshd over loopback only.
		sshHost = "127.0.0.1"
	}

	args := []string{"--port", strconv.Itoa(port), "--host", m.WettyHostname}
	if sshHost != "" {
		args = append(args, "--ssh-host", sshHost)
	}
	if sshPort > 0 {
		args = append(args, "--ssh-port", strconv.Itoa(sshPort))
	}
	if sshUser != "" {
		args = append(args, "--ssh-user", sshUser)
	}
	for _, origin := range wettyOrigins(port, hosts) {
		args = append(args, "--allowed-origin", origin)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir

	if !managed {
		return m.spawn(cmd, id)
	}

	wettyPid, err := m.spawn(cmd, id, func(int) { m.stopManagedSSHDProcess(sshdPid) })
	if err != nil {
		m.stopManagedSSHDProcess(sshdPid)
		return 0, err
	}
	// A broken wetty (e.g. a missing Node runtime) dies within a few
	// milliseconds; do not leave the private sshd behind in that case.
	time.Sleep(200 * time.Millisecond)
	if !Alive(wettyPid) {
		m.stopManagedSSHDProcess(sshdPid)
		return 0, fmt.Errorf("wetty exited immediately; see %s", filepath.Join(m.LogDir, id+".log"))
	}
	m.recordManagedSSHDWettyPID(sshdPid, wettyPid)
	return wettyPid, nil
}

// wettyManagedSSH reports whether the card uses narthex's private sshd. The
// configured mode decides; a non-loopback sshHost always means "connect to
// that server" because a private sshd only exists on this host.
func (m *Manager) wettyManagedSSH() bool {
	if m.WettySSHMode == store.WettySSHSystem {
		return false
	}
	return wettyLocalTarget(m.WettySSHHost)
}

// wettyLocalTarget reports whether host points at this machine (empty,
// localhost or a loopback IP, optionally with a port).
func wettyLocalTarget(host string) bool {
	host = strings.TrimSpace(normalizeHost(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// currentUsername resolves the OS account narthex runs as. `id -un` is used
// first because release builds are CGO-free and os/user's pure-Go lookup
// does not see macOS OpenDirectory accounts in all cases.
func currentUsername() (string, error) {
	if out, err := exec.Command("/usr/bin/id", "-un").Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name, nil
		}
	}
	if out, err := exec.Command("id", "-un").Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			return name, nil
		}
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username, nil
	}
	return "", fmt.Errorf("cannot determine the current user for the managed sshd")
}

// startManagedSSHD writes a loopback-only sshd config, starts the server and
// waits until it accepts TCP connections. The returned port is what WeTTY
// must use as --ssh-port.
func (m *Manager) startManagedSSHD(username, id string) (int, int, error) {
	dir := m.wettySSHDDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, 0, fmt.Errorf("create managed sshd dir: %w", err)
	}
	if err := ensureManagedSSHDHostKey(dir); err != nil {
		return 0, 0, err
	}
	portRange := m.WettySSHPortRange
	if portRange == [2]int{0, 0} {
		portRange = defaultWettySSHPortRange
	}
	port, err := freePortIn(portRange)
	if err != nil {
		return 0, 0, fmt.Errorf("pick managed sshd port: %w", err)
	}
	configPath := m.managedSSHDConfigPath()
	if err := os.WriteFile(configPath, []byte(managedSSHDConfig(username, port, dir)), 0o600); err != nil {
		return 0, 0, fmt.Errorf("write managed sshd config: %w", err)
	}
	bin := managedSSHDBin()
	if bin == "" {
		return 0, 0, fmt.Errorf("sshd not found (PATH or /usr/sbin/sshd); set wetty.sshMode to %q to use an external SSH server", store.WettySSHSystem)
	}
	cmd := exec.Command(bin, "-D", "-e", "-f", configPath, "-p", strconv.Itoa(port))
	cmd.Dir = dir
	logID := id + "-sshd"
	pid, err := m.spawn(cmd, logID)
	if err != nil {
		return 0, 0, err
	}
	deadline := time.Now().Add(3 * time.Second)
	for !Healthy("127.0.0.1", port) {
		if !Alive(pid) || time.Now().After(deadline) {
			Stop(pid)
			return 0, 0, fmt.Errorf("managed sshd did not start on 127.0.0.1:%d; see %s", port, filepath.Join(m.LogDir, logID+".log"))
		}
		time.Sleep(50 * time.Millisecond)
	}

	m.wettyMu.Lock()
	defer m.wettyMu.Unlock()
	if err := m.writeManagedSSHDStateLocked(managedSSHDState{SSHPID: pid, SSHPort: port}); err != nil {
		Stop(pid)
		return 0, 0, err
	}
	return pid, port, nil
}

// managedSSHDConfig renders the private sshd configuration. It is loopback
// only, password/PAM only, and restricted to the narthex user; root login
// and every forwarding feature stay disabled.
func managedSSHDConfig(username string, port int, dir string) string {
	return fmt.Sprintf(`# Generated by narthex for the WeTTY card's private SSH endpoint.
# Loopback only: WeTTY connects to it, the LAN never does.
Port %d
ListenAddress 127.0.0.1
HostKey %s
PidFile %s
PasswordAuthentication yes
PubkeyAuthentication no
KbdInteractiveAuthentication no
PermitEmptyPasswords no
PermitRootLogin no
AllowUsers %s
UsePAM yes
StrictModes no
MaxAuthTries 6
LoginGraceTime 30
X11Forwarding no
AllowAgentForwarding no
AllowTcpForwarding no
AllowStreamLocalForwarding no
PermitTunnel no
PrintMotd no
UseDNS no
LogLevel INFO
`,
		port,
		sshdConfigQuote(filepath.Join(dir, "host_ed25519")),
		sshdConfigQuote(filepath.Join(dir, "sshd.pid")),
		username,
	)
}

// sshdConfigQuote double-quotes a path for sshd_config (which accepts
// quoted strings, unlike shell-style escaping).
func sshdConfigQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// ensureManagedSSHDHostKey creates the private host key once. WeTTY passes
// UserKnownHostsFile=/dev/null and StrictHostKeyChecking=no, so the key only
// has to be stable and readable by sshd, not known to the client.
func ensureManagedSSHDHostKey(dir string) error {
	key := filepath.Join(dir, "host_ed25519")
	if info, err := os.Stat(key); err == nil {
		if info.Mode().Perm()&0o077 != 0 {
			return os.Chmod(key, 0o600)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat managed sshd host key: %w", err)
	}
	bin := lookPath("ssh-keygen")
	if bin == "" {
		if _, err := os.Stat("/usr/bin/ssh-keygen"); err == nil {
			bin = "/usr/bin/ssh-keygen"
		}
	}
	if bin == "" {
		return fmt.Errorf("ssh-keygen not found; cannot create the managed sshd host key")
	}
	out, err := exec.Command(bin, "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput()
	if err != nil {
		return fmt.Errorf("generate managed sshd host key: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return os.Chmod(key, 0o600)
}

// managedSSHDBin resolves the sshd binary (NARTHEX_SSHD_BIN overrides, for
// tests/containers), then PATH, then the usual absolute locations.
func managedSSHDBin() string {
	if v := os.Getenv("NARTHEX_SSHD_BIN"); v != "" {
		return v
	}
	if p := lookPath("sshd"); p != "" {
		return p
	}
	for _, p := range []string{"/usr/sbin/sshd", "/usr/local/sbin/sshd"} {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// wettySSHDDir is where the generated config, host key and pid state live.
func (m *Manager) wettySSHDDir() string {
	if m.WettySSHDDir != "" {
		return m.WettySSHDDir
	}
	return filepath.Join(m.LogDir, "wetty-sshd")
}

func (m *Manager) managedSSHDConfigPath() string {
	return filepath.Join(m.wettySSHDDir(), "sshd_config")
}

func (m *Manager) managedSSHDStatePath() string {
	return filepath.Join(m.wettySSHDDir(), "state.json")
}

// cleanupManagedSSHD reaps a sidecar sshd left behind by a previous serve or
// a killed narthex. A still-running card is left alone.
func (m *Manager) cleanupManagedSSHD() {
	m.wettyMu.Lock()
	defer m.wettyMu.Unlock()
	st, ok := m.readManagedSSHDStateLocked()
	if !ok {
		return
	}
	if st.WettyPID > 0 && Alive(st.WettyPID) {
		return
	}
	if st.SSHPID > 0 && m.isManagedSSHDPid(st.SSHPID) {
		Stop(st.SSHPID)
	}
	m.clearManagedSSHDStateLocked()
}

// stopManagedSSHD kills the sidecar owned by the WeTTY process wettyPid (0
// means "any"), so stopping another card can never touch it.
func (m *Manager) stopManagedSSHD(wettyPid int) {
	m.wettyMu.Lock()
	defer m.wettyMu.Unlock()
	st, ok := m.readManagedSSHDStateLocked()
	if !ok {
		return
	}
	if wettyPid > 0 && st.WettyPID > 0 && st.WettyPID != wettyPid {
		return
	}
	if st.SSHPID > 0 && m.isManagedSSHDPid(st.SSHPID) {
		Stop(st.SSHPID)
	}
	m.clearManagedSSHDStateLocked()
}

// stopManagedSSHDProcess is the wetty exit hook: it targets exactly the
// sidecar it was started with, so a fast restart can never kill the new one.
func (m *Manager) stopManagedSSHDProcess(sshdPid int) {
	m.wettyMu.Lock()
	defer m.wettyMu.Unlock()
	if sshdPid > 0 && m.isManagedSSHDPid(sshdPid) {
		Stop(sshdPid)
	}
	st, ok := m.readManagedSSHDStateLocked()
	if ok && (st.SSHPID == 0 || st.SSHPID == sshdPid) {
		m.clearManagedSSHDStateLocked()
	}
}

// recordManagedSSHDWettyPID upgrades the sidecar state with the wetty pid
// once the server survived its startup grace period.
func (m *Manager) recordManagedSSHDWettyPID(sshdPid, wettyPid int) {
	m.wettyMu.Lock()
	defer m.wettyMu.Unlock()
	st, ok := m.readManagedSSHDStateLocked()
	if !ok || st.SSHPID != sshdPid {
		return
	}
	st.WettyPID = wettyPid
	_ = m.writeManagedSSHDStateLocked(st)
}

func (m *Manager) readManagedSSHDStateLocked() (managedSSHDState, bool) {
	b, err := os.ReadFile(m.managedSSHDStatePath())
	if err != nil {
		return managedSSHDState{}, false
	}
	var st managedSSHDState
	if err := json.Unmarshal(b, &st); err != nil {
		return managedSSHDState{}, false
	}
	return st, true
}

func (m *Manager) writeManagedSSHDStateLocked(st managedSSHDState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	path := m.managedSSHDStatePath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write managed sshd state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("write managed sshd state: %w", err)
	}
	return nil
}

func (m *Manager) clearManagedSSHDStateLocked() {
	os.Remove(m.managedSSHDStatePath())
}

// isManagedSSHDPid guards against PID reuse: only a process whose command
// line references our generated config may be killed from the state file.
func (m *Manager) isManagedSSHDPid(pid int) bool {
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("ps", "-ww", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), m.managedSSHDConfigPath())
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
