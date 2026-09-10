# Narthex — Lightweight App Launcher

[中文文档](docs/README.zh-CN.md) · [Agent Guide](docs/agents/)

After entering a username and password, launch and stop your local apps (**ComfyUI servers**, **opencode web**, **mdBook docs**, **VS Code/VSCodium web**, **WeTTY terminal**) from the web and jump straight to their web UIs. Each app type has at most one card, identified by its type: live status, memory usage, one-click start/stop, configurable icon and background. The UI is a minimalist glassmorphism design (translucent panels over a background image) and adapts to any screen size.

## Features

- **Super lightweight**: single static binary with the frontend and assets embedded via `go:embed`; ~10 MB idle memory; zero dependencies except `golang.org/x/crypto`
- **Card management**: editable icons (37 Lucide icons) and backgrounds (8 free photos); one card per app type (`comfyui` / `opencode` / `mdbook` / `vscode` / `vscodium` / `wetty`), no paths involved (except the mdBook card, which points at a book project directory)
- **App detection**: the installed apps are detected at startup; **apps that are not installed are not shown** — no add entry, no create/start
  - `comfyui` cards start a **ComfyUI server** (`python main.py --port <n> --listen 0.0.0.0`); the code directory and interpreter are resolved from ComfyUI Desktop's install records (`installations.json`, overridable via `NARTHEX_COMFY_DESKTOP_DIR`). The port is picked from a configurable range (default 4100–4299) and the server listens on `0.0.0.0` for **LAN access** — the "Open" button is `http://127.0.0.1:<port>/` locally and `http://<dashboard host>:<port>/` from other devices
  - `opencode` cards start the **opencode web UI** (`opencode web --port <n> --hostname 0.0.0.0`) — AI coding in the browser. The port is picked from a configurable range (default 4300–4499) and the server listens on `0.0.0.0` for **LAN access**. Because the server binds a non-loopback address, a **random password is generated once and persisted** (`OPENCODE_SERVER_PASSWORD`, username `opencode`); the **username and password are both shown on the card and can be copied by clicking**, and printed on first serve startup. Starting the card does **not open a browser on the host** — use the "Open" button
  - `mdbook` cards serve an **mdBook documentation project** (`mdbook serve --port <n> --hostname 0.0.0.0`); the kind appears only when the **`mdbook` CLI is installed and `mdbook.dirs` is configured**. Picking the kind lists the books found one level deep under each configured directory (a book is a directory containing `book.toml`); a new book can be created there too (`mdbook init`). The chosen project directory is stored on the card
  - `vscode` cards start the **VS Code web server** (`code serve-web --host 0.0.0.0 --port <n>`). The kind appears when the **`code` CLI** is installed. Because the server binds a non-loopback address, a **random connection token is generated once and persisted** (`vscode.connectionToken`); the token is shown on the card and copied by clicking, and the "Open" URL carries it (`?tkn=`) so the browser authenticates on first load. No folder is selected up front — the browser lets you pick a folder from the server's filesystem
  - `vscodium` cards start the **VSCodium web server** (`codium serve-web --host 0.0.0.0 --port <n>`), an independent kind from VS Code so both can run at the same time. The kind appears when the **`codium` CLI** is installed; its token is generated and persisted separately (`vscodium.connectionToken`), and its "Open" URL carries it the same way
  - `wetty` cards start the **WeTTY terminal-over-web server** (`wetty --port <n> --host 0.0.0.0`) — a terminal in the browser. The kind appears when the **`wetty` CLI** is installed. WeTTY has **no HTTP-layer auth**: the browser prompts for the SSH account of `wetty.sshHost` (default `localhost`, so enable Remote Login/SSH on the host). The listen address, port range and SSH target come from `wetty.*`; the card shows no credentials
  - `comfyui` is detected in `/Applications` and `~/Applications` (overridable via `NARTHEX_COMFY_APP_DIR`); `opencode` is detected by the `opencode` CLI on PATH (overridable via `NARTHEX_OPENCODE_BIN`); `mdbook` by the `mdbook` CLI (overridable via `NARTHEX_MDBOOK_BIN`); VS Code by the `code` CLI (overridable via `NARTHEX_VSCODE_BIN`); VSCodium by the `codium` CLI (overridable via `NARTHEX_VSCODIUM_BIN`); WeTTY by the `wetty` CLI (overridable via `NARTHEX_WETTY_BIN`)
