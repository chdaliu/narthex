package procman

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// EmulatorGame is a ROM file recognized under one of the configured
// directories, with the EmulatorJS core and system it maps to.
type EmulatorGame struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Core   string `json:"core"`
	System string `json:"system"`
}

// emulatorCores maps a ROM file extension (lowercase, without the dot) to
// the EmulatorJS core and a human-readable system name. Only the common
// consoles are listed; an unknown extension is not a game.
var emulatorCores = map[string]struct{ core, system string }{
	"nes":  {"fceumm", "NES"},
	"fds":  {"fceumm", "NES"},
	"unf":  {"fceumm", "NES"},
	"unif": {"fceumm", "NES"},
	"smc":  {"snes9x", "SNES"},
	"sfc":  {"snes9x", "SNES"},
	"fig":  {"snes9x", "SNES"},
	"gd3":  {"snes9x", "SNES"},
	"gd7":  {"snes9x", "SNES"},
	"dx2":  {"snes9x", "SNES"},
	"bsx":  {"snes9x", "SNES"},
	"swc":  {"snes9x", "SNES"},
	"gb":   {"gambatte", "Game Boy"},
	"gbc":  {"gambatte", "Game Boy Color"},
	"gba":  {"mgba", "Game Boy Advance"},
	"n64":  {"mupen64plus_next", "Nintendo 64"},
	"z64":  {"mupen64plus_next", "Nintendo 64"},
	"v64":  {"mupen64plus_next", "Nintendo 64"},
	"nds":  {"melonds", "Nintendo DS"},
	"md":   {"genesis_plus_gx", "Mega Drive"},
	"gen":  {"genesis_plus_gx", "Mega Drive"},
	"smd":  {"genesis_plus_gx", "Mega Drive"},
	"sms":  {"genesis_plus_gx", "Master System"},
	"gg":   {"genesis_plus_gx", "Game Gear"},
	"32x":  {"picodrive", "Sega 32X"},
	"pce":  {"mednafen_pce", "PC Engine"},
	"ngp":  {"mednafen_ngp", "Neo Geo Pocket"},
	"ngc":  {"mednafen_ngp", "Neo Geo Pocket Color"},
	"vb":   {"beetle_vb", "Virtual Boy"},
	"lnx":  {"handy", "Atari Lynx"},
	"a26":  {"stella2014", "Atari 2600"},
	"a78":  {"prosystem", "Atari 7800"},
	"a52":  {"a5200", "Atari 5200"},
	"col":  {"gearcoleco", "ColecoVision"},
	"c64":  {"vice_x64", "Commodore 64"},
	"d64":  {"vice_x64", "Commodore 64"},
	"t64":  {"vice_x64", "Commodore 64"},
	"prg":  {"vice_x64", "Commodore 64"},
	"adf":  {"puae", "Commodore Amiga"},
	"psx":  {"pcsx_rearmed", "PlayStation"},
	"iso":  {"pcsx_rearmed", "PlayStation"},
	"cue":  {"pcsx_rearmed", "PlayStation"},
	"chd":  {"pcsx_rearmed", "PlayStation"},
	"pbp":  {"pcsx_rearmed", "PlayStation"},
}

// emulatorArchiveExts are the container formats whose inner extension (the
// one before the archive extension) selects the core, e.g. "game.nes.zip".
var emulatorArchiveExts = map[string]bool{"zip": true, "7z": true, "rar": true}

// EmulatorCoreFor returns the EmulatorJS core and system name for a ROM file,
// or ok=false when the extension is not recognized. Archived ROMs resolve
// through the extension before the archive extension.
func EmulatorCoreFor(path string) (core, system string, ok bool) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if emulatorArchiveExts[ext] {
		inner := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(inner)), ".")
	}
	e, ok := emulatorCores[ext]
	if !ok {
		return "", "", false
	}
	return e.core, e.system, true
}

// emulatorGameName is the display name of a ROM: the file base without the
// archive and ROM extensions ("game.nes.zip" → "game").
func emulatorGameName(path string) string {
	base := filepath.Base(path)
	if emulatorArchiveExts[strings.TrimPrefix(strings.ToLower(filepath.Ext(base)), ".")] {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// DetectEmulatorGames scans the configured directories recursively for ROM
// files with a recognized extension. Hidden files and directories are
// skipped; results are deduplicated and sorted by path.
func DetectEmulatorGames(dirs []string) []EmulatorGame {
	const maxGames = 5000
	seen := map[string]bool{}
	var out []EmulatorGame
	for _, root := range dirs {
		root = filepath.Clean(root)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if path != root && strings.HasPrefix(d.Name(), ".") {
					return fs.SkipDir
				}
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			core, system, ok := EmulatorCoreFor(path)
			if !ok || seen[path] {
				return nil
			}
			if len(out) >= maxGames {
				return fs.SkipAll
			}
			seen[path] = true
			out = append(out, EmulatorGame{
				Name:   emulatorGameName(path),
				Path:   path,
				Core:   core,
				System: system,
			})
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// startEmulatorJS launches the EmulatorJS player in a narthex child process:
// the hidden `emulator-serve` subcommand serves the generated player page,
// the ROM at dir and an optional local data/ directory. narthex re-execs its
// own binary because EmulatorJS ships no CLI of its own; the child binds its
// own port so the card can be reached directly as well as through the
// /emulator/ same-origin proxy.
func (m *Manager) startEmulatorJS(dir, id string, port int) (int, error) {
	bin := DetectEmulatorServer()
	if bin == "" {
		return 0, fmt.Errorf("narthex binary not found for emulator-serve")
	}
	core, _, ok := EmulatorCoreFor(dir)
	if !ok {
		return 0, fmt.Errorf("unsupported ROM file: %s", dir)
	}
	args := []string{"emulator-serve",
		"--rom", dir,
		"--core", core,
		"--name", emulatorGameName(dir),
		"--host", m.EmulatorHostname,
		"--port", strconv.Itoa(port),
	}
	if m.EmulatorDataDir != "" {
		args = append(args, "--data", m.EmulatorDataDir)
	}
	if m.EmulatorCDNVersion != "" {
		args = append(args, "--cdn", m.EmulatorCDNVersion)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = filepath.Dir(dir)
	return m.spawn(cmd, id)
}

// DetectEmulatorServer returns the binary that runs the `emulator-serve`
// subcommand: NARTHEX_EMULATOR_BIN (test override) or the current narthex
// executable. EmulatorJS has no external CLI, so narthex serves it itself.
func DetectEmulatorServer() string {
	if v := os.Getenv("NARTHEX_EMULATOR_BIN"); v != "" {
		return v
	}
	if p, err := os.Executable(); err == nil {
		return p
	}
	return ""
}
