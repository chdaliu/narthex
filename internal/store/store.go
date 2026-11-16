// Package store persists narthex configuration and runtime state as JSON
// files in the config directory (~/.config/narthex by default).
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ConfigFile = "config.json"
	StateFile  = "state.json"
	LogsDir    = "logs"
	// DefaultUsername is the login username used when no account is
	// configured yet (project name, lowercase).
	DefaultUsername = "narthex"
	// KindComfyUI launches the ComfyUI Desktop app.
	KindComfyUI = "comfyui"
	// KindOpencode launches the opencode web interface; the opencode CLI
	// serves its own web UI on the assigned port.
	KindOpencode = "opencode"
	// OpencodeUsername is the basic-auth username of the opencode web
	// server (OPENCODE_SERVER_USERNAME). It is shown on the opencode card
	// so users can log in to the web UI.
	OpencodeUsername = "opencode"
	// DefaultOpencodeGatewayPort is the fixed port the narthex opencode
	// reverse-proxy gateway binds by default (top of the opencode range),
	// so the card "Open" URL stays stable across restarts.
	DefaultOpencodeGatewayPort = 4399
	// KindMdbook launches an mdBook documentation server; the card points
	// at a book project (a directory containing book.toml).
	KindMdbook = "mdbook"
	// KindGitvelo launches the GitVelo web service. The GitVelo login is
	// cookie-based, so narthex cannot inject it; instead the server runs
	// with --no-auth on loopback behind the narthex gateway when the card
	// uses the reverse proxy (the default), and with its own login when
	// opened directly (see procman.startGitvelo).
	KindGitvelo = "gitvelo"
	// DefaultGitveloGatewayPort is the fixed port the narthex gitvelo
	// reverse-proxy gateway binds by default, leaving GitVelo's own
	// conventional web port (26100) to manually started instances.
	DefaultGitveloGatewayPort = 26101
	// KindVSCode launches the VS Code web server (`code serve-web`).
	KindVSCode = "vscode"
	// KindVSCodium launches the VSCodium web server (`codium serve-web`).
	// Kept separate from KindVSCode so both can run at the same time.
	KindVSCodium = "vscodium"
	// KindWetty launches the WeTTY terminal-over-web server (`wetty`).
	// WeTTY has no HTTP-layer auth: the browser authenticates with the
	// SSH account of the configured sshHost (localhost by default).
	KindWetty = "wetty"
	// KindEmulatorJS launches the EmulatorJS retro-emulator player. Unlike
	// the other kinds there is no external CLI: narthex re-execs its own
	// binary (the hidden `emulator-serve` subcommand) to serve the player
	// page and the selected ROM (see procman.startEmulatorJS).
	KindEmulatorJS = "emulatorjs"
	// KindCustom launches a user-defined command. Unlike the built-in kinds,
	// any number of custom cards may exist (they are identified by ID).
	KindCustom = "custom"
)

// Custom card command modes (CustomCard.ExecMode).
const (
	// CustomExecArgs runs the resolved program with CustomCard.Command as
	// shell-parsed arguments.
	CustomExecArgs = "args"
	// CustomExecShell runs CustomCard.Command itself through /bin/sh -c;
	// CustomCard.Exec is optional and only used for installation detection.
	CustomExecShell = "shell"
)

// Custom card port modes (CustomCard.PortMode).
const (
	// CustomPortAuto picks a free port from custom.portRange at start time
	// (reusing the card's last auto port when it is still free).
	CustomPortAuto = "auto"
	// CustomPortFixed uses CustomCard.Port.
	CustomPortFixed = "fixed"
	// CustomPortNone starts a non-web process: no health check and no URL.
	CustomPortNone = "none"
)

// Custom card proxy modes (CustomCard.ProxyMode).
const (
	// CustomProxyOff opens the app's own direct URL.
	CustomProxyOff = "off"
	// CustomProxyPath proxies the app same-origin under /custom/<id>/.
	CustomProxyPath = "path"
	// CustomProxyGateway proxies the app on a dedicated per-card listener.
	CustomProxyGateway = "gateway"
)

