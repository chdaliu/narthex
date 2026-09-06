// Package autostart installs and removes a macOS launchd job so narthex
// starts automatically at login (user-level LaunchAgent).
//
// The pure helpers (Label, PlistPath, Render) work on any OS for
// testability; Install/Uninstall/Status call launchctl and only run on
// darwin. The launchctl and filesystem calls are exposed as package-level
// function variables so tests in this package can swap them without
// touching the real system.
package autostart

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

const (
	KindServe = "serve"
)

var (
	ErrUnsupportedOS = errors.New("autostart: unsupported OS (macOS only)")
)

// Spec describes the user-level launchd job to install.
type Spec struct {
	Kind       string // KindServe
	BinaryPath string // absolute path to the narthex binary; empty → os.Executable
	ConfigPath string // --config flag value written into the plist
	LogPath    string // optional; defaults to <config dir>/<kind>.log
}

// InstallStatus reports the install state of a Spec.
type InstallStatus struct {
	Installed bool   `json:"installed"` // plist file exists
	Loaded    bool   `json:"loaded"`    // launchctl reports the service loaded
	Label     string `json:"label"`
	PlistPath string `json:"plistPath"`
}

// Validate checks the spec kind.
func Validate(spec Spec) error {
	if spec.Kind != KindServe {
		return fmt.Errorf("autostart: unsupported kind %q", spec.Kind)
	}
	return nil
}

// Label returns the launchd label, e.g. "com.narthex.serve".
func Label(spec Spec) string {
	return "com.narthex." + spec.Kind
}

// PlistPath returns where the plist file lives: ~/Library/LaunchAgents.
func PlistPath(spec Spec) (string, error) {
	name := Label(spec) + ".plist"
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", name), nil
}

// logPath resolves the StandardOut/StandardError path: spec.LogPath if
// set, otherwise <config dir>/<kind>.log.
func logPath(spec Spec) string {
	if spec.LogPath != "" {
		return spec.LogPath
	}
	dir := filepath.Dir(spec.ConfigPath)
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, spec.Kind+".log")
}

// resolveBinary returns the absolute binary path written into the plist.
// os.Executable is the source of truth so the launchd job runs the same
// binary the user just invoked.
func resolveBinary(spec Spec) (string, error) {
	if spec.BinaryPath != "" {
		return spec.BinaryPath, nil
	}
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved, nil
	}
	return p, nil
}

// programArgs builds the argv written into ProgramArguments.
func programArgs(spec Spec) ([]string, error) {
	bin, err := resolveBinary(spec)
	if err != nil {
		return nil, err
	}
	args := []string{bin, spec.Kind}
	if spec.ConfigPath != "" {
		args = append(args, "--config", spec.ConfigPath)
	}
	return args, nil
}

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>{{.Label}}</string>
  <key>ProgramArguments</key>
  <array>
{{range .Args}}    <string>{{.}}</string>
{{end}}  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>{{.LogPath}}</string>
  <key>StandardErrorPath</key>
  <string>{{.LogPath}}</string>
