#!/usr/bin/env bash
# Smoke test: build, setup, serve, then exercise the whole API.
#
# Fake .app bundles (Comfy Desktop.app) are used instead of the real
# apps so the test never touches your real installs.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d /tmp/narthex-smoke.XXXXXX)"
PORT=8877
USER="narthex"
NEWUSER="demo-user"
PASS="smoke-pass-42"
PASS2="smoke-pass-new"
BIN="$TMP/narthex"
CFG="$TMP/config.json"
COOKIE="$TMP/cookies.txt"
SRV_PID=""

cleanup() {
  [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null || true
  rm -rf "$TMP"
}
trap cleanup EXIT

# Fake Comfy Desktop.app bundle (used only for the install check).
COMFYAPP="$TMP/fake-apps/Comfy Desktop.app"
mkdir -p "$COMFYAPP/Contents/MacOS"
cat > "$COMFYAPP/Contents/MacOS/Comfy Desktop" <<'SHIM'
#!/bin/sh
sleep 300
SHIM
chmod +x "$COMFYAPP/Contents/MacOS/Comfy Desktop"

# Fake ComfyUI Desktop support dir with a managed install: the code dir
# contains a main.py shim that serves HTTP on the --port given.
COMFYDESK="$TMP/fake-comfy-desktop"
COMFYDIR="$TMP/fake-comfy-install/ComfyUI"
mkdir -p "$COMFYDIR" "$COMFYDESK"
cat > "$COMFYDIR/main.py" <<'PY'
#!/usr/bin/env python3
import http.server, sys
port = 8188
prev = None
for a in sys.argv[1:]:
    if prev == "--port":
        port = int(a)
    prev = a
http.server.ThreadingHTTPServer(("127.0.0.1", port), http.server.SimpleHTTPRequestHandler).serve_forever()
PY
python3 - "$COMFYDIR" "$COMFYDESK" <<'PY'
import json, sys
install = sys.argv[1]
data = [{"installPath": install.rsplit("/ComfyUI", 1)[0], "status": "installed"}]
open(sys.argv[2] + "/installations.json", "w").write(json.dumps(data))
PY

# Fake `opencode` CLI: mimics `opencode web` by serving HTTP on the port
# given via `--port`. When OC_ENV_FILE is set it also dumps the basic-auth
# credentials and PATH so the test can assert the child env. Shadowed on
# PATH so the real CLI is never touched.
mkdir -p "$TMP/bin"
cat > "$TMP/bin/opencode" <<'SHIM'
#!/bin/sh
if [ -n "$OC_ENV_FILE" ]; then
  printf '%s|%s|%s\n' "$OPENCODE_SERVER_USERNAME" "$OPENCODE_SERVER_PASSWORD" "$PATH" > "$OC_ENV_FILE"
fi
port=4300
prev=
for a in "$@"; do
  if [ "$prev" = "--port" ]; then port=$a; fi
  prev=$a
done
exec python3 -m http.server "$port" --bind 127.0.0.1
SHIM
chmod +x "$TMP/bin/opencode"

# Fake `mdbook` CLI: `serve` serves HTTP on the port given via `--port`;
# `init <name>` writes a minimal book skeleton (book.toml + src).
cat > "$TMP/bin/mdbook" <<'SHIM'
#!/bin/sh
if [ "$1" = "serve" ]; then
  port=4500
  prev=
  for a in "$@"; do
    if [ "$prev" = "--port" ]; then port=$a; fi
    prev=$a
  done
  exec python3 -m http.server "$port" --bind 127.0.0.1
fi
if [ "$1" = "init" ]; then
  name="$2"
  mkdir -p "$name/src"
  printf '[book]\ntitle = "x"\n' > "$name/book.toml"
  printf '# Summary\n\n- [Chapter 1](./chapter_1.md)\n' > "$name/src/SUMMARY.md"
  printf '# Chapter 1\n' > "$name/src/chapter_1.md"
  exit 0
fi
exit 0
SHIM
chmod +x "$TMP/bin/mdbook"

# Fake `code` CLI: mimics `code serve-web` by serving HTTP on the port
# given via `--port`.
cat > "$TMP/bin/code" <<'SHIM'
#!/bin/sh
if [ "$1" = "serve-web" ]; then
  port=4700
  prev=
  for a in "$@"; do
    if [ "$prev" = "--port" ]; then port=$a; fi
    prev=$a
  done
  exec python3 -m http.server "$port" --bind 127.0.0.1
fi
exit 0
SHIM
chmod +x "$TMP/bin/code"

# Fake `codium` CLI: same shape as the fake `code`, for the vscodium kind.
cat > "$TMP/bin/codium" <<'SHIM'
#!/bin/sh
if [ "$1" = "serve-web" ]; then
  port=4700
  prev=
  for a in "$@"; do
    if [ "$prev" = "--port" ]; then port=$a; fi
    prev=$a
  done
  exec python3 -m http.server "$port" --bind 127.0.0.1
fi
exit 0
SHIM
chmod +x "$TMP/bin/codium"

# Fake `wetty` CLI: serves HTTP on the port given via `--port`.
cat > "$TMP/bin/wetty" <<'SHIM'
#!/bin/sh
port=4900
prev=
for a in "$@"; do
  if [ "$prev" = "--port" ]; then port=$a; fi
  prev=$a
done
exec python3 -m http.server "$port" --bind 127.0.0.1
SHIM
chmod +x "$TMP/bin/wetty"

# A recognized mdBook project plus a configured project directory.
MDBOOK_BOOKS="$TMP/mdbook-books"
mkdir -p "$MDBOOK_BOOKS/guide/src"
cat > "$MDBOOK_BOOKS/guide/book.toml" <<'EOF'
[book]
title = "Guide"
EOF
printf '# Summary\n\n- [Chapter 1](./chapter_1.md)\n' > "$MDBOOK_BOOKS/guide/src/SUMMARY.md"
printf '# Chapter 1\n' > "$MDBOOK_BOOKS/guide/src/chapter_1.md"

fail() { echo "FAIL: $1" >&2; exit 1; }

echo "== build =="
(cd "$ROOT" && go build -o "$BIN" ./cmd/narthex)

echo "== setup =="
"$BIN" setup --config "$CFG" --password "$PASS" --force >/dev/null
"$BIN" setup --config "$CFG" --password "$PASS" >/dev/null 2>&1 && fail "setup without --force should fail"
grep -q '"usernameEnc"' "$CFG" || fail "config should store an encrypted username"
grep -q "\"$USER\"" "$CFG" && fail "username must not be stored in plaintext"
# Configure the mdBook project directories so the mdbook card kind shows,
# and point the vscodium kind at a Machine settings seed file.
VSCODIUM_MACHINE="$TMP/vscodium-machine.json"
cat > "$VSCODIUM_MACHINE" <<'EOF'
{ "editor.fontSize": 16, "files.autoSave": "afterDelay" }
EOF
python3 - "$CFG" "$MDBOOK_BOOKS" "$VSCODIUM_MACHINE" <<'PY'
import json, sys
cfg, books, machine = sys.argv[1], sys.argv[2], sys.argv[3]
c = json.load(open(cfg))
c.setdefault("mdbook", {})["dirs"] = [books]
c.setdefault("vscodium", {})["machineSettingsFile"] = machine
json.dump(c, open(cfg, "w"), indent=2)
PY
# A pre-existing server-side Machine setting that the seed must preserve.
mkdir -p "$TMP/vscodium-data/data/Machine"
printf '{ "editor.fontSize": 99 }\n' > "$TMP/vscodium-data/data/Machine/settings.json"

echo "== first-run auto-setup (serve without config) =="
AUTO_CFG="$TMP/auto-config.json"
"$BIN" serve --config "$AUTO_CFG" --port "$PORT" >"$TMP/auto-server.log" 2>&1 &
AUTO_PID=$!
for _ in $(seq 1 40); do
  curl -fsS "http://127.0.0.1:$PORT/api/session" >/dev/null 2>&1 && break
  sleep 0.25
done
curl -fsS "http://127.0.0.1:$PORT/api/session" >/dev/null || fail "auto-setup server did not start"
grep -qi "created one with a random password" "$TMP/auto-server.log" || fail "auto-setup message missing: $(cat "$TMP/auto-server.log")"
AUTOPASS=$(grep -o 'generated password (keep it safe): [^ ]*' "$TMP/auto-server.log" | awk '{print $NF}')
[ -n "$AUTOPASS" ] || fail "no generated password in auto-setup log"
[ -s "$AUTO_CFG" ] || fail "auto-setup did not write a config"
curl -fsS -c "$TMP/auto-cookies.txt" -H 'Content-Type: application/json' \
  -d "{\"username\":\"narthex\",\"password\":\"$AUTOPASS\"}" "http://127.0.0.1:$PORT/api/login" \
  | python3 -c 'import sys,json;assert json.load(sys.stdin)["ok"] is True' || fail "login with auto-generated password failed"
kill "$AUTO_PID" 2>/dev/null || true
wait "$AUTO_PID" 2>/dev/null || true

echo "== serve =="
OC_ENV_FILE="$TMP/opencode-env.txt" PATH="$TMP/bin:$PATH" NARTHEX_COMFY_APP_DIR="$COMFYAPP" NARTHEX_COMFY_DESKTOP_DIR="$COMFYDESK" NARTHEX_MDBOOK_BIN="$TMP/bin/mdbook" NARTHEX_VSCODE_BIN="$TMP/bin/code" NARTHEX_VSCODIUM_BIN="$TMP/bin/codium" NARTHEX_WETTY_BIN="$TMP/bin/wetty" "$BIN" serve --config "$CFG" --port "$PORT" >"$TMP/server.log" 2>&1 &
SRV_PID=$!
for _ in $(seq 1 40); do
  curl -fsS "http://127.0.0.1:$PORT/api/session" >/dev/null 2>&1 && break
  sleep 0.25
done
curl -fsS "http://127.0.0.1:$PORT/api/session" >/dev/null || fail "server did not start"
grep -qi "detected apps" "$TMP/server.log" || fail "serve should print detected apps: $(cat "$TMP/server.log")"

echo "== unauthenticated is rejected =="
code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/api/cards")
[ "$code" = "401" ] || fail "expected 401, got $code"
# Restart must require a session; an unauthenticated call must never trigger it.
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:$PORT/api/restart")
[ "$code" = "401" ] || fail "expected 401 for unauthenticated restart, got $code"

echo "== login =="
curl -fsS -D "$TMP/login-headers.txt" -o /dev/null -c "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" "http://127.0.0.1:$PORT/api/login" || fail "login failed"
grep -qi "max-age=2592000" "$TMP/login-headers.txt" || fail "session cookie should be 30 days (Max-Age=2592000)"
AUTHED=$(curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/session" | python3 -c 'import sys,json;print(json.load(sys.stdin)["authed"])')
[ "$AUTHED" = "True" ] || fail "session not authed"
curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/meta" | python3 -c 'import sys,json;assert json.load(sys.stdin)["username"]=="'"$USER"'","meta should expose the username"'

echo "== detected apps in meta =="
curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/meta" | python3 -c "
import sys, json
apps = json.load(sys.stdin)['apps']
assert apps['comfyui']['installed'] is True, apps
assert apps['comfyui']['label'] == 'ComfyUI', apps
assert apps['opencode']['installed'] is True, apps
assert apps['opencode']['label'] == 'OpenCode', apps
assert apps['mdbook']['installed'] is True, apps
assert apps['mdbook']['label'] == 'MdBook', apps
assert apps['vscode']['installed'] is True, apps
assert apps['vscode']['label'] == 'VS Code', apps
assert apps['vscodium']['installed'] is True, apps
assert apps['vscodium']['label'] == 'VSCodium', apps
assert apps['wetty']['installed'] is True, apps
assert apps['wetty']['label'] == 'WeTTY', apps"

echo "== language switch =="
curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/meta" | python3 -c 'import sys,json;assert json.load(sys.stdin)["lang"]=="en","default lang should be en"'
curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' -d '{"language":"zh-TW"}' "http://127.0.0.1:$PORT/api/settings" >/dev/null || fail "settings switch failed"
curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/meta" | python3 -c 'import sys,json;assert json.load(sys.stdin)["lang"]=="zh-TW","lang should be zh-TW"'
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"language":"fr"}' "http://127.0.0.1:$PORT/api/settings")
[ "$code" = "400" ] || fail "invalid language should be 400, got $code"
curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' -d '{"language":"en"}' "http://127.0.0.1:$PORT/api/settings" >/dev/null

echo "== upload + page background =="
python3 -c "import base64;open('$TMP/t.png','wb').write(base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAACklEQVR4nGMAAQAABQABDQottAAAAABJRU5ErkJggg=='))"
UPRESP=$(curl -fsS -b "$COOKIE" -F "file=@$TMP/t.png" "http://127.0.0.1:$PORT/api/uploads")
UPID=$(echo "$UPRESP" | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
UPURL=$(echo "$UPRESP" | python3 -c 'import sys,json;print(json.load(sys.stdin)["url"])')
[ -n "$UPID" ] || fail "upload failed: $UPRESP"
curl -fsS -o /dev/null "http://127.0.0.1:$PORT$UPURL" || fail "uploaded image not served"
curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"pageBackground\":\"$UPID\"}" "http://127.0.0.1:$PORT/api/settings" >/dev/null || fail "set pageBackground failed"
curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/meta" | python3 -c "import sys,json;assert json.load(sys.stdin)['pageBackground']=='$UPID'"
curl -fsS -b "$COOKIE" -X DELETE "http://127.0.0.1:$PORT/api/uploads/$UPID" >/dev/null || fail "delete upload failed"
curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"pageBackground":"bg-01"}' "http://127.0.0.1:$PORT/api/settings" >/dev/null

echo "== slogan =="
curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/meta" | python3 -c 'import sys,json;assert json.load(sys.stdin)["slogan"]=="From nothing, to nothing.","default slogan (en)"'
curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"slogan":"hello"}' "http://127.0.0.1:$PORT/api/settings" >/dev/null || fail "set slogan failed"
curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/meta" | python3 -c 'import sys,json;assert json.load(sys.stdin)["slogan"]=="hello"'
curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"slogan":""}' "http://127.0.0.1:$PORT/api/settings" >/dev/null || fail "clear slogan failed"
curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/meta" | python3 -c 'import sys,json;assert json.load(sys.stdin)["slogan"]==""'