// ComfyUIConfig controls how ComfyUI servers are spawned.
type ComfyUIConfig struct {
	// Hostname is the listen address of the ComfyUI server. 0.0.0.0 makes
	// it reachable from other devices on the LAN.
	Hostname string `json:"hostname"`
	// PortRange is the range a free port is picked from.
	PortRange [2]int `json:"portRange"`
}

// OpencodeConfig controls how the opencode web server is spawned.
type OpencodeConfig struct {
	// Hostname is the listen address of the opencode web server. 0.0.0.0
	// makes it reachable from other devices on the LAN.
	Hostname string `json:"hostname"`
	// PortRange is the range a free port is picked from.
	PortRange [2]int `json:"portRange"`
	// GatewayPort is the fixed local port the narthex reverse-proxy
	// gateway for opencode binds, so the card "Open" URL stays stable
	// across restarts. When it is occupied a free port is picked instead.
	// 0 lets the kernel assign an ephemeral port.
	GatewayPort int `json:"gatewayPort,omitempty"`
	// Password is the basic-auth password the opencode web server uses
	// (OPENCODE_SERVER_PASSWORD). Auto-generated on first run; mandatory
	// because the server binds a non-loopback address.
	Password string `json:"password"`
}

// MdbookConfig controls how mdBook servers are spawned.
type MdbookConfig struct {
	// Hostname is the listen address of the mdBook server. 0.0.0.0 makes
	// it reachable from other devices on the LAN.
	Hostname string `json:"hostname"`
	// PortRange is the range a free port is picked from.
	PortRange [2]int `json:"portRange"`
	// Dirs are the directories under which mdBook projects (directories
	// containing book.toml) are recognized and new books are created. The
	// mdbook card kind is hidden while this list is empty.
	Dirs []string `json:"dirs,omitempty"`
}

// GitveloConfig controls how the GitVelo web server is spawned.
type GitveloConfig struct {
	// Hostname is the listen address of the GitVelo server when the card
	// opens it directly (proxy off). 0.0.0.0 makes it reachable from other
	// devices on the LAN. With the reverse proxy on the server always
	// binds 127.0.0.1 and runs with --no-auth, so the port is never
	// exposed unauthenticated.
	Hostname string `json:"hostname"`
	// PortRange is the range a free port is picked from.
	PortRange [2]int `json:"portRange"`
	// GatewayPort is the fixed local port the narthex reverse-proxy
	// gateway for gitvelo binds, so the card "Open" URL stays stable
	// across restarts. When it is occupied a free port is picked instead.
	GatewayPort int `json:"gatewayPort,omitempty"`
}

// VscodeConfig controls how the VS Code web server is spawned.
type VscodeConfig struct {
	// Hostname is the listen address of the web server. 0.0.0.0 makes it
	// reachable from other devices on the LAN.
	Hostname string `json:"hostname"`
	// PortRange is the range a free port is picked from.
	PortRange [2]int `json:"portRange"`
	// ConnectionToken is the token the web UI asks for in the browser
	// (--connection-token). Auto-generated on first run; mandatory because
	// the server binds a non-loopback address.
	ConnectionToken string `json:"connectionToken,omitempty"`
	// MachineSettingsFile is the path to a JSON file whose keys are seeded
	// into the server-side Machine settings (<server-data-dir>/data/Machine/
	// settings.json) on start. Only keys missing from the target are added,
	// so settings edited later via Remote Settings are preserved. Relative
	// paths are resolved against the config directory.
	MachineSettingsFile string `json:"machineSettingsFile,omitempty"`
	// DataDir overrides the server data directory (--server-data-dir).
	// Empty uses <config>/vscode-data. Point it at another server's dir
	// (e.g. ~/.vscodium-server) to share server-side state with an instance
	// launched manually. A leading ~/ is expanded.
	DataDir string `json:"dataDir,omitempty"`
}

