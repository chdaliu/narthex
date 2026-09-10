# Narthex — Agent Guide (English)

This document is for AI agents / developers working in the Narthex codebase: how to understand the architecture, reproduce the build, verify changes and follow the conventions.

## 1. What Narthex is

Narthex is a **super lightweight** local web dashboard: after entering a password, it launches and stops local apps (ComfyUI servers, opencode/mdBook/VS Code/VSCodium/WeTTY web) keyed by app type, with one-click start/stop, live status and memory, and one-click access to their web UIs. Core constraints:

- **Minimal footprint**: a single static Go binary with the frontend and assets embedded via `go:embed`; ~10 MB idle memory
- **Minimal dependencies**: only `golang.org/x/crypto` (argon2id); **any new third-party dependency must be justified in this document**
- **Keyed by kind**: each app type (`comfyui` / `opencode` / `mdbook` / `vscode` / `vscodium` / `wetty`) has at most one card; `Kind` is the unique identity; **no paths, no directory search** — except the mdBook card, which stores the selected book project directory (see §7 kind semantics)
- **App detection at startup**: apps that are not installed are never shown (the `apps` field of `/api/meta` reports installed state)

## 2. Architecture and data flow

```
Browser (embedded web/*)
   │  HTTP + session cookie
   ▼
internal/server  routing + auth middleware + static files
   │
   ▼
internal/api    Service (mutex-protected State)
   │  ├─ auth: login rate limit → argon2id verify → HMAC session
   │  └─ procman.Backend interface (the ONLY exit for process operations):
   │        Manager: Start dispatches by kind and spawns the instance
   │        process (Setpgid process group)
   ▼
internal/store   atomic JSON writes: config.json / state.json
                 instance logs → logs/<cardID>.log
```

Key flows:

- **Create a card**: `api.HandleCreateCard` → kind is known → the app is installed (`err.appNotFound`) → the kind is not duplicated (`err.duplicateKind`); default name/icon per kind (ComfyUI→palette, opencode→terminal, mdbook→book-open, vscode→code, vscodium→code, wetty→terminal); an mdBook card additionally requires `dir` to be one of the projects recognized under `mdbook.dirs` (`err.mdbookNoProject`)
- **Start a card**: `api.HandleStartCard` → `Backend.Start(kind, dir, id)`; the comfyui code dir is resolved at start time by `procman.DetectComfyUIInstalls`; the opencode/VS Code/VSCodium/WeTTY working dir is `os.UserHomeDir()`; the mdBook working dir is the project directory stored on the card → spawn (cwd = the relevant dir, `Setpgid: true`, logs to `<LogDir>/<id>.log`) → record `pid/port/startedAt` in state; the opencode process carries its basic-auth password via the `OPENCODE_SERVER_PASSWORD` env var, the VS Code/VSCodium process carries its connection token via `--connection-token`, the WeTTY process gets its SSH target via `--ssh-host`/`--ssh-port`/`--ssh-user` (no HTTP-layer secret)
- **Status refresh**: the frontend polls `GET /api/cards` every 5 s; the server calls `Backend.Status(kind, pid, port)` per card (kill 0 + TCP health check + RSS)
- **Stop**: `Backend.Stop(pid)` → SIGTERM to the process group, then SIGKILL after a 5 s grace period
- **Restart recovery**: state.json persists pid/port; instances are lazily re-associated after a restart and dead processes show as "stopped"; when the desktop app is already running (single-instance handoff), `appProcessRunning` (pgrep) reports it as running
- **Password/username change**: `narthex passwd` and `narthex reset-auth` re-hash with argon2id and **rotate `sessionSecret`** (every issued session becomes invalid), re-**encrypting** the stored username with the new key (see `secretbox`); serve reads the config at startup, so restart serve after a CLI change; changing the account from the top-bar "Account" page (`/api/account`) updates memory and disk directly and takes effect immediately

## 3. File map