</dict>
</plist>
`

type plistData struct {
	Label   string
	Args    []string
	LogPath string
}

// Render produces the plist XML for the spec. Pure: callable on any OS.
func Render(spec Spec) (string, error) {
	if err := Validate(spec); err != nil {
		return "", err
	}
	args, err := programArgs(spec)
	if err != nil {
		return "", err
	}
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	escaped := make([]string, len(args))
	for i, a := range args {
		escaped[i] = esc.Replace(a)
	}
	tpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := tpl.Execute(&b, plistData{
		Label:   esc.Replace(Label(spec)),
		Args:    escaped,
		LogPath: esc.Replace(logPath(spec)),
	}); err != nil {
		return "", err
	}
	return b.String(), nil
}

// domainLabel is the launchctl domain target for LaunchAgents.
func domainLabel(spec Spec) string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

// --- side-effectful seams (overridable for tests) ---

var (
	fnBootstrap func(domain, plistPath string) error                   = realBootstrap
	fnBootout   func(domain, label string) error                       = realBootout
	fnLoaded    func(domain, label string) bool                        = realLoaded
	fnWriteFile func(path string, data []byte, perm os.FileMode) error = os.WriteFile
	fnRemove    func(path string) error                                = os.Remove
	fnStat      func(path string) (os.FileInfo, error)                 = os.Stat
	fnMkdirAll  func(path string, perm os.FileMode) error              = os.MkdirAll
	fnIsDarwin  func() bool                                            = func() bool { return runtime.GOOS == "darwin" }
)

func realBootstrap(domain, plistPath string) error {
	out, err := exec.Command("launchctl", "bootstrap", domain, plistPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootstrap: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func realBootout(domain, label string) error {
	out, err := exec.Command("launchctl", "bootout", domain+"/"+label).CombinedOutput()
	if err != nil {
		msg := strings.ToLower(strings.TrimSpace(string(out)))
		// Already unloaded — treat as success so (re)install and uninstall
		// are idempotent.
		if strings.Contains(msg, "no service") || strings.Contains(msg, "not loaded") || strings.Contains(msg, "not found") {
			return nil
		}
		return fmt.Errorf("launchctl bootout: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func realLoaded(domain, label string) bool {
	return exec.Command("launchctl", "print", domain+"/"+label).Run() == nil
}

// precheck enforces the platform requirements shared by
// Install/Uninstall/Status.
func precheck(spec Spec) error {
	if !fnIsDarwin() {
		return ErrUnsupportedOS
	}
	return Validate(spec)
}

// Install writes the plist and loads the job. If a job with the same label
// is already loaded it is first booted out, so Install is safe to re-run.
func Install(spec Spec) (InstallStatus, error) {
	if err := precheck(spec); err != nil {
		return InstallStatus{}, err
	}
	plistPath, err := PlistPath(spec)
	if err != nil {
		return InstallStatus{}, err
	}
	content, err := Render(spec)
	if err != nil {
		return InstallStatus{}, err
	}
	if err := fnMkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return InstallStatus{}, fmt.Errorf("create launch dir: %w", err)
	}
	if err := fnWriteFile(plistPath, []byte(content), 0o644); err != nil {
		return InstallStatus{}, fmt.Errorf("write plist: %w", err)
	}
	dom := domainLabel(spec)
	_ = fnBootout(dom, Label(spec))
	if err := fnBootstrap(dom, plistPath); err != nil {
		return InstallStatus{}, err
	}
	return InstallStatus{
		Installed: true,
		Loaded:    true,
		Label:     Label(spec),
		PlistPath: plistPath,
	}, nil
}

// Uninstall stops the job (if loaded) and removes the plist file. Idempotent.
func Uninstall(spec Spec) (InstallStatus, error) {
	if err := precheck(spec); err != nil {
		return InstallStatus{}, err
	}
	plistPath, err := PlistPath(spec)
	if err != nil {
		return InstallStatus{}, err
	}
	dom := domainLabel(spec)
	_ = fnBootout(dom, Label(spec))
	if err := fnRemove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return InstallStatus{}, fmt.Errorf("remove plist: %w", err)
	}
	return InstallStatus{Installed: false, Loaded: false, Label: Label(spec), PlistPath: plistPath}, nil
}

// Status reports whether the plist exists and whether launchd has the job
// loaded.
func Status(spec Spec) (InstallStatus, error) {
	if err := precheck(spec); err != nil {
		return InstallStatus{}, err
	}
	plistPath, err := PlistPath(spec)
	if err != nil {
		return InstallStatus{}, err
	}
	st := InstallStatus{Label: Label(spec), PlistPath: plistPath}
	if _, err := fnStat(plistPath); err == nil {
		st.Installed = true
	}
	if st.Installed {
		st.Loaded = fnLoaded(domainLabel(spec), Label(spec))
	}
	return st, nil
}