// VscodiumConfig controls how the VSCodium web server is spawned. It is
// independent of VscodeConfig so both editors can run at the same time.
type VscodiumConfig struct {
	// Hostname is the listen address of the web server. 0.0.0.0 makes it
	// reachable from other devices on the LAN.
	Hostname string `json:"hostname"`
	// PortRange is the range a free port is picked from.
	PortRange [2]int `json:"portRange"`
	// ConnectionToken is the token the web UI asks for in the browser
	// (--connection-token). Auto-generated on first run; mandatory because
	// the server binds a non-loopback address.
	ConnectionToken string `json:"connectionToken,omitempty"`
	// MachineSettingsFile is the path to a JSON file whose keys are seeded
	// into the server-side Machine settings on start; independent from the
	// VS Code one. Only keys missing from the target are added.
	MachineSettingsFile string `json:"machineSettingsFile,omitempty"`
	// DataDir overrides the server data directory (--server-data-dir).
	// Empty uses <config>/vscodium-data. Point it at another server's dir
	// (e.g. ~/.vscodium-server) to share server-side state with an instance
	// launched manually. A leading ~/ is expanded.
	DataDir string `json:"dataDir,omitempty"`
}

// WeTTY SSH modes (WettyConfig.SSHMode).
const (
	// WettySSHManaged spawns a private sshd bound to 127.0.0.1 for the
	// card's lifetime (default); no system SSH server is needed.
	WettySSHManaged = "managed"
	// WettySSHSystem connects to WettyConfig.SSHHost/SSHPort instead.
	WettySSHSystem = "system"
)

// WettyConfig controls how the WeTTY terminal-over-web server is spawned.
type WettyConfig struct {
	// Hostname is the listen address of the WeTTY server. 0.0.0.0 makes it
	// reachable from other devices on the LAN.
	Hostname string `json:"hostname"`
	// PortRange is the range a free port is picked from.
	PortRange [2]int `json:"portRange"`
	// SSHMode selects how WeTTY reaches SSH: WettySSHManaged (default)
	// spawns a private sshd bound to 127.0.0.1 for the card's lifetime,
	// WettySSHSystem uses SSHHost/SSHPort as-is.
	SSHMode string `json:"sshMode,omitempty"`
	// SSHPortRange is the range a free port is picked from for the managed
	// sshd.
	SSHPortRange [2]int `json:"sshPortRange,omitempty"`
	// SSHHost is the SSH server WeTTY connects to (--ssh-host) in system
	// mode. Defaults to localhost (the narthex host). In managed mode it
	// must be a loopback address; pointing it at another machine always
	// connects directly.
	SSHHost string `json:"sshHost,omitempty"`
	// SSHPort is the SSH server port (--ssh-port) in system mode. 0 uses
	// WeTTY's default.
	SSHPort int `json:"sshPort,omitempty"`
	// SSHUser is the default SSH user (--ssh-user). Empty lets WeTTY prompt
	// for a username; in managed mode it defaults to the narthex user (the
	// only account the private sshd accepts).
	SSHUser string `json:"sshUser,omitempty"`
}

// EmulatorJSConfig controls the EmulatorJS retro-emulator card. The player
// page is served by a narthex child process (the hidden `emulator-serve`
// subcommand), reached either same-origin through the /emulator/ proxy or
// directly on its own port.
type EmulatorJSConfig struct {
	// Hostname is the listen address of the EmulatorJS server. 0.0.0.0
	// makes it reachable from other devices on the LAN.
	Hostname string `json:"hostname"`
	// PortRange is the range a free port is picked from.
	PortRange [2]int `json:"portRange"`
	// Dirs are the directories scanned for ROMs. The emulatorjs card kind
	// is hidden while this list is empty.
	Dirs []string `json:"dirs,omitempty"`
	// DataPath is a local EmulatorJS data/ directory (loader.js, cores, …)
	// used instead of the public CDN, so the player works offline. A leading
	// ~/ is expanded; empty uses https://cdn.emulatorjs.org/<cdnVersion>/data/.
	DataPath string `json:"dataPath,omitempty"`
	// CDNVersion selects the CDN build when DataPath is empty: stable
	// (default), latest or nightly.
	CDNVersion string `json:"cdnVersion,omitempty"`
}