- **Instance management**: instance processes are managed by narthex (own process group, whole-tree termination); status dot + health check (TCP) + memory (RSS) + uptime; instances re-associate after a narthex restart
- **Password change**: `narthex passwd` resets the password and rotates the session key, invalidating every session at once
- **Account management**: the username (default `narthex`) and password can be changed on the top-bar "Account" page or via `reset-auth`; the username is stored **encrypted (AES-256-GCM)** in the config, and only the argon2id hash of the password is ever stored
- **Trilingual UI**: en (default) / Simplified Chinese / Traditional Chinese; switch via a startup flag or an in-page dropdown, CLI prompts included
- **Security**: argon2id password hashing, HMAC-signed session cookies (30 days by default, configurable via `sessionTTLHours`), login rate limiting (5/min, reset on success); listens on `0.0.0.0` by default for external access
- **Responsive**: adaptive card grid; single column with larger touch targets on mobile

## Requirements

- Go ≥ 1.26 (build only)
- The apps you manage must be installed locally:
- `comfyui` cards need **ComfyUI Desktop** (`/Applications/Comfy Desktop.app` or a user-level install, overridable via `NARTHEX_COMFY_APP_DIR`) with at least one **installed ComfyUI instance** (`installations.json`; the support dir is overridable via `NARTHEX_COMFY_DESKTOP_DIR`)
- `opencode` cards need the **opencode CLI** on PATH (overridable via `NARTHEX_OPENCODE_BIN`)
- `mdbook` cards need the **mdbook CLI** on PATH (overridable via `NARTHEX_MDBOOK_BIN`) **and** at least one directory configured in `mdbook.dirs`
- `vscode` cards need the **VS Code (`code`) CLI** on PATH (overridable via `NARTHEX_VSCODE_BIN`)
- `vscodium` cards need the **VSCodium (`codium`) CLI** on PATH (overridable via `NARTHEX_VSCODIUM_BIN`)
- `wetty` cards need the **WeTTY CLI** on PATH (overridable via `NARTHEX_WETTY_BIN`) **and** an SSH server on `wetty.sshHost` (default `localhost`) to log into
- macOS or Linux (memory: `ps` on macOS, `/proc/<pid>/status` on Linux)

## Quick start

```bash
make build            # builds build/narthex
./build/narthex serve   # starts the dashboard at http://127.0.0.1:9090
```

**No manual setup needed on first run**: when no config is found, `serve` creates one automatically with a **random password** (printed to the terminal); the username defaults to `narthex`. You can also initialize manually:

```bash
./build/narthex setup   # sets the password interactively, writes ~/.config/narthex/config.json
./build/narthex serve   # starts the dashboard at http://127.0.0.1:9090
```

Open `http://127.0.0.1:9090` in a browser and enter the username (`narthex`) and password.

### setup flags

| Flag | Description |
|---|---|
| `--config <path>` | Config file path (default `~/.config/narthex/config.json`) |
| `--password <pw>` | Set the password non-interactively |
| `--random` | Generate a random password and print it |
| `--force` | Overwrite an existing config |
| `--username <name>` | Login username (default `narthex`), stored encrypted in the config |

### serve flags

| Flag | Description |
|---|---|
| `--config <path>` | Config file path |
| `--port <n>` | Override the configured listen port |
| `--hostname <h>` | Override the listen address (`0.0.0.0` for LAN access) |
| `--lang <lang>` | Language for this run: `en` (default) / `zh-CN` / `zh-TW`, overrides the config value |

At startup the detected apps are printed, e.g. `detected apps: ComfyUI, opencode, MdBook, VS Code, VSCodium, WeTTY`; apps that are not installed never appear in the UI.

