// Package procman spawns and supervises managed instances (ComfyUI
// servers, the opencode web server).
package procman

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"narthex/internal/store"
)

// Manager starts and stops managed instances locally.
type Manager struct {
	// Hostname is the listen address passed to spawned servers (ComfyUI).
	// 0.0.0.0 makes them reachable from other devices.
	Hostname string
	// PortRange is the range a free port is picked from for server-kind
	// cards (ComfyUI).
	PortRange [2]int
	LogDir    string
	// OpencodeHostname is the listen address passed to the opencode web
	// server (0.0.0.0 makes it reachable from other devices).
	OpencodeHostname string
	// OpencodePortRange is the range a free port is picked from for the
	// opencode web server.
	OpencodePortRange [2]int
	// OpencodePassword is the basic-auth password (OPENCODE_SERVER_PASSWORD)
	// the opencode web server requires. Mandatory because it binds a
	// non-loopback address.
	OpencodePassword string
	// MdbookHostname is the listen address passed to mdBook servers
	// (0.0.0.0 makes them reachable from other devices).
	MdbookHostname string
	// MdbookPortRange is the range a free port is picked from for mdBook
	// servers.
	MdbookPortRange [2]int
	// VscodeHostname is the listen address passed to the VS Code web
	// server (0.0.0.0 makes it reachable from other devices).
	VscodeHostname string
	// VscodePortRange is the range a free port is picked from for the
	// VS Code web server.
	VscodePortRange [2]int
	// VscodeConnectionToken is the token the web UI asks for in the browser
	// (--connection-token). Mandatory because it binds a non-loopback
	// address.
	VscodeConnectionToken string
	// VscodiumHostname is the listen address passed to the VSCodium web
	// server (0.0.0.0 makes it reachable from other devices).
	VscodiumHostname string
	// VscodiumPortRange is the range a free port is picked from for the
	// VSCodium web server.
	VscodiumPortRange [2]int
	// VscodiumConnectionToken is the token the VSCodium web UI asks for in
	// the browser (--connection-token). Mandatory because it binds a
	// non-loopback address.
	VscodiumConnectionToken string
	// WettyHostname is the listen address passed to the WeTTY server
	// (0.0.0.0 makes it reachable from other devices).
	WettyHostname string
	// WettyPortRange is the range a free port is picked from for the WeTTY
	// server.
	WettyPortRange [2]int
	// WettySSHHost is the SSH server WeTTY connects to (--ssh-host).
	WettySSHHost string
	// WettySSHPort is the SSH server port (--ssh-port); 0 uses WeTTY's
	// default.
	WettySSHPort int
	// WettySSHUser is the default SSH user (--ssh-user); empty lets WeTTY
	// prompt for a username.
	WettySSHUser string
}

// New creates a Manager with sane defaults.
func New(hostname string, portRange [2]int, logDir string) *Manager {
	return &Manager{Hostname: hostname, PortRange: portRange, LogDir: logDir}
}

// Start launches the instance for kind. For comfyui a free port from the
// range is assigned and the ComfyUI server is spawned in dir; for
// opencode a free port from the opencode range is assigned and the
// opencode web server is spawned with dir as its working directory. The
// child runs in its own process group so Stop can terminate the whole
// tree. Output is appended to <LogDir>/<id>.log.
func (m *Manager) Start(kind, dir, id string) (pid int, port int, err error) {
	switch kind {
	case store.KindComfyUI:
		port, err = m.FreePort()
		if err != nil {
			return 0, 0, err
		}
		pid, err = m.startComfyUI(dir, id, port)
	case store.KindOpencode:
		port, err = freePortIn(m.OpencodePortRange)
		if err != nil {
			return 0, 0, err
		}
		pid, err = m.startOpencode(dir, id, port)
	case store.KindMdbook:
		port, err = freePortIn(m.MdbookPortRange)
		if err != nil {
			return 0, 0, err
		}
		pid, err = m.startMdbook(dir, id, port)
	case store.KindVSCode:
		port, err = freePortIn(m.VscodePortRange)
		if err != nil {
			return 0, 0, err
		}
		pid, err = m.startVSCode(dir, id, port)
	case store.KindVSCodium:
		port, err = freePortIn(m.VscodiumPortRange)
		if err != nil {
			return 0, 0, err
		}
		pid, err = m.startVSCodium(dir, id, port)
	case store.KindWetty:
		port, err = freePortIn(m.WettyPortRange)
		if err != nil {
			return 0, 0, err
		}
		pid, err = m.startWetty(dir, id, port)
	default:
		return 0, 0, fmt.Errorf("unsupported service kind: %s", kind)
	}
	return pid, port, err
}

// spawn launches cmd in its own process group, appending output to
// <LogDir>/<id>.log.
func (m *Manager) spawn(cmd *exec.Cmd, id string) (int, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	logPath := filepath.Join(m.LogDir, id+".log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open log: %w", err)
	}
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		log.Close()
		return 0, fmt.Errorf("start %s: %w", cmd.Path, err)
	}
	go func() {
		cmd.Wait()
		log.Close()
	}()
	return cmd.Process.Pid, nil
}

// Stop terminates the process group of pid.
func (m *Manager) Stop(pid int) error {
	return Stop(pid)
}

// Status reports the live state of the instance with pid/port: the
// process must be alive and its web service must answer on the port.
func (m *Manager) Status(kind string, pid, port int) Status {
	st := Status{Alive: Alive(pid)}
	if st.Alive {
		// Health checks always dial the loopback address: spawned
		// servers bind 0.0.0.0 but are reached via 127.0.0.1.
		st.Healthy = Healthy("127.0.0.1", port)
		st.MemoryKB, _ = MemoryKB(pid)
	}
	return st
}

// Stop terminates the process group: SIGTERM, then SIGKILL after a grace
// period of 5 seconds. If the recorded process group no longer exists
// (ESRCH), it falls back to signalling the pid directly.
func Stop(pid int) error {
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(-pid, syscall.SIGTERM)
	if err != nil && err != syscall.ESRCH {
		return err
	}
	if err == syscall.ESRCH {
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
			return err
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !Alive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	err = syscall.Kill(-pid, syscall.SIGKILL)
	if err == syscall.ESRCH {
		err = syscall.Kill(pid, syscall.SIGKILL)
	}
	if err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

// Alive reports whether the process with pid exists.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
