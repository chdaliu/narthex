// Command narthex is a lightweight web dashboard to launch and manage
// desktop apps (ComfyUI Desktop) and the opencode web interface.
package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"narthex/internal/api"
	"narthex/internal/auth"
	"narthex/internal/autostart"
	"narthex/internal/i18n"
	"narthex/internal/procman"
	"narthex/internal/secretbox"
	"narthex/internal/server"
	"narthex/internal/store"
	"narthex/internal/termios"
)

const version = "0.0.1"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, i18n.T(i18n.EN, "usage"), version, filepath.Join(store.Dir(), store.ConfigFile))
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve", "start":
		err = cmdServe(os.Args[2:])
	case "setup":
		err = cmdSetup(os.Args[2:])
	case "passwd":
		err = cmdPasswd(os.Args[2:])
	case "reset-auth":
		err = cmdResetAuth(os.Args[2:])
	case "autostart":
		err = cmdAutostart(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println("narthex", version)
	case "help", "-h", "--help":
		fmt.Fprint(os.Stderr, i18n.T(i18n.EN, "usage"), version, filepath.Join(store.Dir(), store.ConfigFile))
	default:
		fmt.Fprint(os.Stderr, i18n.T(i18n.EN, "usage"), version, filepath.Join(store.Dir(), store.ConfigFile))
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// resolveLang picks the language: --lang flag wins, then config, then en.
func resolveLang(flagVal string, cfg *store.Config) (i18n.Lang, error) {
	if flagVal != "" {
		lang, ok := i18n.Parse(flagVal)
		if !ok {
			return "", fmt.Errorf(i18n.T(i18n.EN, "err.invalidLang"), flagVal)
		}
		return lang, nil
	}
	if cfg != nil {
		if lang, ok := i18n.Parse(cfg.Language); ok {
			return lang, nil
		}
	}
	return i18n.EN, nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", filepath.Join(store.Dir(), store.ConfigFile), "config file path")
	portOverride := fs.Int("port", 0, "override listen port")
	hostnameFlag := fs.String("hostname", "", "override listen hostname (e.g. 0.0.0.0 for LAN access)")
	langFlag := fs.String("lang", "", "language: en | zh-CN | zh-TW")
	fs.Parse(args)

	cfg, err := store.LoadConfig(*configPath)
	firstRun := errors.Is(err, os.ErrNotExist)
	if err != nil && !firstRun {
		return fmt.Errorf(i18n.T(i18n.EN, "err.loadConfig"), *configPath, err)
	}
	if firstRun {
		cfg = store.DefaultConfig()
	}
	lang, err := resolveLang(*langFlag, cfg)
	if err != nil {
		return err
	}
	if *langFlag != "" {
		cfg.Language = *langFlag
	}
	L := func(key string, args ...any) string {
		msg := i18n.T(lang, key)
		if len(args) > 0 {
			return fmt.Sprintf(msg, args...)
		}
		return msg
	}
	if firstRun {
		if err := firstRunSetup(*configPath, cfg, L); err != nil {
			return err
		}
	}

	changed := false
	if cfg.SessionSecret == "" {
		cfg.SessionSecret = store.RandomID(32)
		changed = true
	}
	if cfg.Opencode.Password == "" {
		// The opencode web server binds 0.0.0.0 (LAN access), so it must
		// be behind basic auth; a random password is generated once and
		// persisted. It is shown on the opencode card and printed here.
		cfg.Opencode.Password = auth.GeneratePassword(16)
		changed = true
		fmt.Println(L("msg.opencodePassword", cfg.Opencode.Password))
	}
	if cfg.Vscode.ConnectionToken == "" {
		// The VS Code web server binds 0.0.0.0 (LAN access), so
		// it must be behind a connection token; a random token is generated
		// once and persisted. It is shown on the VS Code card and printed
		// here.
		cfg.Vscode.ConnectionToken = auth.GeneratePassword(16)
		changed = true
		fmt.Println(L("msg.vscodeToken", cfg.Vscode.ConnectionToken))
	}
	if cfg.Vscodium.ConnectionToken == "" {
		// The VSCodium web server binds 0.0.0.0 (LAN access), so it must
		// be behind a connection token; a random token is generated once
		// and persisted. It is shown on the VSCodium card and printed
		// here.
		cfg.Vscodium.ConnectionToken = auth.GeneratePassword(16)
		changed = true
		fmt.Println(L("msg.vscodiumToken", cfg.Vscodium.ConnectionToken))
	}
	if *portOverride > 0 {
		cfg.Port = *portOverride
	}
	if *hostnameFlag != "" {
		cfg.Hostname = *hostnameFlag
	}
	username, err := resolveUsername(cfg)
	if err != nil {
		return err
	}
	if cfg.UsernameEnc == "" {
		// Migration for configs created before accounts existed.
		cfg.UsernameEnc, err = secretbox.Encrypt(cfg.SessionSecret, username)
		if err != nil {
			return fmt.Errorf("%s", i18n.T(i18n.EN, "err.usernameInvalid"))
		}
		changed = true
	}
	if changed {
		if err := store.SaveConfig(*configPath, cfg); err != nil {
			return fmt.Errorf("%v: %w", L("err.saveConfig"), err)
		}
	}
	if cfg.PasswordHash == "" {
		return fmt.Errorf("%s", L("err.noPassword"))
	}

	dir := filepath.Dir(*configPath)
	if err := store.EnsureDir(dir); err != nil {
		return err
	}

	backend := procman.New(cfg.ComfyUI.Hostname, cfg.ComfyUI.PortRange, filepath.Join(dir, store.LogsDir))
	backend.OpencodeHostname = cfg.Opencode.Hostname
	backend.OpencodePortRange = cfg.Opencode.PortRange
	backend.OpencodePassword = cfg.Opencode.Password
	backend.MdbookHostname = cfg.Mdbook.Hostname
	backend.MdbookPortRange = cfg.Mdbook.PortRange
	backend.VscodeHostname = cfg.Vscode.Hostname
	backend.VscodePortRange = cfg.Vscode.PortRange
	backend.VscodeConnectionToken = cfg.Vscode.ConnectionToken
	backend.VscodiumHostname = cfg.Vscodium.Hostname
	backend.VscodiumPortRange = cfg.Vscodium.PortRange
	backend.VscodiumConnectionToken = cfg.Vscodium.ConnectionToken
	backend.VscodeDataDir = dataDirFor(dir, cfg.Vscode.DataDir, "vscode-data")
	backend.VscodiumDataDir = dataDirFor(dir, cfg.Vscodium.DataDir, "vscodium-data")
	backend.VscodeMachineSettingsFile = resolveFromDir(dir, cfg.Vscode.MachineSettingsFile)
	backend.VscodiumMachineSettingsFile = resolveFromDir(dir, cfg.Vscodium.MachineSettingsFile)
	backend.WettyHostname = cfg.Wetty.Hostname
	backend.WettyPortRange = cfg.Wetty.PortRange
	backend.WettySSHHost = cfg.Wetty.SSHHost
	backend.WettySSHPort = cfg.Wetty.SSHPort
	backend.WettySSHUser = cfg.Wetty.SSHUser

	state, err := store.LoadState(filepath.Join(dir, store.StateFile))
	if err != nil {
		return fmt.Errorf(L("err.loadState"), err)
	}

	svc := api.NewService(dir, cfg, state, backend, username)
	srv := server.New(cfg, svc)

	addr := net.JoinHostPort(cfg.Hostname, strconv.Itoa(cfg.Port))
	if conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond); err == nil {
		conn.Close()
		if resp, err := http.Get("http://" + addr + "/api/session"); err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			fmt.Println(L("msg.alreadyRunning", displayAddr(cfg.Hostname, cfg.Port)))
			return nil
		}
		return fmt.Errorf(L("err.portBusy"), addr)
	}

	labels := detectedApps(cfg)
	if len(labels) == 0 {
		labels = []string{L("msg.appsNone")}
	}
	fmt.Println(L("msg.appsDetected", strings.Join(labels, ", ")))
	fmt.Println(L("msg.listening", version, addr))
	if !api.IsLoopback(cfg.Hostname) {
		fmt.Println(L("warn.insecureHTTP", addr))
	}
	return http.ListenAndServe(addr, srv.Handler())
}