> To keep it running: use `narthex autostart install` (macOS launchd) — see [Auto-start](#auto-start). On Linux use systemd or `nohup ./build/narthex serve &`.
> Starting twice: if a narthex is already listening on the port, serve prints "already running" and exits cleanly instead of failing.

### Language switching

- **Startup flag**: `narthex serve --lang zh-TW` (all subcommands accept `--lang`; `setup` writes the chosen language into the config)
- **In-page switcher**: the dropdown on the right of the top bar (English / 简体中文 / 繁體中文) applies instantly and persists to the config, no refresh needed
- Default is **en**; CLI prompts (setup/passwd/serve) are localized the same way

### Changing the account (username / password)

```bash
./build/narthex passwd           # prompts for the new password
./build/narthex passwd --random  # generates and prints a random password
./build/narthex reset-auth --username myname --password newpass  # reset username and password in one go
```

Changing the password also **rotates `sessionSecret`**, so every logged-in session is invalidated immediately; restart `narthex serve` for it to take effect.

The login username and password can also be changed on the dedicated top-bar "Account" page (current password required). Any account change rotates the session key and **re-encrypts** the stored username with the new key.

## Auto-start

`narthex autostart` installs a macOS launchd job so narthex starts automatically — no hand-edited plist required:

```bash
narthex autostart install                # serve at login (user-level LaunchAgent)
narthex autostart status                 # show the install state
narthex autostart uninstall              # remove the job
```

The job points at the current binary (`os.Executable()`), so reinstall after you move or rebuild the binary. Flags: `--config <path>` (default `~/.config/narthex/config.json`) and `--lang`. Logs go to `~/.config/narthex/serve.log`.

The **Settings** modal in the dashboard also has a "Start at login" switch for the user-level serve job (one click, no terminal).

> Linux note: `narthex autostart` returns a "macOS only" message; use systemd or `nohup` instead.

## Usage

1. **Add a card**: click "Add card" → choose the app (**ComfyUI** / **opencode** / **MdBook** / **VS Code** / **VSCodium** / **WeTTY**; only **installed and not-yet-added** apps are listed) → set the name/icon/background → create. No paths, no search — the app bundle is auto-detected at start time (the **MdBook** kind first asks you to pick one of the books found under `mdbook.dirs`, or create a new one there; that project directory is stored on the card)
2. **Start/stop**: click "Start" or "Stop" on a card; starting instances show "Starting…" and become a green status dot with memory usage and uptime once ready
3. **Open**: click "Open" to open the web UI in a new tab (ComfyUI, opencode, mdBook, VS Code, VSCodium and WeTTY all `http://<host>:<port>/`, the VS Code/VSCodium links carry their connection token as `?tkn=` so the browser authenticates automatically); they all listen on `0.0.0.0`, so other devices reach them through the same hostname as the dashboard. If an **internal network address** is configured in Settings, the jump address uses it instead (see [Configuration](#configuration)). The **opencode card shows the web login username and password** (username `opencode`); the **VS Code and VSCodium cards show their connection tokens**; click either to copy it. Browsers remember credentials per site, so you enter them once per browser. The **WeTTY card shows no credentials**: the browser prompts for the SSH account of `wetty.sshHost` (default `localhost`)
4. **Edit**: change the name, icon or background; deleting a card stops the app first
5. **Settings**: the top-bar "Settings" button → language / page background (incl. upload) / slogan (toggle and custom text) / **internal network address** / start-at-login toggle (macOS); all apply instantly
   - **Internal network address**: once set, card "Open" buttons jump to this host (IP or hostname, may include a port) instead of the address you reached Narthex from; leave empty to keep the current access address (the field's placeholder shows the current address)
6. **Account**: the top-bar "Account" button → a dedicated page to change the username and password (current password required); saving invalidates sessions on other devices
7. **Compact mode**: the "Compact mode / Standard mode" button in the top bar switches the card grid; the preference persists to the config
8. **Background import**:
   - **Page background**: the background picker in Settings → pick a preset or upload a local image (jpg/png/webp, up to 15 MB); applies instantly
   - **Card background**: the "+" button at the end of the background picker in the add/edit card modal uploads an image and selects it automatically
   - Uploads are stored in `~/.config/narthex/uploads/` and can be deleted from the pickers

## Configuration

`~/.config/narthex/config.json`:

```json
{
  "hostname": "0.0.0.0",
  "port": 9090,
  "sessionSecret": "<auto-generated>",
  "passwordHash": "argon2id$v=19$m=65536,t=3,p=2$…",
  "usernameEnc": "<AES-256-GCM encrypted username>",
  "language": "en",
  "sessionTTLHours": 720,
  "compactCards": false,
  "pageBackground": "bg-05",
  "slogan": "From nothing, to nothing.",
  "internalAddress": "",
  "comfyui": { "hostname": "0.0.0.0", "portRange": [4100, 4299] },
  "opencode": { "hostname": "0.0.0.0", "portRange": [4300, 4499], "password": "<auto-generated>" },
  "mdbook": { "hostname": "0.0.0.0", "portRange": [4500, 4699], "dirs": ["~/books"] },
  "vscode": { "hostname": "0.0.0.0", "portRange": [4700, 4899], "connectionToken": "<auto-generated>" },
  "vscodium": { "hostname": "0.0.0.0", "portRange": [4700, 4899], "connectionToken": "<auto-generated>" },
  "wetty": { "hostname": "0.0.0.0", "portRange": [4900, 5099], "sshHost": "localhost" }
}
```

- `hostname`: dashboard listen address, `0.0.0.0` by default (external access); set `127.0.0.1` for local-only use
- `passwordHash`: argon2id hash of the password (one-way; the plaintext password is never stored)
- `usernameEnc`: the login username encrypted with AES-256-GCM; the key is derived from `sessionSecret` via HMAC-SHA256 — this deters casual config browsing, not an attacker who holds the whole file; the two must rotate together (handled automatically when changing the password or username)
- `language`: UI and CLI language (`en`/`zh-CN`/`zh-TW`, default `en`); the `--lang` flag wins
- `sessionTTLHours`: session validity in hours (default 720 = 30 days)
- `compactCards`: compact card mode (toggleable from the top bar, persisted)
- `pageBackground`: page background (a preset id like `bg-05` or an uploaded image id)
- `slogan`: top bar slogan; unset shows the localized default (en/zh-CN/zh-TW), `""` hides it, custom text up to 80 characters (editable in the Settings modal)
- `internalAddress`: the internal network address (host IP or hostname, may include a port); when set, card "Open" URLs use it instead of the address you reached Narthex from, so other devices can reach the services over the internal network; leave empty to keep the current access address (editable in the Settings modal)
- `comfyui.hostname`: ComfyUI server listen address, `0.0.0.0` by default (LAN access; set `127.0.0.1` for local-only)
- `comfyui.portRange`: the range ComfyUI server ports are picked from, default [4100, 4299]
- `opencode.hostname`: opencode web listen address, `0.0.0.0` by default (LAN access; set `127.0.0.1` for local-only)
- `opencode.portRange`: the range opencode web ports are picked from, default [4300, 4499]
- `opencode.password`: the opencode web basic-auth password, auto-generated and persisted on first serve; shown on the opencode card. Change it in the config to rotate (restart the instance afterwards)
- `mdbook.hostname`: mdBook server listen address, `0.0.0.0` by default (LAN access; set `127.0.0.1` for local-only)
- `mdbook.portRange`: the range mdBook server ports are picked from, default [4500, 4699]
- `mdbook.dirs`: directories under which mdBook projects (directories containing `book.toml`) are recognized and new books are created; while empty, the mdBook card kind is hidden entirely
- `vscode.hostname`: VS Code web server listen address, `0.0.0.0` by default (LAN access; set `127.0.0.1` for local-only)
- `vscode.portRange`: the range the VS Code web server ports are picked from, default [4700, 4899] (a free port is picked at start)
- `vscode.connectionToken`: the connection token the VS Code web UI asks for in the browser, auto-generated and persisted on first serve; shown on the VS Code card. Change it in the config to rotate (restart the instance afterwards)
- `vscodium.hostname` / `vscodium.portRange` / `vscodium.connectionToken`: the same three knobs for the **VSCodium** card, fully independent from `vscode` so both can run at the same time
- `wetty.hostname`: WeTTY listen address, `0.0.0.0` by default (LAN access; set `127.0.0.1` for local-only)
- `wetty.portRange`: the range WeTTY ports are picked from, default [4900, 5099]
- `wetty.sshHost`: the SSH server WeTTY connects to (`--ssh-host`), default `localhost` (the narthex host). Set it to reach a remote machine
- `wetty.sshPort`: the SSH server port (`--ssh-port`); omit to use WeTTY's default (22)
- `wetty.sshUser`: the default SSH user (`--ssh-user`); omit to be prompted in the browser. WeTTY has **no HTTP-layer auth** — the SSH account is the only protection
- Data files: state `state.json` (cards by app type, one per type), per-instance logs `logs/<cardID>.log` and uploads `uploads/` live next to the config

## External access (on by default)

Narthex listens on **`0.0.0.0`** by default, so the dashboard is reachable from other devices out of the box:

- From this machine: `http://127.0.0.1:9090`
- From the LAN/internet: `http://<machine IP>:9090` (e.g. `http://192.168.1.100:9090`)

**For local-only use, tighten it up**:

```bash
narthex serve --hostname 127.0.0.1              # for this run
# or config.json → "hostname": "127.0.0.1"      # persistent
```

**Security warnings** (also printed at startup):

- Over plain HTTP the password travels **unencrypted**; for public exposure, put an HTTPS reverse proxy (Caddy/Nginx) in front of port 9090
- The managed apps are launched locally for local use
- macOS may prompt "allow incoming connections" on the first LAN access — allow `narthex`; on Linux open the port in the firewall
- Once exposed, the login rate limit (5/min) and a strong password are the last line of defense

## API summary

| Method | Path | Description |
|---|---|---|
| POST | `/api/login` | Username + password login, sets session cookie |
| POST | `/api/logout` | Log out |
| GET | `/api/session` | Whether the session is valid, plus the UI language (the login page renders accordingly) |
| POST | `/api/account` | Change username/password (current password required), rotates the session key |
| GET | `/api/cards` | Card list (with running/healthy/memoryKB/uptime/url) |
| POST | `/api/cards` | Create a card (`kind`: `comfyui`/`opencode`/`mdbook`/`vscode`/`vscodium`/`wetty`; 400 when the app is not installed, 409 on duplicate kind; `mdbook` also requires `dir`, a recognized project) |
| PATCH | `/api/cards/{id}` | Edit name/icon/background |
| DELETE | `/api/cards/{id}` | Delete (stops the app first) |
| POST | `/api/cards/{id}/start` | Start the app (bundle auto-detected at start time) |
| POST | `/api/cards/{id}/stop` | Stop the app |
| GET | `/api/meta` | Icon/background presets (incl. uploads), detected apps (`apps` with installed/label) and current settings (incl. `internalAddress`) |
| GET | `/api/mdbook/projects` | List the configured `mdbook.dirs` and the recognized book projects under them (`{dirs, projects:[{name,path}]}`) |
| POST | `/api/mdbook/projects` | Create a new book in a configured directory (`{dir, name}` → runs `mdbook init`) |
| POST | `/api/settings` | Update settings: `language` / `compact` / `pageBackground` / `slogan` / `internalAddress`, persisted to the config |
| GET | `/api/autostart` | Report the macOS launchd auto-start state for `narthex serve` (`{supported, installed, loaded, label, plistPath}`) |
| POST | `/api/autostart` | Install (`{action:"install"}`) or uninstall (`{action:"uninstall"}`) the user-level serve job |
| POST | `/api/uploads` | Upload a background image (multipart `file`, jpg/png/webp ≤15 MB) |
| DELETE | `/api/uploads/{id}` | Delete an uploaded image |
| GET | `/uploads/<file>` | Serve uploaded images (static) |

All endpoints except login/session/logout require the session cookie; `/uploads/` is public static content (random filenames).

## FAQ

- **`http://0.0.0.0:9090` opens a blank page?** `0.0.0.0` is the server-side listen address, not a visitable URL; Firefox and some other browsers refuse it. Use `http://127.0.0.1:9090` (this machine) or `http://<LAN IP>:9090` (other devices, e.g. `http://192.168.0.102:9090`)
- **Blank page from another device?** Most likely the macOS firewall is blocking: System Settings → Network → Firewall, allow incoming connections for `narthex`; on Linux open the port
- **An app type is missing from the add dialog?** Only **installed** apps are shown. Make sure the desktop app (`Comfy Desktop.app`) is installed in `/Applications` or `~/Applications` (or point `NARTHEX_COMFY_APP_DIR` at it); for `opencode`, make sure the `opencode` CLI is on PATH (or point `NARTHEX_OPENCODE_BIN` at it); for `mdbook`, the `mdbook` CLI must be on PATH (or `NARTHEX_MDBOOK_BIN`) **and** `mdbook.dirs` must be non-empty in the config; for VS Code, make sure `code` is on PATH (or point `NARTHEX_VSCODE_BIN` at it); for VSCodium, make sure `codium` is on PATH (or point `NARTHEX_VSCODIUM_BIN` at it); each type has **one card**, and types already added are not listed again
- **ComfyUI card won't start?** **ComfyUI Desktop** must be installed with at least one installed instance (code dir containing `main.py`, resolved from `installations.json`; overridable via `NARTHEX_COMFY_DESKTOP_DIR`). Narthex starts a ComfyUI **server** (`python main.py --port <n> --listen 0.0.0.0`); the port is picked from `comfyui.portRange` (default 4100–4299). "Open" is `http://127.0.0.1:<port>/` locally and `http://<dashboard host>:<port>/` from other devices
- **opencode card won't start?** The **opencode CLI** must be installed (`which opencode`, or set `NARTHEX_OPENCODE_BIN`). Narthex runs `opencode web --port <n> --hostname 0.0.0.0` in your home directory; the port is picked from `opencode.portRange` (default 4300–4499). **Enter the password shown on the card in the browser** (username `opencode`); browsers remember basic-auth credentials per site, so each browser/device asks only once
- **mdBook card won't start?** The **mdbook CLI** must be installed (`which mdbook`, or set `NARTHEX_MDBOOK_BIN`) and at least one directory configured in `mdbook.dirs`. The card points at a book project (a directory containing `book.toml`) picked or created in the add-card flow. Narthex runs `mdbook serve --port <n> --hostname 0.0.0.0` in the project directory; the port is picked from `mdbook.portRange` (default 4500–4699). If the book fails to build, the card stays "Starting…" — check `logs/<cardID>.log`
- **VS Code card won't start?** The **VS Code (`code`) CLI** must be installed (or set `NARTHEX_VSCODE_BIN`). Narthex runs `code serve-web --host 0.0.0.0 --port <n> --connection-token <token>` in your home directory; the port is picked from `vscode.portRange` (default 4700–4899). **Enter the connection token shown on the card** in the browser to connect
- **VSCodium card won't start?** Same as VS Code, but with the **`codium` CLI** (or `NARTHEX_VSCODIUM_BIN`): Narthex runs `codium serve-web --host 0.0.0.0 --port <n> --connection-token <token>`; the port is picked from `vscodium.portRange` (default 4700–4899). It is an independent kind, so it can run alongside a VS Code card
- **WeTTY card won't start?** The **`wetty` CLI** must be installed (`npm -g i wetty`, or set `NARTHEX_WETTY_BIN`). Narthex runs `wetty --port <n> --host 0.0.0.0 --ssh-host <host>` in your home directory; the port is picked from `wetty.portRange` (default 4900–5099). The browser then asks for the **SSH username/password** of `wetty.sshHost` (default `localhost`), so the host must have an SSH server running (macOS: System Settings → General → Sharing → Remote Login). WeTTY itself has no HTTP login, so anyone who can reach the port sees the SSH prompt — set `wetty.hostname` to `127.0.0.1` to keep it local

## Security notes

- Listens on `0.0.0.0` by default; a security warning is printed at startup — for local-only use switch back to `127.0.0.1` (see above)
- Passwords travel in plaintext over HTTP; put an HTTPS reverse proxy (Caddy/Nginx) in front of the dashboard for public access
- The username is stored encrypted and the password only as an argon2id hash — a leaked config file does not directly expose either in plaintext; still, anyone holding the full config can decrypt the username, so protect `~/.config/narthex` like you would the password itself
- Forgot the password? Run `./build/narthex setup --force`, or `./build/narthex reset-auth` (can also reset the username); for a regular change use `./build/narthex passwd` (rotates the session key)

## Testing

```bash
go test ./...            # unit tests
make smoke               # end-to-end smoke test (uses fake .app bundles, never touches real apps)
```

## Asset sources

- Icons: [Lucide](https://lucide.dev), ISC license
- Backgrounds: [Lorem Picsum](https://picsum.photos) (photos from Unsplash, free to use)
- Assets are committed to the repository, so builds work offline; refresh with `scripts/download-assets.sh`

## Layout

```
cmd/narthex/       entrypoint (serve / setup / passwd / reset-auth / autostart subcommands)
internal/api/      HTTP handlers and card business logic
internal/auth/     argon2id passwords, HMAC sessions, rate limiting
internal/autostart/  macOS launchd install/uninstall/status (serve)
internal/i18n/     CLI/API message catalogs (en / zh-CN / zh-TW)
internal/procman/  Backend abstraction, local instance lifecycle (ComfyUI server, opencode/mdbook/VS Code/VSCodium/WeTTY web), install & .app detection
internal/secretbox/  username encryption at rest (AES-256-GCM, key derived from sessionSecret)
internal/server/   routing, auth middleware, static files
internal/store/    config and state persistence
internal/termios/  echo-off password input
web/               embedded frontend (HTML/CSS/JS/i18n.js + icons/backgrounds)
scripts/           asset download, smoke test
docs/              user and agent documentation
```
