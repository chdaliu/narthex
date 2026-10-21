package procman

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"narthex/internal/store"
)

// startOpencode launches the opencode web server from the opencode CLI.
// opencode v2 removed the `web` subcommand and serves the web UI from
// `opencode serve`; v1 still uses `opencode web`, so the invocation is
// picked from the CLI version (see opencodeArgs). The working directory dir
// is the project the server operates on. The server is secured with HTTP
// basic auth via OPENCODE_SERVER_PASSWORD / OPENCODE_SERVER_USERNAME
// (mandatory because the server binds a non-loopback address).
//
// `opencode web` (v1) opens a browser tab on the host at startup, which a
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
	cmd := exec.Command(bin, opencodeArgs(bin, port, m.OpencodeHostname)...)
	cmd.Dir = dir
	cmd.Env = env
	return m.spawn(cmd, id)
}

// versionRe matches the first major.minor version in CLI version output
// (`opencode v2.0.25` → 2.0).
var versionRe = regexp.MustCompile(`(\d+)\.\d+`)

// opencodeArgs returns the arguments that start the opencode web server:
// `serve --port <n> --hostname <host>` on opencode v2, `web --port <n>
// --hostname <host>` on v1 (v2 removed the `web` subcommand).
func opencodeArgs(bin string, port int, hostname string) []string {
	sub := "web"
	if opencodeMajor(bin) >= 2 {
		sub = "serve"
	}
	return []string{sub, "--port", strconv.Itoa(port), "--hostname", hostname}
}

// opencodeMajor runs `opencode --version` and returns its major version, or
// -1 when the CLI cannot be run or the output contains no version. The
// probe is bounded so a hung CLI cannot block a card start; callers fall
// back to the v1 (`web`) invocation when it fails.
func opencodeMajor(bin string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return -1
	}
	m := versionRe.FindStringSubmatch(string(out))
	if m == nil {
		return -1
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return -1
	}
	return major
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
// override), then the opencode executable on PATH or in a well-known
// Homebrew bin directory (see lookPath).
func DetectOpencode() string {
	if v := os.Getenv("NARTHEX_OPENCODE_BIN"); v != "" {
		return v
	}
	return lookPath("opencode")
}
