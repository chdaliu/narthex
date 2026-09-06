package procman

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"narthex/internal/store"
)

// startOpencode launches the opencode web server from the opencode CLI:
// `opencode web --port <n> --hostname <host>`. The working directory dir
// is the project the server operates on. The server is secured with HTTP
// basic auth via OPENCODE_SERVER_PASSWORD / OPENCODE_SERVER_USERNAME
// (mandatory because the server binds a non-loopback address).
//
// `opencode web` opens a browser tab on the host at startup, which a
// managed card should not do (the "Open" button is the entry point). There
// is no CLI flag to disable that, so the browser-opener (`open` on macOS,
// `xdg-open` on Linux) is shadowed with a no-op on the child's PATH.
func (m *Manager) startOpencode(dir, id string, port int) (int, error) {
	bin := DetectOpencode()
	if bin == "" {
		return 0, fmt.Errorf("opencode CLI not found in PATH")
	}
	noOpen, err := m.noOpenDir()
	if err != nil {
		return 0, err
	}
	env := append(os.Environ(),
		"OPENCODE_SERVER_PASSWORD="+m.OpencodePassword,
		"OPENCODE_SERVER_USERNAME="+store.OpencodeUsername,
	)
	env = prependPath(env, noOpen)
	cmd := exec.Command(bin, "web", "--port", strconv.Itoa(port), "--hostname", m.OpencodeHostname)
	cmd.Dir = dir
	cmd.Env = env
	return m.spawn(cmd, id)
}

// noOpenDir returns a directory holding no-op `open`/`xdg-open` scripts
// that shadow the real ones on the child's PATH. The scripts forward
// non-http(s) arguments to the real command (so opening local files still
// works) and silently swallow URLs (which is what a browser launch is).
func (m *Manager) noOpenDir() (string, error) {
	dir := filepath.Join(m.LogDir, "no-open")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create no-open dir: %w", err)
	}
	for name, real := range map[string]string{
		"open":     "/usr/bin/open",
		"xdg-open": "/usr/bin/xdg-open",
	} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		script := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n  http://*|https://*) exit 0 ;;\nesac\ncommand -v %s >/dev/null 2>&1 && exec %s \"$@\"\nexit 0\n", real, real)
		if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
			return "", fmt.Errorf("write no-open script: %w", err)
		}
	}
	return dir, nil
}

// prependPath returns env with prefix placed at the front of the PATH
// entry (a new PATH entry is added when none is present).
func prependPath(env []string, prefix string) []string {
	out := make([]string, 0, len(env)+1)
	added := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			out = append(out, "PATH="+prefix+string(os.PathListSeparator)+strings.TrimPrefix(kv, "PATH="))
			added = true
		} else {
			out = append(out, kv)
		}
	}
	if !added {
		out = append(out, "PATH="+prefix)
	}
	return out
}

// DetectOpencode returns the path to the opencode CLI, or "" when it is
// not installed. Resolution order: NARTHEX_OPENCODE_BIN (test/container
// override), then the opencode executable on PATH.
func DetectOpencode() string {
	if v := os.Getenv("NARTHEX_OPENCODE_BIN"); v != "" {
		return v
	}
	if p, err := exec.LookPath("opencode"); err == nil {
		return p
	}
	return ""
}
