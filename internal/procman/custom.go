package procman

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"narthex/internal/store"
)

// ResolveExecutable resolves a configured program path. A value containing a
// path separator is treated as a path (a leading ~/ is expanded, a relative
// path is resolved against the user's home) and must exist and be executable.
// A bare name is looked up on PATH, then in extraDirs, then in the well-known
// Homebrew bin directories.
func ResolveExecutable(raw string, extraDirs []string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("program path is empty")
	}
	if strings.ContainsRune(raw, os.PathSeparator) {
		p := expandHome(raw)
		if !filepath.IsAbs(p) {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, p)
			}
		}
		info, err := os.Stat(p)
		if err != nil {
			return "", fmt.Errorf("program not found: %s", raw)
		}
		if info.IsDir() || info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("program is not executable: %s", raw)
		}
		return p, nil
	}
	if p := lookPathExtra(raw, extraDirs); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("program not found in PATH: %s", raw)
}

// lookPathExtra is lookPath plus caller-provided directories.
func lookPathExtra(name string, extraDirs []string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, dir := range extraDirs {
		dir = expandHome(strings.TrimSpace(dir))
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, name)
		if f, err := exec.LookPath(p); err == nil {
			return f
		}
	}
	for _, dir := range brewBinDirs {
		p := filepath.Join(dir, name)
		if f, err := exec.LookPath(p); err == nil {
			return f
		}
	}
	return ""
}

func expandHome(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// shellQuote quotes s for /bin/sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// prepareCustom prepares a user-defined card. It resolves the program, picks
// or validates the port, expands the {port}/{host} placeholders, and runs the
// command in its own process group.
func prepareCustom(m *Manager, spec StartSpec) (PreparedLaunch, error) {
	c := spec.Launch
	if c == nil {
		c = spec.Card.Custom
	}
	if c == nil {
		return PreparedLaunch{}, fmt.Errorf("custom card has no configuration")
	}
	copy := *c
	store.NormalizeCustomCard(&copy)
	c = &copy

	execPath := ""
	var err error
	if strings.TrimSpace(c.Exec) != "" {
		execPath, err = ResolveExecutable(c.Exec, m.CustomPathDirs)
		if err != nil {
			// In shell mode Exec is optional metadata used only for install
			// detection; the shell command may resolve the program itself.
			if c.ExecMode == store.CustomExecArgs {
				return PreparedLaunch{}, err
			}
			execPath = ""
		}
	}
	if c.ExecMode == store.CustomExecArgs && execPath == "" {
		return PreparedLaunch{}, fmt.Errorf("program path is required in args mode")
	}
	if strings.TrimSpace(c.Command) == "" && c.ExecMode == store.CustomExecShell {
		return PreparedLaunch{}, fmt.Errorf("shell command is required in shell mode")
	}

	port, err := m.customPort(spec.Card, c)
	if err != nil {
		return PreparedLaunch{}, err
	}
	host := m.CustomHostname
	if host == "" {
		host = "0.0.0.0"
	}
	if c.ProxyMode == store.CustomProxyPath || c.ProxyMode == store.CustomProxyGateway {
		// Proxied apps must be reachable on the loopback address.
		host = "127.0.0.1"
	}

	line := strings.ReplaceAll(c.Command, "{port}", strconv.Itoa(port))
	line = strings.ReplaceAll(line, "{host}", host)
	if c.ExecMode == store.CustomExecArgs {
		line = "exec " + shellQuote(execPath)
		if strings.TrimSpace(c.Command) != "" {
			line += " " + c.Command
			line = strings.ReplaceAll(line, "{port}", strconv.Itoa(port))
			line = strings.ReplaceAll(line, "{host}", host)
		}
	}

	cwd := spec.Dir
	if cwd == "" {
		cwd = expandHome(strings.TrimSpace(c.WorkDir))
	}
	if cwd == "" {
		cwd, _ = os.UserHomeDir()
	}
	if info, statErr := os.Stat(cwd); statErr != nil || !info.IsDir() {
		return PreparedLaunch{}, fmt.Errorf("working directory not found: %s", cwd)
	}

	env := append(os.Environ(),
		"PORT="+strconv.Itoa(port),
		"NARTHEX_PORT="+strconv.Itoa(port),
		"NARTHEX_HOST="+host,
	)
	env = customPathEnv(env, m.CustomPathDirs)
	return PreparedLaunch{
		Path: "/bin/sh",
		Args: []string{"-c", line},
		Env:  env,
		Dir:  cwd,
		Port: port,
	}, nil
}

// customPort picks the runtime port of a custom card.
func (m *Manager) customPort(card store.Card, c *store.LaunchConfig) (int, error) {
	if c == nil {
		return 0, nil
	}
	switch c.PortMode {
	case store.CustomPortNone:
		return 0, nil
	case store.CustomPortFixed:
		if c.Port < 1 || c.Port > 65535 {
			return 0, fmt.Errorf("invalid fixed port: %d", c.Port)
		}
		if !portAvailable(c.Port) {
			return 0, fmt.Errorf("port %d is already in use", c.Port)
		}
		return c.Port, nil
	default:
		if card.LastPort > 0 && portAvailable(card.LastPort) {
			return card.LastPort, nil
		}
		return freePortIn(m.CustomPortRange)
	}
}

func portAvailable(port int) bool {
	if port <= 0 {
		return false
	}
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// customPathEnv appends existing custom path directories (plus the usual
// Homebrew dirs) to PATH. Existing entries keep their order.
func customPathEnv(env []string, extraDirs []string) []string {
	var dirs []string
	for _, dir := range extraDirs {
		dir = expandHome(strings.TrimSpace(dir))
		if dir == "" {
			continue
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			dirs = append(dirs, dir)
		}
	}
	if len(dirs) > 0 {
		env = prependPath(env, strings.Join(dirs, string(os.PathListSeparator)))
	}
	return childPathEnv(env)
}

// RunInstall executes an install command non-interactively, appending nothing
// to stdout/stderr other than the log file. It returns an error when the
// command exits non-zero, cannot start, or the context is cancelled.
func RunInstall(ctx context.Context, command, dir, logPath string, extraPathDirs []string) error {
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("install command is empty")
	}
	cwd := dir
	if cwd == "" {
		cwd, _ = os.UserHomeDir()
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open install log: %w", err)
	}
	defer log.Close()

	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Dir = cwd
	cmd.Env = customPathEnv(os.Environ(), extraPathDirs)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start install command: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		return ctx.Err()
	}
}