// detectedApps returns the labels of the installed apps narthex manages.
// mdbook additionally requires at least one configured project directory.
func detectedApps(cfg *store.Config) []string {
	var labels []string
	if procman.DetectComfyUIApp() != "" && len(procman.DetectComfyUIInstalls()) > 0 {
		labels = append(labels, "ComfyUI")
	}
	if procman.DetectOpencode() != "" {
		labels = append(labels, "OpenCode")
	}
	if procman.DetectMdbook() != "" && len(cfg.Mdbook.Dirs) > 0 {
		labels = append(labels, "MdBook")
	}
	if procman.DetectVSCode() != "" {
		labels = append(labels, "VS Code")
	}
	if procman.DetectVSCodium() != "" {
		labels = append(labels, "VSCodium")
	}
	if procman.DetectWetty() != "" {
		labels = append(labels, "WeTTY")
	}
	return labels
}

// firstRunSetup creates a fresh config with a random password when none
// exists yet, so `narthex serve` works out of the box. The generated
// password and username are printed for the user to log in.
func firstRunSetup(configPath string, cfg *store.Config, L func(string, ...any) string) error {
	generated := auth.GeneratePassword(16)
	hash, err := auth.HashPassword(generated)
	if err != nil {
		return fmt.Errorf(L("err.hashPassword"), err)
	}
	cfg.PasswordHash = hash
	cfg.SessionSecret = store.RandomID(32)
	enc, err := secretbox.Encrypt(cfg.SessionSecret, store.DefaultUsername)
	if err != nil {
		return fmt.Errorf("%s", L("err.usernameInvalid"))
	}
	cfg.UsernameEnc = enc
	dir := filepath.Dir(configPath)
	if err := store.EnsureDir(dir); err != nil {
		return err
	}
	if err := store.SaveConfig(configPath, cfg); err != nil {
		return fmt.Errorf(L("err.saveConfig"), err)
	}
	fmt.Println(L("msg.firstRunSetup", configPath))
	fmt.Println(L("msg.passwordGenerated", generated))
	fmt.Println(L("msg.username", store.DefaultUsername))
	return nil
}