// CustomConfig controls user-defined custom cards.
type CustomConfig struct {
	// Hostname is the listen address proposed to custom apps via the
	// {host} placeholder when the card opens directly. 0.0.0.0 makes the
	// app reachable from other devices on the LAN.
	Hostname string `json:"hostname"`
	// PortRange is the range a free port is picked from when a custom card
	// uses CustomPortAuto.
	PortRange [2]int `json:"portRange"`
	// GatewayPortRange is the range a free gateway port is picked from when
	// a custom card uses CustomProxyGateway without an explicit port.
	GatewayPortRange [2]int `json:"gatewayPortRange"`
	// PathDirs are extra directories appended to the child PATH (and used
	// by bare program-name lookups). They make tools installed outside the
	// default PATH (~/.local/bin, ~/go/bin, ...) usable under launchd.
	PathDirs []string `json:"pathDirs,omitempty"`
}

// Config is the persisted narthex configuration.
type Config struct {
	Hostname      string        `json:"hostname"`
	Port          int           `json:"port"`
	SessionSecret string        `json:"sessionSecret"`
	PasswordHash  string        `json:"passwordHash"`
	ComfyUI       ComfyUIConfig `json:"comfyui"`
	// Opencode controls the opencode web server card.
	Opencode OpencodeConfig `json:"opencode"`
	// Mdbook controls the mdBook documentation server card.
	Mdbook MdbookConfig `json:"mdbook"`
	// Gitvelo controls the GitVelo web service card.
	Gitvelo GitveloConfig `json:"gitvelo"`
	// Vscode controls the VS Code web server card.
	Vscode VscodeConfig `json:"vscode"`
	// Vscodium controls the VSCodium web server card.
	Vscodium VscodiumConfig `json:"vscodium"`
	// Wetty controls the WeTTY terminal-over-web card.
	Wetty WettyConfig `json:"wetty"`
	// EmulatorJS controls the EmulatorJS retro-emulator card.
	EmulatorJS EmulatorJSConfig `json:"emulatorjs"`
	// Custom controls user-defined custom cards.
	Custom CustomConfig `json:"custom"`
	// Language selects the UI and CLI language: en (default), zh-CN, zh-TW.
	Language string `json:"language"`
	// SessionTTLHours is the session validity in hours (default 720 = 30 days).
	SessionTTLHours int `json:"sessionTTLHours"`
	// PageBackground is the dashboard background: a preset id (e.g. bg-05)
	// or an uploaded image id.
	PageBackground string `json:"pageBackground"`
	// Slogan is the topbar slogan. nil means "use the localized default";
	// an empty string hides it; any other value is shown as-is.
	Slogan *string `json:"slogan,omitempty"`
	// UsernameEnc is the login username encrypted with AES-GCM; the key is
	// derived from SessionSecret (see internal/secretbox).
	UsernameEnc string `json:"usernameEnc"`
	// InternalAddress is the host (IP or hostname) other devices use to
	// reach the managed services. When set, card "open" URLs use it
	// instead of the visitor's request host; it may include a port.
	InternalAddress string `json:"internalAddress,omitempty"`
}

// DefaultConfig returns a sensible first-run configuration.
func DefaultConfig() *Config {
	return &Config{
		Hostname:        "0.0.0.0",
		Port:            9090,
		Language:        "en",
		SessionTTLHours: 720,
		PageBackground:  "bg-05",
		ComfyUI: ComfyUIConfig{
			Hostname:  "0.0.0.0",
			PortRange: [2]int{4100, 4299},
		},
		Opencode: OpencodeConfig{
			Hostname:    "0.0.0.0",
			PortRange:   [2]int{4300, 4499},
			GatewayPort: DefaultOpencodeGatewayPort,
		},
		Mdbook: MdbookConfig{
			Hostname:  "0.0.0.0",
			PortRange: [2]int{4500, 4699},
			Dirs:      []string{},
		},
		Gitvelo: GitveloConfig{
			Hostname:    "0.0.0.0",
			PortRange:   [2]int{5400, 5599},
			GatewayPort: DefaultGitveloGatewayPort,
		},
		Vscode: VscodeConfig{
			Hostname:  "0.0.0.0",
			PortRange: [2]int{4700, 4899},
		},
		Vscodium: VscodiumConfig{
			Hostname:  "0.0.0.0",
			PortRange: [2]int{4700, 4899},
		},
		Wetty: WettyConfig{
			Hostname:     "0.0.0.0",
			PortRange:    [2]int{4900, 5099},
			SSHMode:      WettySSHManaged,
			SSHPortRange: [2]int{5300, 5399},
			SSHHost:      "localhost",
		},
		EmulatorJS: EmulatorJSConfig{
			Hostname:   "0.0.0.0",
			PortRange:  [2]int{5100, 5299},
			Dirs:       []string{},
			CDNVersion: "stable",
		},
		Custom: CustomConfig{
			Hostname:         "0.0.0.0",
			PortRange:        [2]int{5600, 5799},
			GatewayPortRange: [2]int{26200, 26299},
			PathDirs:         []string{},
		},
	}
}

