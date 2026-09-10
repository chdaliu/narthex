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
	// KindMdbook launches an mdBook documentation server; the card points
	// at a book project (a directory containing book.toml).
	KindMdbook = "mdbook"
	// KindVSCode launches the VS Code web server (`code serve-web`).
	KindVSCode = "vscode"
	// KindVSCodium launches the VSCodium web server (`codium serve-web`).
	// Kept separate from KindVSCode so both can run at the same time.
	KindVSCodium = "vscodium"
	// KindWetty launches the WeTTY terminal-over-web server (`wetty`).
	// WeTTY has no HTTP-layer auth: the browser authenticates with the
	// SSH account of the configured sshHost (localhost by default).
	KindWetty = "wetty"
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
}

// WettyConfig controls how the WeTTY terminal-over-web server is spawned.
type WettyConfig struct {
	// Hostname is the listen address of the WeTTY server. 0.0.0.0 makes it
	// reachable from other devices on the LAN.
	Hostname string `json:"hostname"`
	// PortRange is the range a free port is picked from.
	PortRange [2]int `json:"portRange"`
	// SSHHost is the SSH server WeTTY connects to (--ssh-host). Defaults to
	// localhost (the narthex host).
	SSHHost string `json:"sshHost,omitempty"`
	// SSHPort is the SSH server port (--ssh-port). 0 uses WeTTY's default.
	SSHPort int `json:"sshPort,omitempty"`
	// SSHUser is the default SSH user (--ssh-user). Empty lets WeTTY prompt
	// for a username.
	SSHUser string `json:"sshUser,omitempty"`
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
	// Vscode controls the VS Code web server card.
	Vscode VscodeConfig `json:"vscode"`
	// Vscodium controls the VSCodium web server card.
	Vscodium VscodiumConfig `json:"vscodium"`
	// Wetty controls the WeTTY terminal-over-web card.
	Wetty WettyConfig `json:"wetty"`
	// Language selects the UI and CLI language: en (default), zh-CN, zh-TW.
	Language string `json:"language"`
	// SessionTTLHours is the session validity in hours (default 720 = 30 days).
	SessionTTLHours int `json:"sessionTTLHours"`
	// CompactCards renders the dashboard in compact mode.
	CompactCards bool `json:"compactCards"`
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
		CompactCards:    false,
		PageBackground:  "bg-05",
		ComfyUI: ComfyUIConfig{
			Hostname:  "0.0.0.0",
			PortRange: [2]int{4100, 4299},
		},
		Opencode: OpencodeConfig{
			Hostname:  "0.0.0.0",
			PortRange: [2]int{4300, 4499},
		},
		Mdbook: MdbookConfig{
			Hostname:  "0.0.0.0",
			PortRange: [2]int{4500, 4699},
			Dirs:      []string{},
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
			Hostname:  "0.0.0.0",
			PortRange: [2]int{4900, 5099},
			SSHHost:   "localhost",
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
	if c.Mdbook.Hostname == "" {
		c.Mdbook.Hostname = def.Mdbook.Hostname
	}
	if c.Mdbook.PortRange == [2]int{0, 0} {
		c.Mdbook.PortRange = def.Mdbook.PortRange
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

// Card is a managed desktop-app entry. Each kind (comfyui) has at most
// one card; Kind is the unique identity.
type Card struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	Background string `json:"background"`
	PID        int    `json:"pid"`
	Port       int    `json:"port"`
	StartedAt  int64  `json:"startedAt"`
	// Kind selects the managed app: "comfyui".
	Kind string `json:"kind"`
	// Dir is the project directory the card operates on. Only set for
	// kinds that pick a project (mdbook); other kinds resolve their
	// install directory at start time.
	Dir string `json:"dir,omitempty"`
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
	// Cards are identified by kind: unknown or duplicated kinds are
	// dropped, keeping the first occurrence.
	seen := map[string]bool{}
	kept := s.Cards[:0]
	for _, c := range s.Cards {
		if c.Kind != KindComfyUI && c.Kind != KindOpencode && c.Kind != KindMdbook && c.Kind != KindVSCode && c.Kind != KindVSCodium && c.Kind != KindWetty {
			continue
		}
		if seen[c.Kind] {
			continue
		}
		seen[c.Kind] = true
		kept = append(kept, c)
	}
	s.Cards = kept
	if s.Cards == nil {
		s.Cards = []Card{}
	}
	return &s, nil
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