| Path | Responsibility |
|---|---|
| `cmd/narthex/main.go` | Entrypoint: serve / setup / passwd / reset-auth / autostart subcommands and flags; prints the detected apps at startup (`detectedApps`); generates the opencode password and the VS Code/VSCodium connection tokens on first serve (WeTTY needs no generated secret); port-in-use pre-check; `resolveUsername`/`rotateSessionSecret`/`cmdAutostart` |
| `internal/api/service.go` | `Service` struct (State + mutex), login (username+password)/session/meta/settings/account handlers, CardView status assembly (via Backend); detection seams (`ComfyUIAppDir`/`ComfyUIDirs`/`OpencodeBin`/`MdbookBin`/`MdbookProjects`/`MdbookCreate`/`VscodeBin`/`VscodiumBin`); `HandleMeta` returns `apps` (kind → installed/label); card "Open" URL assembly via `instanceURL` (prefers `Config.InternalAddress`, else the request host); `IsLoopback` (serve warning only) |
| `internal/api/handlers.go` | Card CRUD, start/stop; `appInstalled` check; `kindLabel` default names; `defaultIcon` per kind (palette/terminal/book-open/code); `installDir` resolves per kind (mdBook uses the card's `dir`) |
| `internal/api/mdbook.go` | `HandleMdbookProjects` (GET: configured dirs + recognized books) and `HandleMdbookCreate` (POST: `mdbook init` in a configured dir, with name/existence validation) |
| `internal/api/uploads.go` | Background upload/delete/enumeration, extension whitelist, upload storage dir |
| `internal/api/autostart.go` | `HandleAutostart`: GET (status) / POST (install/uninstall) for the user-level serve launchd job |
| `internal/api/json.go` | JSON read/write helpers |
| `internal/auth/password.go` | argon2id hash/verify, random password generation |
| `internal/auth/session.go` | HMAC-SHA256 stateless sessions (Sign/Verify; TTL comes from config) |
| `internal/auth/ratelimit.go` | In-memory fixed-window limiter (with Reset) |
| `internal/autostart/autostart.go` | macOS launchd install/uninstall/status for `narthex serve` only; pure helpers (`Label`/`PlistPath`/`Render`) plus side-effectful `Install`/`Uninstall`/`Status` with package-level function-variable seams (`fnBootstrap` etc.) for tests; non-darwin returns `ErrUnsupportedOS` |
| `internal/i18n/i18n.go` | CLI/API message catalogs (en / zh-CN / zh-TW), `T(lang, key)` with en fallback |
| `internal/procman/backend.go` | `Backend` interface (`Start(kind, dir, id)`/`Stop`/`Status(kind, pid, port)`) and `Status` struct (single exit for process operations) |
| `internal/procman/manager.go` | `Manager`: kind dispatch in `Start` (comfyui → server, opencode → web, mdbook → serve, vscode/vscodium → serve-web, wetty → wetty), `spawn` (process group + logs), `Stop` (SIGTERM→SIGKILL), `Status` (Alive + TCP health check), `Alive`, `FreePort`/`freePortIn` |
| `internal/procman/app.go` | `.app` detection (`detectAppBundle`: env override + `/Applications` + `~/Applications`) |
| `internal/procman/comfyui.go` | ComfyUI server launch (`main.py --port --listen`, python resolved from `standalone-env`/`.venv`/PATH), `DetectComfyUIInstalls` (code dirs from `installations.json`, overridable via `NARTHEX_COMFY_DESKTOP_DIR`), `DetectComfyUIApp` (`Comfy Desktop.app`, overridable via `NARTHEX_COMFY_APP_DIR`) |
| `internal/procman/opencode.go` | opencode web launch: `opencode web --port --hostname`, cwd = project dir, `OPENCODE_SERVER_PASSWORD` injected for basic auth; `DetectOpencode` (looks up `opencode` on PATH, overridable via `NARTHEX_OPENCODE_BIN`) |
| `internal/procman/mdbook.go` | mdBook launch: `mdbook serve --port --hostname`, cwd = book project dir; `DetectMdbook` (looks up `mdbook` on PATH, overridable via `NARTHEX_MDBOOK_BIN`); `DetectMdbookProjects` (one-level scan of configured dirs for `book.toml`); `CreateMdbookProject`/`ValidBookName` (`mdbook init --force --ignore none`, non-interactive) |
| `internal/procman/wetty.go` | WeTTY terminal launch: `wetty --port --host [--ssh-host --ssh-port --ssh-user]`, cwd = user home; `DetectWetty` (looks up `wetty` on PATH, overridable via `NARTHEX_WETTY_BIN`) |
| `internal/procman/vscode.go` | VS Code/VSCodium web launch: `<code|codium> serve-web --host --port --connection-token --accept-server-license-terms` via `startCodeServe`, cwd = user home, child PATH shadowed with the no-open dir; `DetectVSCode` (looks up `code`, overridable via `NARTHEX_VSCODE_BIN`), `DetectVSCodium` (looks up `codium`, overridable via `NARTHEX_VSCODIUM_BIN`) — two independent kinds |
| `internal/procman/probe.go` | TCP health check, memory (macOS `ps` / Linux `/proc`) |
| `internal/server/server.go` | Routes (Go 1.22+ method patterns), auth middleware, `go:embed` static files + cache headers |
| `internal/secretbox/secretbox.go` | Username encryption at rest: AES-256-GCM, key derived from `sessionSecret` via HMAC-SHA256; `Encrypt`/`Decrypt`/`DeriveKey` |
| `internal/store/store.go` | Config/State structs, atomic writes (temp file + rename), config dir (~/.config/narthex); `LoadState` dedupes by kind and drops unknown kinds |
| `internal/termios/*` | Echo-off password input (unix termios + fallback for other OSes) |
| `web/web.go` | `//go:embed` of the whole frontend |
| `web/index.html`, `app.css`, `app.js` | Single-page frontend: login/dashboard/cards/modals, glassmorphism + responsive; the add-card modal has no search step — the kind list is filtered by `meta.apps` and the already-added kinds; choosing the mdBook kind opens a project step (pick an existing book under `mdbook.dirs` or create a new one) before the pickers |
| `web/i18n.js` | Client message catalog (en/zh-CN/zh-TW), `t()`, `applyI18n()`, `data-i18n` handling |
| `web/assets/icons/*.svg` | 37 Lucide icons (ISC) |
| `web/assets/backgrounds/*.jpg` | 8 background photos (picsum/Unsplash) |
| `scripts/download-assets.sh` | Downloads assets (committed to the repo, offline builds work) |
| `scripts/smoke-test.sh` | End-to-end smoke: local flow + passwd/account session rotation + comfyui/opencode/mdbook/vscode/vscodium/wetty card lifecycles + mdbook projects API + autostart status/error paths (fake .app bundles and fake `opencode`/`mdbook`/`code`/`codium`/`wetty` binaries, never touches real apps) |

## 4. Requirements

- Go ≥ 1.26 (`go.mod` declares `go 1.26`)
- At runtime: the managed apps (`Comfy Desktop.app` installed in `/Applications` or `~/Applications`, or pointed to via `NARTHEX_COMFY_APP_DIR`; the `opencode` CLI on PATH, or pointed to via `NARTHEX_OPENCODE_BIN`; the `mdbook` CLI on PATH + a non-empty `mdbook.dirs`, or pointed to via `NARTHEX_MDBOOK_BIN`; the `code` CLI on PATH, or pointed to via `NARTHEX_VSCODE_BIN`; the `codium` CLI on PATH, or pointed to via `NARTHEX_VSCODIUM_BIN`; the `wetty` CLI on PATH, or pointed to via `NARTHEX_WETTY_BIN`, plus an SSH server on `wetty.sshHost`); not needed for build or tests
- macOS (development) or Linux; test scripts need `bash`, `curl`, `python3`

## 5. Reproduction (from scratch to working)

```bash
git clone <repo> && cd Narthex
go mod tidy              # fetches golang.org/x/crypto only
make build               # → build/narthex (single binary, no other files needed)
./build/narthex setup      # interactive password → ~/.config/narthex/config.json
./build/narthex serve      # → http://127.0.0.1:9090
```

Non-interactive: `./build/narthex setup --random --force` (prints the random password); the login username defaults to `narthex` and can be set with `--username`.
Account changes: `./build/narthex passwd` or `reset-auth` (rotates sessionSecret; restart serve afterwards).
serve prints `detected apps: ...` at startup; only detected apps can be added as cards in the UI.

## 6. Verification checklist (run after any change)

1. `gofmt -l .` prints nothing
2. `go vet ./...` clean
3. `go test ./...` passes: auth (hashing/session/rate-limit), secretbox (encryption round-trip/wrong-secret/tamper/random nonce), i18n (completeness across the three languages), procman (desktop-app lifecycle/process-scan fallback/health/memory/.app detection/opencode lifecycle and errors/mdbook detection & project scan & init/vscode+vscodium detection & lifecycle/wetty detection & lifecycle), store (defaults/normalization/atomic round-trip/dedupe by kind), autostart (plist rendering/path resolution/install/uninstall/status via fakes — no real launchctl), api (fake-Backend full-API integration: auth/CRUD/lifecycle/rate-limit/meta incl. apps/language switch/session TTL/account changes/kind validation, duplicate-kind 409, uninstalled-app 400, internal-address URL replacement, mdbook card & projects endpoints, vscode/vscodium token display, wetty card)
4. `make smoke` prints `SMOKE OK` (local full flow (incl. first-run auto-setup without a config) → **passwd session rotation** (old cookie 401, new password logs in) → **account change** (rename username, old session 401, new username logs in, old username 401) → **comfyui card** (fake `Comfy Desktop.app` + `NARTHEX_COMFY_APP_DIR`, create/start/URL 8000/stop/delete, duplicate kind 409) → **opencode card** (fake `opencode` CLI + PATH injection, create/start/healthy/stop/delete, password generated and persisted) → **mdbook projects API + card** (fake `mdbook` CLI + `mdbook.dirs`, list/create a book, card start/healthy/stop/delete) → **vscode card** (fake `code` CLI, create/start/healthy/stop/delete, token generated and persisted) → **vscodium card** (fake `codium` CLI, create/start/healthy/stop/delete, independent token generated and persisted) → **wetty card** (fake `wetty` CLI, create/start/healthy/stop/delete, no credentials) → **autostart CLI** (status / missing-action / unknown-action / missing-config error paths, no real launchctl load))
5. Frontend changes need manual verification: login page, card CRUD, start/stop, the add modal listing only installed-and-not-yet-added kinds, ≤768px mobile layout

## 7. Conventions (must follow)

- **Dependency rule**: only `golang.org/x/crypto` (official) is allowed. Any new dependency must be justified in this section; prefer stdlib implementations (see the hand-rolled `internal/termios` echo-off input)
- **Comment discipline**: no decorative comments; comment only the "why"
- **State file format**: `Card` field changes in state.json are breaking — new fields must be `omitempty` or have backward-compatible defaults handled in `store.LoadState`; never rename existing JSON fields
- **API changes**: all `/api/*` endpoints (except login/session/logout) must stay behind auth; register new endpoints in the protected mux in `internal/server/server.go` and keep the frontend in sync
- **Concurrency**: every handler that reads/writes `Service.State` must hold `s.mu` first; process operations (Start/Stop) run under the lock to prevent concurrent start/stop of the same card
- **Single exit for process operations**: all start/stop/status calls must go through the `procman.Backend` interface; never call `os/exec` or the `procman` internals directly from the api/server layers
- **Account changes must rotate**: `passwd`/`reset-auth`/`/api/account` must rotate `sessionSecret` along with `passwordHash` and **re-encrypt** `usernameEnc` (the two are coupled, see `secretbox`); changing only the hash is a defect
- **Username is ciphertext-only**: never store the username in plaintext in the config; always `secretbox.Encrypt` for new/changed accounts and read via `resolveUsername`/`secretbox.Decrypt`; the username lives decrypted only in memory (`api.Service.Username`)
- **Settings semantics**: `POST /api/settings` applies fields one by one — empty `language`/`pageBackground` means "not set" (no change), `compact` is a pointer to distinguish explicit false; `slogan` is a `*string` (`nil` = no change, `""` = hide, non-empty custom ≤80 chars); `internalAddress` is a `*string` (`nil` = no change, `""` = clear, custom ≤253 chars); a nil/unset `slogan` resolves to `i18n.T(lang, "slogan.default")` per the current language, and a customized slogan no longer follows language switches (see `api.sloganValue`)
- **Internal network address**: when `Config.InternalAddress` is set, card "Open" URLs (`api.instanceURL`) use it instead of the request host; the address may include a port (`net.SplitHostPort` decides: with a port it is used verbatim, otherwise the card port is appended); the frontend Settings input shows the current access host as its placeholder (`window.location.host`) and saves on a debounced input (see `web/app.js`)
- **i18n rule**: every user-visible string (CLI, API errors, frontend) must go through `i18n.T`/`t()`; hardcoded strings are a defect. The `internal/i18n` completeness test fails on any missing translation (all three languages must be present)
- **Upload safety**: `/api/uploads` accepts only jpg/png/webp, ≤15 MB, random hex filenames; the `/uploads/` static handler must reject path traversal and unknown extensions (see `server.uploadsHandler` and `api.UploadExt`)
- **Adding a language**: fill every key in the three `internal/i18n` maps → fill the three dicts in `web/i18n.js` → add the language to the `i18n.Parse` whitelist → add an option to the language dropdown in `index.html` → update both docs
- **Kind semantics**: cards are identified by `Kind` (`comfyui`/`opencode`/`mdbook`/`vscode`/`vscodium`/`wetty`), one card per kind at most; creation requires the app to be installed (400 `err.appNotFound`) and the kind to be new (409 `err.duplicateKind`); cards **store no path** — the install directory is resolved at start time by the detection functions (**exception**: the mdBook card stores the chosen book project in `Card.Dir`, validated at creation against the projects detected under `mdbook.dirs`)
- **ComfyUI server launch**: go through `procman.startComfyUI` (spawn `<python> main.py --port <n> --listen <host>`, cwd = install code dir, Setpgid); `appInstalled` requires both `DetectComfyUIApp()` and `DetectComfyUIInstalls()` to be non-empty; the listen address and port range come from `Config.ComfyUI` (`0.0.0.0` by default for LAN access)
- **opencode web launch**: go through `procman.startOpencode` (spawn `opencode web --port <n> --hostname <host>`, cwd = user home, Setpgid); `appInstalled` requires `DetectOpencode()` to be non-empty; **`OPENCODE_SERVER_PASSWORD` is mandatory** — the server binds `0.0.0.0`, and without a password it is unsecured on the LAN. The password is generated on first serve, persisted in `Config.Opencode.Password`, and surfaced on the card via `CardView.Password`. The listen address and port range come from `Config.Opencode` (`0.0.0.0` / `[4300, 4499]` by default)
- **mdBook server launch**: go through `procman.startMdbook` (spawn `mdbook serve --port <n> --hostname <host>`, cwd = book project dir, Setpgid); `appInstalled` requires `DetectMdbook()` non-empty **and** a non-empty `Config.Mdbook.Dirs` — while the dirs list is empty the mdBook kind is hidden entirely; the card creation requires a `dir` in `DetectMdbookProjects(Config.Mdbook.Dirs)`; new books are created via `CreateMdbookProject` (`mdbook init --force --ignore none`, non-interactive) under one of the configured dirs only. The listen address and port range come from `Config.Mdbook` (`0.0.0.0` / `[4500, 4699]` by default)
- **VS Code web launch**: go through `procman.startVSCode` (spawn `code serve-web --host <h> --port <n> --connection-token <t> --accept-server-license-terms --disable-telemetry`, cwd = user home, Setpgid, child PATH shadowed with the no-open dir); `appInstalled` requires `DetectVSCode()` to be non-empty; **`--connection-token` is mandatory** — the server binds `0.0.0.0`, and without a token it is unsecured on the LAN. The token is generated on first serve, persisted in `Config.Vscode.ConnectionToken`, and surfaced on the card via `CardView.Password` (no username); the card's "Open" URL embeds it as `?tkn=` via `api.instanceURL`/`api.codeToken`, because the server answers 403 without the token (the browser sets the auth cookie on first load). The listen address and port range come from `Config.Vscode` (`0.0.0.0` / `[4700, 4899]` by default)
- **VSCodium web launch**: go through `procman.startVSCodium` (spawn `codium serve-web --host <h> --port <n> --connection-token <t> --accept-server-license-terms --disable-telemetry`, cwd = user home, Setpgid, child PATH shadowed with the no-open dir); `appInstalled` requires `DetectVSCodium()` to be non-empty; the token is generated on first serve, persisted in `Config.Vscodium.ConnectionToken`, and surfaced on the card via `CardView.Password` (no username); its "Open" URL carries the token the same way. It is a fully independent kind from VS Code (own config, own token, own port pick), so both can run at the same time. Listen address and port range come from `Config.Vscodium` (`0.0.0.0` / `[4700, 4899]` by default, a free port is picked at start)
- **WeTTY terminal launch**: go through `procman.startWetty` (spawn `wetty --port <n> --host <h> [--ssh-host <h>] [--ssh-port <n>] [--ssh-user <u>]`, cwd = user home, Setpgid); `appInstalled` requires `DetectWetty()` to be non-empty. WeTTY has **no HTTP-layer auth**: the browser authenticates with the SSH account of `Config.Wetty.SSHHost` (default `localhost`), so the SSH target comes from config, never from URL parameters (`--allow-remote-hosts`/`--allow-remote-command` stay off). The card surfaces no credentials. Listen address, port range and SSH target come from `Config.Wetty` (`0.0.0.0` / `[4900, 5099]` by default)
- **Port rules**: comfyui ports are probed from `comfyui.portRange` (default [4100, 4299]); opencode ports from `opencode.portRange` (default [4300, 4499]); mdbook ports from `mdbook.portRange` (default [4500, 4699]); vscode ports from `vscode.portRange` (default [4700, 4899]); vscodium ports from `vscodium.portRange` (default [4700, 4899]); wetty ports from `wetty.portRange` (default [4900, 5099]); never hardcode ports. Health checks always dial `127.0.0.1`

## 8. Common extension points

- **Add a managed app**: add a detection function (`detectAppBundle`) and a launch branch in `procman`, a kind constant in `store`, default name/icon in `api.kindLabel`/`defaultIcon`, and extend the frontend `availableKinds` list plus the `modal.kind<kind>Desc` copy (following the §7 kind semantics)
- **Add assets**: drop files into `web/assets/` and rebuild (embed picks them up automatically); the `/api/meta` endpoint lists icons/backgrounds at runtime
- **Docs**: update the root `README.md` (English) and `docs/README.zh-CN.md` (Chinese) in pairs; `docs/agents/AGENTS.*` likewise. The root `AGENTS.md` is a pointer only

## 9. Known limitations

- Only instances started by Narthex are managed; services started externally on the same port are not in card state
- Memory is the process RSS, not the sum of child processes
- Sessions are stateless signed cookies; a signature stays valid up to 30 days after logout by default (configurable via `sessionTTLHours`; accepted tradeoff — rotate `sessionSecret` if immediate invalidation is needed)
- A password change requires restarting `narthex serve` (sessionSecret/passwordHash are read into memory at startup); account changes from the UI (`/api/account`) take effect immediately without a restart
- The ComfyUI server is started directly via `python main.py` and is independent of the ComfyUI Desktop window; if the user runs the desktop app too, its embedded server (8000–9000 range) is not managed by Narthex