// Dir returns the default config directory: $XDG_CONFIG_HOME/narthex or
// ~/.config/narthex (matching the XDG convention).
func Dir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "narthex")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "narthex")
	}
	return filepath.Join(os.TempDir(), "narthex")
}

// EnsureDir creates the config directory and the logs subdirectory.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, LogsDir), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return nil
}

// ExpandPath resolves a configured path against baseDir: a leading ~/ (or a
// bare ~) expands to the user's home, an absolute path is returned as-is, a
// relative path is joined with baseDir, and an empty path stays empty.
func ExpandPath(baseDir, path string) string {
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
	return filepath.Join(baseDir, path)
}

// LoadConfig reads and normalizes a config file.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	def := DefaultConfig()
	if c.Hostname == "" {
		c.Hostname = def.Hostname
	}
	if c.Port == 0 {
		c.Port = def.Port
	}
	if c.ComfyUI.Hostname == "" {
		c.ComfyUI.Hostname = def.ComfyUI.Hostname
	}
	if c.ComfyUI.PortRange == [2]int{0, 0} {
		c.ComfyUI.PortRange = def.ComfyUI.PortRange
	}
	if c.Opencode.Hostname == "" {
		c.Opencode.Hostname = def.Opencode.Hostname
	}
	if c.Opencode.PortRange == [2]int{0, 0} {
		c.Opencode.PortRange = def.Opencode.PortRange
	}
	if c.Opencode.GatewayPort == 0 {
		c.Opencode.GatewayPort = def.Opencode.GatewayPort
	}
	if c.Mdbook.Hostname == "" {
		c.Mdbook.Hostname = def.Mdbook.Hostname
	}
	if c.Mdbook.PortRange == [2]int{0, 0} {
		c.Mdbook.PortRange = def.Mdbook.PortRange
	}
	if c.Gitvelo.Hostname == "" {
		c.Gitvelo.Hostname = def.Gitvelo.Hostname
	}
	if c.Gitvelo.PortRange == [2]int{0, 0} {
		c.Gitvelo.PortRange = def.Gitvelo.PortRange
	}
	if c.Gitvelo.GatewayPort == 0 {
		c.Gitvelo.GatewayPort = def.Gitvelo.GatewayPort
	}
	if c.Vscode.Hostname == "" {
		c.Vscode.Hostname = def.Vscode.Hostname
	}
	if c.Vscode.PortRange == [2]int{0, 0} {
		c.Vscode.PortRange = def.Vscode.PortRange
	}
	if c.Vscodium.Hostname == "" {
		c.Vscodium.Hostname = def.Vscodium.Hostname
	}
	if c.Vscodium.PortRange == [2]int{0, 0} {
		c.Vscodium.PortRange = def.Vscodium.PortRange
	}
	if c.Wetty.Hostname == "" {
		c.Wetty.Hostname = def.Wetty.Hostname
	}
	if c.Wetty.PortRange == [2]int{0, 0} {
		c.Wetty.PortRange = def.Wetty.PortRange
	}
	if c.Wetty.SSHHost == "" {
		c.Wetty.SSHHost = def.Wetty.SSHHost
	}
	if c.Wetty.SSHPortRange == [2]int{0, 0} {
		c.Wetty.SSHPortRange = def.Wetty.SSHPortRange
	}
	if c.Wetty.SSHMode == "" {
		// Backward compatibility: an explicitly configured sshPort meant
		// "use the system sshd on that port", so do not silently replace it
		// with a managed sshd. Fresh/old configs without it default to the
		// managed endpoint.
		if c.Wetty.SSHPort != 0 {
			c.Wetty.SSHMode = WettySSHSystem
		} else {
			c.Wetty.SSHMode = def.Wetty.SSHMode
		}
	}
	if c.Wetty.SSHMode != WettySSHManaged && c.Wetty.SSHMode != WettySSHSystem {
		return nil, fmt.Errorf("wetty.sshMode must be %q or %q, got %q", WettySSHManaged, WettySSHSystem, c.Wetty.SSHMode)
	}
	if c.EmulatorJS.Hostname == "" {
		c.EmulatorJS.Hostname = def.EmulatorJS.Hostname
	}
	if c.EmulatorJS.PortRange == [2]int{0, 0} {
		c.EmulatorJS.PortRange = def.EmulatorJS.PortRange
	}
	if c.EmulatorJS.CDNVersion == "" {
		c.EmulatorJS.CDNVersion = def.EmulatorJS.CDNVersion
	}
	if c.Custom.Hostname == "" {
		c.Custom.Hostname = def.Custom.Hostname
	}
	if c.Custom.PortRange == [2]int{0, 0} {
		c.Custom.PortRange = def.Custom.PortRange
	}
	if c.Custom.GatewayPortRange == [2]int{0, 0} {
		c.Custom.GatewayPortRange = def.Custom.GatewayPortRange
	}
	if c.Custom.PortRange[0] <= 0 || c.Custom.PortRange[1] < c.Custom.PortRange[0] {
		return nil, fmt.Errorf("custom.portRange is invalid: %v", c.Custom.PortRange)
	}
	if c.Custom.GatewayPortRange[0] <= 0 || c.Custom.GatewayPortRange[1] < c.Custom.GatewayPortRange[0] {
		return nil, fmt.Errorf("custom.gatewayPortRange is invalid: %v", c.Custom.GatewayPortRange)
	}
	if c.Language == "" {
		c.Language = def.Language
	}
	if c.SessionTTLHours <= 0 {
		c.SessionTTLHours = def.SessionTTLHours
	}
	if c.PageBackground == "" {
		c.PageBackground = def.PageBackground
	}
	return &c, nil
}