echo "== wrong credentials rejected =="
code=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"wrong\"}" "http://127.0.0.1:$PORT/api/login")
[ "$code" = "401" ] || fail "expected 401 for wrong password, got $code"
code=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"username":"nobody","password":"'"$PASS"'"}' "http://127.0.0.1:$PORT/api/login")
[ "$code" = "401" ] || fail "expected 401 for wrong username, got $code"

echo "== successful login resets quota =="
for _ in 1 2 3; do
  curl -s -o /dev/null -H 'Content-Type: application/json' \
    -d "{\"username\":\"$USER\",\"password\":\"wrong\"}" "http://127.0.0.1:$PORT/api/login"
done
code=$(curl -s -o /dev/null -w '%{http_code}' -c "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" "http://127.0.0.1:$PORT/api/login")
[ "$code" = "200" ] || fail "correct password on 5th attempt should succeed, got $code"

echo "== comfyui card =="
CARD=$(curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"kind\":\"comfyui\",\"name\":\"comfy\",\"icon\":\"rocket\",\"background\":\"bg-02\"}" \
  "http://127.0.0.1:$PORT/api/cards")
ID=$(python3 -c "import sys,json;print(json.loads(sys.argv[1])['id'])" "$CARD")
[ -n "$ID" ] || fail "no card id in response"
echo "$CARD" | python3 -c 'import sys,json;c=json.load(sys.stdin);assert c["kind"]=="comfyui" and c["icon"]=="rocket" and c["name"]=="comfy",c'