// resolveFromDir resolves a configured path against dir: an empty path is
// returned unchanged, a leading ~/ (or bare ~) is expanded to the user's
// home, an absolute path is returned as-is, and a relative path is joined
// with dir.
func resolveFromDir(dir, path string) string {
	if path == "" {
		return ""
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dir, path)
}

// dataDirFor returns the server data directory for a code-family kind:
// the configured path (resolved) when set, else <configDir>/<fallback>.
func dataDirFor(configDir, configured, fallback string) string {
	if configured == "" {
		return filepath.Join(configDir, fallback)
	}
	return resolveFromDir(configDir, configured)
}

// displayAddr returns a browser-usable address for messages: wildcard
// bind addresses (0.0.0.0 / ::) are shown as 127.0.0.1.
func displayAddr(host string, port int) string {
	if host == "0.0.0.0" || host == "::" || host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func cmdAutostart(args []string) error {
	// The action (install|uninstall|status) must be the first positional
	// arg, before any flags — Go's flag package stops at the first
	// non-flag token, so `narthex autostart install --lang` parses
	// correctly while `--lang install` would not.
	if len(args) == 0 {
		return fmt.Errorf("%s", i18n.T(i18n.EN, "err.autostartMissingAction"))
	}
	action := args[0]
	switch action {
	case "install", "uninstall", "status":
	default:
		return fmt.Errorf(i18n.T(i18n.EN, "err.autostartUnknownAction"), action)
	}

	fs := flag.NewFlagSet("autostart", flag.ExitOnError)
	configPath := fs.String("config", filepath.Join(store.Dir(), store.ConfigFile), "config file path")
	langFlag := fs.String("lang", "", "language: en | zh-CN | zh-TW")
	fs.Parse(args[1:])

	cfg, err := store.LoadConfig(*configPath)
	if err != nil {
		// Status does not need a config to inspect the plist; only install
		// needs one because the launched binary will fail to start without
		// it. Load what we can for language resolution.
		cfg = store.DefaultConfig()
	}
	lang, err := resolveLang(*langFlag, cfg)
	if err != nil {
		return err
	}
	L := func(key string, args ...any) string {
		msg := i18n.T(lang, key)
		if len(args) > 0 {
			return fmt.Sprintf(msg, args...)
		}
		return msg
	}

	spec := autostart.Spec{
		Kind:       autostart.KindServe,
		ConfigPath: *configPath,
	}
	if err := autostart.Validate(spec); err != nil {
		return err
	}

	// Install needs the config file to exist: a plist that points at a
	// missing config will fail to start at boot. Status/Uninstall tolerate
	// a missing config (they only inspect/remove the plist).
	if action == "install" {
		if _, statErr := os.Stat(*configPath); statErr != nil {
			return fmt.Errorf(L("err.autostartMissingConfig"), *configPath)
		}
	}

	switch action {
	case "install":
		st, err := autostart.Install(spec)
		if err != nil {
			if errors.Is(err, autostart.ErrUnsupportedOS) {
				return fmt.Errorf("%s", L("err.autostartUnsupportedOS"))
			}
			return fmt.Errorf(L("err.autostartInstallFailed"), err)
		}
		fmt.Println(L("msg.autostartInstalled", "narthex "+spec.Kind, L("msg.autostartLogin"), st.PlistPath))
	case "uninstall":
		if _, err := autostart.Uninstall(spec); err != nil {
			if errors.Is(err, autostart.ErrUnsupportedOS) {
				return fmt.Errorf("%s", L("err.autostartUnsupportedOS"))
			}
			return fmt.Errorf(L("err.autostartInstallFailed"), err)
		}
		fmt.Println(L("msg.autostartUninstalled", "narthex "+spec.Kind))
	case "status":
		st, err := autostart.Status(spec)
		if err != nil {
			if errors.Is(err, autostart.ErrUnsupportedOS) {
				return fmt.Errorf("%s", L("err.autostartUnsupportedOS"))
			}
			return err
		}
		if st.Installed {
			fmt.Println(L("msg.autostartStatusOn", "narthex "+spec.Kind, st.PlistPath))
		} else {
			fmt.Println(L("msg.autostartStatusOff", "narthex "+spec.Kind))
		}
	}
	return nil
}

func cmdPasswd(args []string) error {
	fs := flag.NewFlagSet("passwd", flag.ExitOnError)
	configPath := fs.String("config", filepath.Join(store.Dir(), store.ConfigFile), "config file path")
	password := fs.String("password", "", "password (non-interactive)")
	random := fs.Bool("random", false, "generate a random password")
	langFlag := fs.String("lang", "", "language: en | zh-CN | zh-TW")
	fs.Parse(args)

	cfg, err := store.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf(i18n.T(i18n.EN, "err.loadConfig"), *configPath, err)
	}
	lang, err := resolveLang(*langFlag, cfg)
	if err != nil {
		return err
	}
	L := func(key string, args ...any) string {
		msg := i18n.T(lang, key)
		if len(args) > 0 {
			return fmt.Sprintf(msg, args...)
		}
		return msg
	}

	generated := ""
	switch {
	case *random:
		generated = auth.GeneratePassword(16)
	case *password != "":
		generated = *password
	default:
		pw, err := promptPassword(lang)
		if err != nil {
			return err
		}
		generated = pw
	}
	if len(generated) < 6 {
		return fmt.Errorf("%s", L("err.passwordTooShort"))
	}

	hash, err := auth.HashPassword(generated)
	if err != nil {
		return fmt.Errorf(L("err.hashPassword"), err)
	}
	cfg.PasswordHash = hash
	username, err := resolveUsername(cfg)
	if err != nil {
		return err
	}
	if err := rotateSessionSecret(cfg, username); err != nil {
		return fmt.Errorf(L("err.saveConfig"), err)
	}

	if err := store.SaveConfig(*configPath, cfg); err != nil {
		return fmt.Errorf(L("err.saveConfig"), err)
	}

	if *password != "" || *random {
		fmt.Println(L("msg.newPasswordGenerated", generated))
	}
	fmt.Println(L("msg.passwordUpdated"))
	return nil
}