// SaveConfig writes the config atomically with 0600 permissions.
func SaveConfig(path string, c *Config) error {
	return atomicWrite(path, c, 0o600)
}

// CustomCard is the configuration of a user-defined card. It is only set
// when Card.Kind is KindCustom.
type CustomCard struct {
	// ExecMode is CustomExecArgs (default) or CustomExecShell.
	ExecMode string `json:"execMode,omitempty"`
	// Exec is the program path: an absolute path, a ~/ path, or a bare
	// command name resolved from PATH (and custom.pathDirs).
	Exec string `json:"exec,omitempty"`
	// Command is the argument line in args mode, or the full shell command
	// in shell mode. {port} and {host} are replaced at start time.
	Command string `json:"command,omitempty"`
	// WorkDir is the process working directory. Empty uses the user's home.
	WorkDir string `json:"workDir,omitempty"`
	// PortMode is CustomPortAuto (default), CustomPortFixed or CustomPortNone.
	PortMode string `json:"portMode,omitempty"`
	// Port is the fixed port in CustomPortFixed mode.
	Port int `json:"port,omitempty"`
	// ProxyMode is CustomProxyOff (default), CustomProxyPath or
	// CustomProxyGateway.
	ProxyMode string `json:"proxyMode,omitempty"`
	// GatewayPort is the preferred dedicated gateway port for
	// CustomProxyGateway; 0 lets narthex allocate one from
	// custom.gatewayPortRange.
	GatewayPort int `json:"gatewayPort,omitempty"`
	// InstallEnabled exposes the install action and the install command.
	InstallEnabled bool `json:"installEnabled,omitempty"`
	// InstallAuto runs the install command automatically when a start finds
	// the configured program missing.
	InstallAuto bool `json:"installAuto,omitempty"`
	// InstallCommand is the non-interactive install command (default
	// "brew install " at creation time).
	InstallCommand string `json:"installCommand,omitempty"`
}