echo "== duplicate kind rejected =="
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"kind":"comfyui"}' "http://127.0.0.1:$PORT/api/cards")
[ "$code" = "409" ] || fail "duplicate kind should be 409, got $code"

echo "== unknown kind rejected =="
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"kind":"wechat"}' "http://127.0.0.1:$PORT/api/cards")
[ "$code" = "400" ] || fail "unknown kind should be 400, got $code"

echo "== start comfyui =="
curl -fsS -b "$COOKIE" -X POST "http://127.0.0.1:$PORT/api/cards/$ID/start" >/dev/null || fail "start request failed"
RUN="no"
for _ in $(seq 1 40); do
  STATUS=$(curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/cards")
  RUN=$(echo "$STATUS" | python3 -c "import sys,json;c=[x for x in json.load(sys.stdin)['cards'] if x['id']=='$ID'][0];print('yes' if c['running'] and c['healthy'] else 'no')")
  [ "$RUN" = "yes" ] && break
  sleep 0.5
done
[ "$RUN" = "yes" ] || { echo "$STATUS"; fail "comfyui app did not become running"; }
echo "$STATUS" | python3 -c "import sys,json;c=[x for x in json.load(sys.stdin)['cards'] if x['id']=='$ID'][0];assert c['url'].startswith('http://127.0.0.1:') and c['url'].endswith('/'),c"

echo "== stop comfyui =="
curl -fsS -b "$COOKIE" -X POST "http://127.0.0.1:$PORT/api/cards/$ID/stop" >/dev/null || fail "stop request failed"
sleep 1
curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/cards" | python3 -c '
import sys, json
c = [x for x in json.load(sys.stdin)["cards"] if x["id"] == sys.argv[1]][0]
assert not c["running"], c' "$ID"
curl -fsS -b "$COOKIE" -X DELETE "http://127.0.0.1:$PORT/api/cards/$ID" >/dev/null || fail "comfyui delete failed"

echo "== opencode card =="
# The default card name is capitalized: OpenCode.
CARD=$(curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"kind":"opencode"}' \
  "http://127.0.0.1:$PORT/api/cards")
ID=$(python3 -c "import sys,json;print(json.loads(sys.argv[1])['id'])" "$CARD")
[ -n "$ID" ] || fail "no opencode card id in response"
echo "$CARD" | python3 -c 'import sys,json;c=json.load(sys.stdin);assert c["kind"]=="opencode" and c["icon"]=="terminal" and c["name"]=="OpenCode" and c["username"]=="opencode" and c["password"],c'
OCOPENCODE_PW=$(echo "$CARD" | python3 -c 'import sys,json;print(json.load(sys.stdin)["password"])')
# The password is persisted in the config so restarts keep the same one.
grep -q "\"password\": \"$OCOPENCODE_PW\"" "$CFG" || fail "opencode password not persisted in config"
curl -fsS -b "$COOKIE" -X POST "http://127.0.0.1:$PORT/api/cards/$ID/start" >/dev/null || fail "opencode start failed"
RUN="no"
for _ in $(seq 1 40); do
  STATUS=$(curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/cards")
  RUN=$(echo "$STATUS" | python3 -c "import sys,json;c=[x for x in json.load(sys.stdin)['cards'] if x['id']=='$ID'][0];print('yes' if c['running'] and c['healthy'] else 'no')")
  [ "$RUN" = "yes" ] && break
  sleep 0.5
done
[ "$RUN" = "yes" ] || { echo "$STATUS"; fail "opencode app did not become running"; }
# The spawned opencode gets the credentials via env and a PATH that
# shadows the browser opener (no browser tab is opened on the host).
for _ in $(seq 1 20); do [ -s "$TMP/opencode-env.txt" ] && break; sleep 0.25; done
[ -s "$TMP/opencode-env.txt" ] || fail "opencode env dump missing: $(cat "$TMP/opencode-env.txt" 2>/dev/null)"
OC_ENV=$(cat "$TMP/opencode-env.txt")
echo "$OC_ENV" | grep -q "^opencode|$OCOPENCODE_PW|" || fail "opencode child env wrong: $OC_ENV"
echo "$OC_ENV" | grep -q "no-open" || fail "opencode child PATH should shadow the browser opener: $OC_ENV"
echo "$STATUS" | python3 -c "import sys,json;c=[x for x in json.load(sys.stdin)['cards'] if x['id']=='$ID'][0];assert c['password'] and c['username']=='opencode',c"
# The "Open" URL points at the narthex gateway (no embedded credentials);
# the direct endpoint is exposed separately for remote API clients.
GW_URL=$(echo "$STATUS" | python3 -c "import sys,json;c=[x for x in json.load(sys.stdin)['cards'] if x['id']=='$ID'][0];print(c['url'])")
API_URL=$(echo "$STATUS" | python3 -c "import sys,json;c=[x for x in json.load(sys.stdin)['cards'] if x['id']=='$ID'][0];print(c.get('apiUrl',''))")
echo "$GW_URL" | python3 -c "import sys,re;assert re.match(r'^http://127\.0\.0\.1:\d+/$', sys.stdin.read().strip()), '$GW_URL'"
echo "$API_URL" | python3 -c "import sys,re;assert re.match(r'^http://127\.0\.0\.1:\d+/$', sys.stdin.read().strip()), '$API_URL'"
# The gateway requires the narthex session and proxies when authenticated;
# the direct API endpoint answers on its own.
code=$(curl -s -o /dev/null -w '%{http_code}' "$GW_URL")
[ "$code" = "302" ] || fail "unauthenticated gateway should redirect, got $code"
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE" "$GW_URL")
[ "$code" = "200" ] || fail "authenticated gateway should proxy, got $code"
code=$(curl -s -o /dev/null -w '%{http_code}' "$API_URL")
[ "$code" = "200" ] || fail "direct opencode API endpoint should answer, got $code"
curl -fsS -b "$COOKIE" -X POST "http://127.0.0.1:$PORT/api/cards/$ID/stop" >/dev/null || fail "opencode stop failed"
# Stopping opencode keeps the gateway listener bound (whole serve lifetime)
# so a stale tab gets a login redirect or a stopped page instead of a
# connection-refused blank page.
GW_PORT=$(echo "$GW_URL" | python3 -c "import sys,re;m=re.search(r':(\d+)/$',sys.stdin.read().strip());print(m.group(1))")
code=$(curl -s -o /dev/null -w '%{http_code}' "$GW_URL" || true)
[ "$code" = "302" ] || fail "gateway after stop should redirect unauthenticated, got $code"
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE" "$GW_URL")
[ "$code" = "503" ] || fail "gateway after stop should serve a stopped page, got $code"
body=$(curl -s -b "$COOKIE" "$GW_URL")
echo "$body" | grep -q "http://127.0.0.1:$PORT/" || fail "stopped page missing dashboard link: $body"
curl -fsS -b "$COOKIE" -X DELETE "http://127.0.0.1:$PORT/api/cards/$ID" >/dev/null || fail "opencode delete failed"

echo "== mdbook projects API =="
curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/mdbook/projects" | python3 -c "
import sys, json
r = json.load(sys.stdin)
assert r['dirs'] == ['$MDBOOK_BOOKS'], r
assert any(p['name'] == 'guide' and p['path'] == '$MDBOOK_BOOKS/guide' for p in r['projects']), r"
NEW=$(curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"dir\":\"$MDBOOK_BOOKS\",\"name\":\"newbook\"}" \
  "http://127.0.0.1:$PORT/api/mdbook/projects")
echo "$NEW" | python3 -c "import sys,json;r=json.load(sys.stdin);assert r['name']=='newbook' and r['path']=='$MDBOOK_BOOKS/newbook',r"
[ -f "$MDBOOK_BOOKS/newbook/book.toml" ] || fail "mdbook init did not create the book skeleton"
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"dir\":\"$MDBOOK_BOOKS\",\"name\":\"../evil\"}" "http://127.0.0.1:$PORT/api/mdbook/projects")
[ "$code" = "400" ] || fail "invalid book name should be 400, got $code"

echo "== mdbook card =="
CARD=$(curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"kind\":\"mdbook\",\"name\":\"docs\",\"dir\":\"$MDBOOK_BOOKS/guide\"}" \
  "http://127.0.0.1:$PORT/api/cards")
ID=$(python3 -c "import sys,json;print(json.loads(sys.argv[1])['id'])" "$CARD")
[ -n "$ID" ] || fail "no mdbook card id in response"
echo "$CARD" | python3 -c "import sys,json;c=json.load(sys.stdin);assert c['kind']=='mdbook' and c['dir']=='$MDBOOK_BOOKS/guide' and c['name']=='docs',c"
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"kind\":\"mdbook\"}" "http://127.0.0.1:$PORT/api/cards")
[ "$code" = "400" ] || fail "mdbook card without a project should be 400, got $code"
curl -fsS -b "$COOKIE" -X POST "http://127.0.0.1:$PORT/api/cards/$ID/start" >/dev/null || fail "mdbook start failed"
RUN="no"
for _ in $(seq 1 40); do
  STATUS=$(curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/cards")
  RUN=$(echo "$STATUS" | python3 -c "import sys,json;c=[x for x in json.load(sys.stdin)['cards'] if x['id']=='$ID'][0];print('yes' if c['running'] and c['healthy'] else 'no')")
  [ "$RUN" = "yes" ] && break
  sleep 0.5
done
[ "$RUN" = "yes" ] || { echo "$STATUS"; fail "mdbook did not become running"; }
# The card "Open" URL is same-origin and the book is proxied on the
# dashboard listener (no extra port to expose).
echo "$STATUS" | python3 -c "import sys,json;c=[x for x in json.load(sys.stdin)['cards'] if x['id']=='$ID'][0];assert c['url']=='/mdbook/',c"
code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/mdbook/")
[ "$code" = "302" ] || fail "unauthenticated mdbook proxy should redirect, got $code"
curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/mdbook/" >/dev/null || fail "mdbook same-origin proxy failed"
curl -fsS -b "$COOKIE" -X POST "http://127.0.0.1:$PORT/api/cards/$ID/stop" >/dev/null || fail "mdbook stop failed"
curl -fsS -b "$COOKIE" -X DELETE "http://127.0.0.1:$PORT/api/cards/$ID" >/dev/null || fail "mdbook delete failed"

echo "== vscode card =="
CARD=$(curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"kind":"vscode"}' \
  "http://127.0.0.1:$PORT/api/cards")
ID=$(python3 -c "import sys,json;print(json.loads(sys.argv[1])['id'])" "$CARD")
[ -n "$ID" ] || fail "no vscode card id in response"
echo "$CARD" | python3 -c "import sys,json;c=json.load(sys.stdin);assert c['kind']=='vscode' and c['icon']=='code' and c['name']=='VS Code' and c['password'],c"
VC_TOKEN=$(echo "$CARD" | python3 -c 'import sys,json;print(json.load(sys.stdin)["password"])')
[ -n "$VC_TOKEN" ] || fail "vscode card should show a connection token"
grep -q "\"connectionToken\": \"$VC_TOKEN\"" "$CFG" || fail "vscode connection token not persisted in config"
curl -fsS -b "$COOKIE" -X POST "http://127.0.0.1:$PORT/api/cards/$ID/start" >/dev/null || fail "vscode start failed"
RUN="no"
for _ in $(seq 1 40); do
  STATUS=$(curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/cards")
  RUN=$(echo "$STATUS" | python3 -c "import sys,json;c=[x for x in json.load(sys.stdin)['cards'] if x['id']=='$ID'][0];print('yes' if c['running'] and c['healthy'] else 'no')")
  [ "$RUN" = "yes" ] && break
  sleep 0.5
done
[ "$RUN" = "yes" ] || { echo "$STATUS"; fail "vscode did not become running"; }
[ -d "$TMP/vscode-data" ] || fail "vscode server data dir not created"
echo "$STATUS" | python3 -c "
import sys, json, re
c = [x for x in json.load(sys.stdin)['cards'] if x['id'] == sys.argv[1]][0]
m = re.search(r':(\d+)/', c['url'])
assert m and 4700 <= int(m.group(1)) <= 4899, c
assert '?tkn=' in c['url'] and c['password'], c" "$ID"
curl -fsS -b "$COOKIE" -X POST "http://127.0.0.1:$PORT/api/cards/$ID/stop" >/dev/null || fail "vscode stop failed"
curl -fsS -b "$COOKIE" -X DELETE "http://127.0.0.1:$PORT/api/cards/$ID" >/dev/null || fail "vscode delete failed"

echo "== vscodium card =="
CARD=$(curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"kind":"vscodium"}' \
  "http://127.0.0.1:$PORT/api/cards")
ID=$(python3 -c "import sys,json;print(json.loads(sys.argv[1])['id'])" "$CARD")
[ -n "$ID" ] || fail "no vscodium card id in response"
echo "$CARD" | python3 -c "import sys,json;c=json.load(sys.stdin);assert c['kind']=='vscodium' and c['icon']=='code' and c['name']=='VSCodium' and c['password'],c"
CD_TOKEN=$(echo "$CARD" | python3 -c 'import sys,json;print(json.load(sys.stdin)["password"])')
[ -n "$CD_TOKEN" ] || fail "vscodium card should show a connection token"
[ "$CD_TOKEN" != "$VC_TOKEN" ] || fail "vscodium and vscode tokens must differ"
grep -q "\"connectionToken\": \"$CD_TOKEN\"" "$CFG" || fail "vscodium connection token not persisted in config"
curl -fsS -b "$COOKIE" -X POST "http://127.0.0.1:$PORT/api/cards/$ID/start" >/dev/null || fail "vscodium start failed"
RUN="no"
for _ in $(seq 1 40); do
  STATUS=$(curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/cards")
  RUN=$(echo "$STATUS" | python3 -c "import sys,json;c=[x for x in json.load(sys.stdin)['cards'] if x['id']=='$ID'][0];print('yes' if c['running'] and c['healthy'] else 'no')")
  [ "$RUN" = "yes" ] && break
  sleep 0.5
done
[ "$RUN" = "yes" ] || { echo "$STATUS"; fail "vscodium did not become running"; }
[ -d "$TMP/vscodium-data" ] || fail "vscodium server data dir not created"
python3 - "$TMP/vscodium-data/data/Machine/settings.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
assert d["editor.fontSize"] == 99, d        # existing Remote Setting preserved
assert d["files.autoSave"] == "afterDelay", d  # missing key seeded from the file
PY
echo "$STATUS" | python3 -c "
import sys, json, re
c = [x for x in json.load(sys.stdin)['cards'] if x['id'] == sys.argv[1]][0]
m = re.search(r':(\d+)/', c['url'])
assert m and 4700 <= int(m.group(1)) <= 4899, c
assert '?tkn=' in c['url'] and c['password'], c" "$ID"
curl -fsS -b "$COOKIE" -X POST "http://127.0.0.1:$PORT/api/cards/$ID/stop" >/dev/null || fail "vscodium stop failed"
curl -fsS -b "$COOKIE" -X DELETE "http://127.0.0.1:$PORT/api/cards/$ID" >/dev/null || fail "vscodium delete failed"

echo "== wetty card =="
CARD=$(curl -fsS -b "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"kind":"wetty"}' \
  "http://127.0.0.1:$PORT/api/cards")
ID=$(python3 -c "import sys,json;print(json.loads(sys.argv[1])['id'])" "$CARD")
[ -n "$ID" ] || fail "no wetty card id in response"
echo "$CARD" | python3 -c "import sys,json;c=json.load(sys.stdin);assert c['kind']=='wetty' and c['icon']=='terminal' and c['name']=='WeTTY',c"
echo "$CARD" | python3 -c "import sys,json;c=json.load(sys.stdin);assert 'username' not in c and 'password' not in c,c"
curl -fsS -b "$COOKIE" -X POST "http://127.0.0.1:$PORT/api/cards/$ID/start" >/dev/null || fail "wetty start failed"
RUN="no"
for _ in $(seq 1 40); do
  STATUS=$(curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/cards")
  RUN=$(echo "$STATUS" | python3 -c "import sys,json;c=[x for x in json.load(sys.stdin)['cards'] if x['id']=='$ID'][0];print('yes' if c['running'] and c['healthy'] else 'no')")
  [ "$RUN" = "yes" ] && break
  sleep 0.5
done
[ "$RUN" = "yes" ] || { echo "$STATUS"; fail "wetty did not become running"; }
echo "$STATUS" | python3 -c "
import sys, json, re
c = [x for x in json.load(sys.stdin)['cards'] if x['id'] == sys.argv[1]][0]
m = re.search(r':(\d+)/', c['url'])
assert m and 4900 <= int(m.group(1)) <= 5099, c" "$ID"
curl -fsS -b "$COOKIE" -X POST "http://127.0.0.1:$PORT/api/cards/$ID/stop" >/dev/null || fail "wetty stop failed"
curl -fsS -b "$COOKIE" -X DELETE "http://127.0.0.1:$PORT/api/cards/$ID" >/dev/null || fail "wetty delete failed"

echo "== friendly port message =="
OUT=$("$BIN" serve --config "$CFG" --port "$PORT" 2>&1) || fail "second serve should exit 0"
echo "$OUT" | grep -qi "already running" || fail "expected already-running message, got: $OUT"
echo "$OUT" | grep -q "http://http" && fail "already-running message must not double http://, got: $OUT"
echo "$OUT" | grep -q "http://127.0.0.1:$PORT" || fail "already-running message should show 127.0.0.1 (wildcard bind), got: $OUT"

echo "== passwd rotation =="
kill "$SRV_PID" 2>/dev/null || true
wait "$SRV_PID" 2>/dev/null || true
"$BIN" passwd --config "$CFG" --password "$PASS2" >/dev/null || fail "passwd failed"

"$BIN" serve --config "$CFG" --port "$PORT" >"$TMP/server2.log" 2>&1 &
SRV_PID=$!
for _ in $(seq 1 40); do
  curl -fsS "http://127.0.0.1:$PORT/api/session" >/dev/null 2>&1 && break
  sleep 0.25
done
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE" "http://127.0.0.1:$PORT/api/cards")
[ "$code" = "401" ] || fail "old session should be rejected after passwd, got $code"
curl -fsS -c "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS2\"}" "http://127.0.0.1:$PORT/api/login" >/dev/null || fail "login with new password failed"
kill "$SRV_PID" 2>/dev/null || true
wait "$SRV_PID" 2>/dev/null || true

echo "== account change (username) =="
"$BIN" serve --config "$CFG" --port "$PORT" >"$TMP/server3.log" 2>&1 &
SRV_PID=$!
for _ in $(seq 1 40); do
  curl -fsS "http://127.0.0.1:$PORT/api/session" >/dev/null 2>&1 && break
  sleep 0.25
done
COOKIE2="$TMP/cookies2.txt"
RESP=$(curl -fsS -b "$COOKIE" -c "$COOKIE2" -H 'Content-Type: application/json' \
  -d "{\"currentPassword\":\"$PASS2\",\"username\":\"$NEWUSER\"}" "http://127.0.0.1:$PORT/api/account")
echo "$RESP" | python3 -c 'import sys,json;assert json.load(sys.stdin)["username"]=="'"$NEWUSER"'","username should change"'
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE" "http://127.0.0.1:$PORT/api/cards")
[ "$code" = "401" ] || fail "old session should be rejected after account change, got $code"
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE2" "http://127.0.0.1:$PORT/api/cards")
[ "$code" = "200" ] || fail "fresh session from account change should work, got $code"
curl -fsS -o /dev/null -H 'Content-Type: application/json' \
  -d "{\"username\":\"$NEWUSER\",\"password\":\"$PASS2\"}" "http://127.0.0.1:$PORT/api/login" || fail "login with new username failed"
code=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS2\"}" "http://127.0.0.1:$PORT/api/login")
[ "$code" = "401" ] || fail "old username should be rejected, got $code"
USER="$NEWUSER"
kill "$SRV_PID" 2>/dev/null || true
wait "$SRV_PID" 2>/dev/null || true

echo "== autostart (web) =="
"$BIN" serve --config "$CFG" --port "$PORT" >"$TMP/server4.log" 2>&1 &
SRV_PID=$!
for _ in $(seq 1 40); do
  curl -fsS "http://127.0.0.1:$PORT/api/session" >/dev/null 2>&1 && break
  sleep 0.25
done
curl -fsS -c "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS2\"}" "http://127.0.0.1:$PORT/api/login" >/dev/null || fail "login failed"
curl -fsS -b "$COOKIE" "http://127.0.0.1:$PORT/api/autostart" | python3 -c '
import sys, json
r = json.load(sys.stdin)
assert "supported" in r and isinstance(r["supported"], bool), r
assert r["label"] == "com.narthex.serve", r
print("  supported=%s installed=%s" % (r["supported"], r["installed"]))'
code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/api/autostart")
[ "$code" = "401" ] || fail "expected 401 for unauthed autostart GET, got $code"
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"action":"bogus"}' "http://127.0.0.1:$PORT/api/autostart")
[ "$code" = "400" ] || fail "expected 400 for bogus autostart action, got $code"
kill "$SRV_PID" 2>/dev/null || true
wait "$SRV_PID" 2>/dev/null || true

echo "== autostart (CLI, error paths) =="
# No action → error (exits non-zero). The action-validation runs before any
# language resolution and is hard-coded to English.
"$BIN" autostart >/dev/null 2>&1 && fail "autostart without action should fail"
# Unknown action → error.
("$BIN" autostart bogus 2>&1 || true) | grep -qi "unknown action" || fail "expected unknown-action error"
# Install with a missing config file → error before any launchctl call
# (works on every OS — the config-existence check precedes the platform
# check; the missing config also forces DefaultConfig, which is English).
("$BIN" autostart install --config /tmp/narthex-does-not-exist.json 2>&1 || true) \
  | grep -qi "config file not found" || fail "expected config-not-found error"
# Status is OS-dependent: on macOS it reports the plist state; on Linux it
# returns the unsupported-OS error. Force --lang en so the assertion is
# stable regardless of the host's configured language.
OUT=$("$BIN" autostart status --lang en 2>&1 || true)
echo "$OUT" | grep -qi "autostart" || fail "autostart status should mention autostart, got: $OUT"

echo "SMOKE OK"