func cmdResetAuth(args []string) error {
	fs := flag.NewFlagSet("reset-auth", flag.ExitOnError)
	configPath := fs.String("config", filepath.Join(store.Dir(), store.ConfigFile), "config file path")
	username := fs.String("username", store.DefaultUsername, "username (default: narthex)")
	password := fs.String("password", "", "password (non-interactive)")
	random := fs.Bool("random", false, "generate a random password")
	langFlag := fs.String("lang", "", "language: en | zh-CN | zh-TW")
	fs.Parse(args)

	cfg, err := store.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf(i18n.T(i18n.EN, "err.loadConfig"), *configPath, err)
	}
	lang, err := resolveLang(*langFlag, cfg)
	if err != nil {
		return err
	}
	L := func(key string, args ...any) string {
		msg := i18n.T(lang, key)
		if len(args) > 0 {
			return fmt.Sprintf(msg, args...)
		}
		return msg
	}
	if len(*username) == 0 || len(*username) > 32 {
		return fmt.Errorf("%s", L("err.usernameInvalid"))
	}

	generated := ""
	switch {
	case *random:
		generated = auth.GeneratePassword(16)
	case *password != "":
		generated = *password
	default:
		pw, err := promptPassword(lang)
		if err != nil {
			return err
		}
		generated = pw
	}
	if len(generated) < 6 {
		return fmt.Errorf("%s", L("err.passwordTooShort"))
	}

	hash, err := auth.HashPassword(generated)
	if err != nil {
		return fmt.Errorf(L("err.hashPassword"), err)
	}
	cfg.PasswordHash = hash
	if err := rotateSessionSecret(cfg, *username); err != nil {
		return fmt.Errorf(L("err.saveConfig"), err)
	}
	if err := store.SaveConfig(*configPath, cfg); err != nil {
		return fmt.Errorf(L("err.saveConfig"), err)
	}

	if *password != "" || *random {
		fmt.Println(L("msg.newPasswordGenerated", generated))
	}
	fmt.Println(L("msg.username", *username))
	fmt.Println(L("msg.authReset"))
	return nil
}

func cmdSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	configPath := fs.String("config", filepath.Join(store.Dir(), store.ConfigFile), "config file path")
	password := fs.String("password", "", "password (non-interactive)")
	random := fs.Bool("random", false, "generate a random password")
	force := fs.Bool("force", false, "overwrite existing config")
	langFlag := fs.String("lang", "", "language: en | zh-CN | zh-TW")
	username := fs.String("username", store.DefaultUsername, "username (default: narthex)")
	fs.Parse(args)

	lang, err := resolveLang(*langFlag, nil)
	if err != nil {
		return err
	}
	L := func(key string, args ...any) string {
		msg := i18n.T(lang, key)
		if len(args) > 0 {
			return fmt.Sprintf(msg, args...)
		}
		return msg
	}

	if _, err := os.Stat(*configPath); err == nil && !*force {
		return fmt.Errorf(L("err.configExists"), *configPath)
	}

	cfg := store.DefaultConfig()
	cfg.Language = string(lang)

	generated := ""
	switch {
	case *random:
		generated = auth.GeneratePassword(16)
	case *password != "":
		generated = *password
	default:
		pw, err := promptPassword(lang)
		if err != nil {
			return err
		}
		generated = pw
	}
	if len(generated) < 6 {
		return fmt.Errorf("%s", L("err.passwordTooShort"))
	}

	hash, err := auth.HashPassword(generated)
	if err != nil {
		return fmt.Errorf(L("err.hashPassword"), err)
	}
	cfg.PasswordHash = hash
	cfg.SessionSecret = store.RandomID(32)
	enc, err := secretbox.Encrypt(cfg.SessionSecret, *username)
	if err != nil {
		return fmt.Errorf("%s", i18n.T(lang, "err.usernameInvalid"))
	}
	cfg.UsernameEnc = enc

	dir := filepath.Dir(*configPath)
	if err := store.EnsureDir(dir); err != nil {
		return err
	}
	if err := store.SaveConfig(*configPath, cfg); err != nil {
		return fmt.Errorf(L("err.saveConfig"), err)
	}

	if *password != "" || *random {
		fmt.Println(L("msg.passwordGenerated", generated))
	}
	fmt.Println(L("msg.configWritten", *configPath))
	fmt.Println(L("msg.username", *username))
	fmt.Println(L("msg.startServer", *configPath))
	return nil
}

// resolveUsername returns the decrypted login username, or the default
// when none is stored yet.
func resolveUsername(cfg *store.Config) (string, error) {
	if cfg.UsernameEnc == "" {
		return store.DefaultUsername, nil
	}
	name, err := secretbox.Decrypt(cfg.SessionSecret, cfg.UsernameEnc)
	if err != nil {
		return "", fmt.Errorf("%s", i18n.T(i18n.EN, "err.usernameDecrypt"))
	}
	return name, nil
}

// rotateSessionSecret rotates the session secret and re-encrypts the
// stored username with the new key (the two are coupled).
func rotateSessionSecret(cfg *store.Config, username string) error {
	cfg.SessionSecret = store.RandomID(32)
	enc, err := secretbox.Encrypt(cfg.SessionSecret, username)
	if err != nil {
		return err
	}
	cfg.UsernameEnc = enc
	return nil
}

func promptPassword(lang i18n.Lang) (string, error) {
	pw, err := termios.ReadPassword(i18n.T(lang, "prompt.password"))
	if err != nil {
		return "", fmt.Errorf(i18n.T(lang, "err.readPassword"), err)
	}
	again, err := termios.ReadPassword(i18n.T(lang, "prompt.passwordAgain"))
	if err != nil {
		return "", fmt.Errorf(i18n.T(lang, "err.readPassword"), err)
	}
	if pw != again {
		return "", fmt.Errorf("%s", i18n.T(lang, "err.passwordMismatch"))
	}
	return pw, nil
}