// Card is a managed app entry. Built-in kinds have at most one card each
// (Kind is their identity); custom cards are identified by ID and may be
// repeated.
type Card struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	Background string `json:"background"`
	PID        int    `json:"pid"`
	Port       int    `json:"port"`
	StartedAt  int64  `json:"startedAt"`
	// Kind selects the managed app: "comfyui" ... "emulatorjs" or "custom".
	Kind string `json:"kind"`
	// Dir is the project directory the card operates on. Set for kinds that
	// pick a project (mdbook) and for emulatorjs (the selected ROM file).
	Dir string `json:"dir,omitempty"`
	// DisableProxy turns off the same-origin reverse proxy for the
	// proxy-capable built-in kinds (mdbook/emulatorjs/opencode/gitvelo):
	// the card then opens the app's own direct URL instead. Missing/false
	// keeps the proxy on. Custom cards use Custom.ProxyMode instead.
	DisableProxy bool `json:"disableProxy,omitempty"`
	// LastPort is the last auto-assigned port of a custom card, so a
	// CustomPortAuto card keeps a stable URL while that port stays free.
	LastPort int `json:"lastPort,omitempty"`
	// Custom is set for KindCustom cards.
	Custom *CustomCard `json:"custom,omitempty"`
}

// State is the persisted card list.
type State struct {
	Cards []Card `json:"cards"`
}

// LoadState reads the state file; a missing file yields an empty state.
func LoadState(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &State{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if s.Cards == nil {
		s.Cards = []Card{}
	}
	// Built-in cards are identified by kind: unknown or duplicated kinds
	// are dropped, keeping the first occurrence. Custom cards are identified
	// by ID and may be repeated.
	seenKind := map[string]bool{}
	seenID := map[string]bool{}
	kept := s.Cards[:0]
	for _, c := range s.Cards {
		if c.ID == "" || seenID[c.ID] {
			continue
		}
		if c.Kind == KindCustom {
			if c.Custom == nil {
				c.Custom = &CustomCard{}
			}
			normalizeCustomCard(c.Custom)
			seenID[c.ID] = true
			kept = append(kept, c)
			continue
		}
		if c.Kind != KindComfyUI && c.Kind != KindOpencode && c.Kind != KindMdbook && c.Kind != KindGitvelo && c.Kind != KindVSCode && c.Kind != KindVSCodium && c.Kind != KindWetty && c.Kind != KindEmulatorJS {
			continue
		}
		if seenKind[c.Kind] {
			continue
		}
		seenKind[c.Kind] = true
		seenID[c.ID] = true
		kept = append(kept, c)
	}
	s.Cards = kept
	if s.Cards == nil {
		s.Cards = []Card{}
	}
	return &s, nil
}

// NormalizeCustomCard fills the default modes of a custom card. It is
// exported so the API can normalize request payloads the same way the state
// loader does.
func NormalizeCustomCard(c *CustomCard) {
	normalizeCustomCard(c)
}

func normalizeCustomCard(c *CustomCard) {
	if c.ExecMode != CustomExecShell {
		c.ExecMode = CustomExecArgs
	}
	if c.PortMode != CustomPortFixed && c.PortMode != CustomPortNone {
		c.PortMode = CustomPortAuto
	}
	if c.ProxyMode != CustomProxyPath && c.ProxyMode != CustomProxyGateway {
		c.ProxyMode = CustomProxyOff
	}
}

// SaveState writes the state file atomically.
func SaveState(path string, s *State) error {
	return atomicWrite(path, s, 0o600)
}

// RandomID returns n random bytes as hex.
func RandomID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func atomicWrite(path string, v any, perm os.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
