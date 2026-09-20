# Narthex — Agent 指南(中文)

本文档面向在 Narthex 代码库中工作的 AI Agent / 开发者:如何理解架构、复现构建、验证改动、遵守约定。

## 1. 项目定位

Narthex 是一个**超级轻量**的本地 Web 仪表盘:输入密码后,以「应用类型」为单位管理本机的应用(ComfyUI 服务器、opencode/mdBook/VS Code/VSCodium/WeTTY web),一键启动/停止并直达其网页界面。核心约束:

- **极低资源占用**:单一 Go 静态二进制,前端与素材 `go:embed` 内嵌,空闲内存 ~10 MB
- **最小依赖**:仅 `golang.org/x/crypto`(argon2id);**新增第三方依赖必须在此文档说明理由**
- **按 kind 管理**:每类应用(`comfyui` / `opencode` / `mdbook` / `vscode` / `vscodium` / `wetty`)至多一张卡片,`Kind` 为唯一标识;**没有路径概念,没有目录搜索**——mdBook 卡片除外,它存储所选书籍项目目录(见 §7 kind 语义)
- **启动时检测应用**:未安装的应用不出现在界面(meta 的 `apps` 报告 installed 状态)

## 2. 架构与数据流

```
浏览器(web/* 内嵌)
   │  HTTP + 会话 Cookie
   ▼
internal/server  路由 + 鉴权中间件 + 静态文件
   │
   ▼
internal/api     Service(互斥锁保护 State)
   │  ├─ auth:登录限速 → argon2id 校验 → HMAC 会话
   │  └─ procman.Backend 接口(进程操作的唯一出口):
   │        Manager:Start 按 kind 分发并 spawn 实例进程(Setpgid 独立进程组)
   ▼
internal/store   原子写 JSON:config.json / state.json
                实例日志 → logs/<cardID>.log
```

关键流程:

- **添加卡片**:`api.HandleCreateCard` → kind 合法 → 应用已安装(`err.appNotFound`)→ kind 未重复(`err.duplicateKind`);默认名/图标按 kind(ComfyUI→palette, opencode→terminal, mdbook→book-open, vscode→code, vscodium→code, wetty→terminal);mdBook 卡片额外要求 `dir` 属于 `mdbook.dirs` 下识别到的项目(`err.mdbookNoProject`)
- **启动卡片**:`api.HandleStartCard` → `Backend.Start(kind, dir, id)`;comfyui 的代码目录由 `procman.DetectComfyUIInstalls` 在启动时解析;opencode/VS Code/VSCodium/WeTTY 的工作目录为 `os.UserHomeDir()`;mdBook 的工作目录为卡片上存储的项目目录 → spawn(cwd=对应目录,`Setpgid: true`,日志写 `<LogDir>/<id>.log`)→ 记录 `pid/port/startedAt` 到 state;opencode 进程以 `OPENCODE_SERVER_PASSWORD` 环境变量携带 basic-auth 密码,VS Code/VSCodium 进程经 `--connection-token` 携带连接令牌,WeTTY 进程经 `--ssh-host`/`--ssh-port`/`--ssh-user` 携带 SSH 目标(无 HTTP 层密钥)
- **状态刷新**:前端每 5 秒 `GET /api/cards`;服务端对每个卡片调 `Backend.Status(kind, pid, port)`(kill 0 + TCP 健康检查 + RSS)
- **停止**:`Backend.Stop(pid)` → 进程组 SIGTERM,5 秒宽限后 SIGKILL
- **重启恢复**:state.json 持久化 pid/port,重启后按存活检测懒恢复;进程已死则显示「已停止」;桌面应用已在运行(单实例移交)时,`appProcessRunning`(pgrep)兜底识别
- **改密码/改用户名**:`narthex passwd` 与 `narthex reset-auth` 重算 argon2id 哈希并**旋转 `sessionSecret`**(所有已签发会话失效),同时用新密钥**重新加密**存储的用户名(见 `secretbox`);serve 启动时读取配置,故 CLI 修改需重启 serve 生效;顶栏「账号」页面(`/api/account`)修改则直接更新内存与磁盘,立即生效

## 3. 文件地图

| 路径 | 职责 |
|---|---|
| `cmd/narthex/main.go` | 入口:serve / setup / passwd / reset-auth / autostart 子命令与 flag;启动时打印检测到的应用(`detectedApps`);首次 serve 生成 opencode 密码与 VS Code/VSCodium 连接令牌(WeTTY 无需生成密钥);设置 vscode/vscodium 服务端数据目录(`Config.Vscode/Vscodium.DataDir`,默认 `<配置目录>/*-data`,展开 `~/`)并解析 vscode/vscodium 的 Machine 设置文件路径;端口占用预检;`resolveUsername`/`rotateSessionSecret`/`cmdAutostart` |
| `internal/api/service.go` | `Service` 结构(State + 互斥锁)、登录(用户名+密码)/会话/meta/settings/account 处理器、CardView 状态装配(经 Backend);检测接缝(`ComfyUIAppDir`/`ComfyUIDirs`/`OpencodeBin`/`MdbookBin`/`MdbookProjects`/`MdbookCreate`/`VscodeBin`/`VscodiumBin`/`WettyBin`);`HandleMeta` 返回 `apps`(kind → installed/label);卡片「打开」URL 装配 `instanceURL`(优先 `Config.InternalAddress`,否则请求主机);`IsLoopback`(仅 serve 警告用) |
| `internal/api/handlers.go` | 卡片 CRUD、start/stop;`appInstalled` 校验;`kindLabel` 默认名;`defaultIcon` 按 kind(palette/terminal/book-open/code);`installDir` 按 kind 解析(mdBook 用卡片 `dir`) |
| `internal/api/mdbook.go` | `HandleMdbookProjects`(GET:配置目录 + 识别到的书籍)与 `HandleMdbookCreate`(POST:在配置目录下执行 `mdbook init`,含名称/存在性校验) |
| `internal/api/uploads.go` | 背景图上传/删除/枚举、扩展名白名单、上传静态目录 |
| `internal/api/autostart.go` | `HandleAutostart`:GET(状态)/ POST(install/uninstall)用户级 serve launchd 任务 |
| `internal/api/json.go` | JSON 读写辅助 |
| `internal/auth/password.go` | argon2id 哈希/校验、随机密码生成 |
| `internal/auth/session.go` | HMAC-SHA256 无状态会话(Sign/Verify;TTL 由配置决定) |
| `internal/auth/ratelimit.go` | 内存固定窗口限速(含 Reset) |
| `internal/autostart/autostart.go` | macOS launchd 安装/卸载/查询(用户级 serve);纯函数 `Label`/`PlistPath`/`Render` 加副作用 `Install`/`Uninstall`/`Status`,通过包级函数变量接缝(`fnBootstrap` 等)便于测试;非 darwin 返回 `ErrUnsupportedOS` |
| `internal/i18n/i18n.go` | CLI/API 消息目录(en / zh-CN / zh-TW),`T(lang, key)` 翻译 + en 回退 |
| `internal/procman/backend.go` | `Backend` 接口(`Start(kind, dir, id)`/`Stop`/`Status(kind, pid, port)`)与 `Status` 结构(进程操作唯一出口) |
| `internal/procman/manager.go` | `Manager`:`Start` 按 kind 分发(comfyui → 服务器,opencode → web,mdbook → serve,vscode/vscodium → serve-web,wetty → wetty)、`spawn`(进程组+日志)、`Stop`(SIGTERM→SIGKILL)、`Status`(Alive + TCP 健康检查)、`Alive`、`FreePort`/`freePortIn` |
| `internal/procman/app.go` | `.app` 检测(`detectAppBundle`:env 覆盖 + `/Applications` + `~/Applications`) |
| `internal/procman/comfyui.go` | ComfyUI 服务器启动(`main.py --port --listen`,python 解析 `standalone-env`/`.venv`/PATH)、`DetectComfyUIInstalls`(从 `installations.json` 解析代码目录,`NARTHEX_COMFY_DESKTOP_DIR` 可覆盖)、`DetectComfyUIApp`(`Comfy Desktop.app`,`NARTHEX_COMFY_APP_DIR` 可覆盖) |
| `internal/procman/opencode.go` | opencode web 启动:`opencode web --port --hostname`,cwd=项目目录,`OPENCODE_SERVER_PASSWORD` 注入 basic-auth;`DetectOpencode`(PATH 查找 `opencode`,`NARTHEX_OPENCODE_BIN` 可覆盖) |
| `internal/procman/mdbook.go` | mdBook 启动:`mdbook serve --port --hostname`,cwd=书籍项目目录;`DetectMdbook`(PATH 查找 `mdbook`,`NARTHEX_MDBOOK_BIN` 可覆盖);`DetectMdbookProjects`(对配置目录做一层扫描识别 `book.toml`);`CreateMdbookProject`/`ValidBookName`(`mdbook init --force --ignore none`,非交互) |
| `internal/procman/wetty.go` | WeTTY 终端启动:`wetty --port --host [--ssh-host --ssh-port --ssh-user]`,cwd=用户主目录;`DetectWetty`(PATH 查找 `wetty`,`NARTHEX_WETTY_BIN` 可覆盖) |
| `internal/procman/vscode.go` | VS Code/VSCodium web 启动:`<code|codium> serve-web --host --port --connection-token --server-data-dir --accept-server-license-terms`(经 `startCodeServe`),cwd=用户主目录,子进程 PATH 前置 no-open 目录;`seedMachineSettings` 把配置的 JSON 文件中缺失的键补齐到 `<server-data-dir>/data/Machine/settings.json`(已有键不覆盖);`DetectVSCode`(PATH 查找 `code`,`NARTHEX_VSCODE_BIN` 可覆盖)、`DetectVSCodium`(PATH 查找 `codium`,`NARTHEX_VSCODIUM_BIN` 可覆盖)——两个独立类型,各有独立服务端数据目录(`VscodeDataDir`/`VscodiumDataDir`)与 Machine 设置文件(`VscodeMachineSettingsFile`/`VscodiumMachineSettingsFile`) |
| `internal/procman/probe.go` | TCP 健康检查、内存(macOS `ps` / Linux `/proc`) |
| `internal/server/server.go` | 路由(Go 1.22+ method patterns)、鉴权中间件、`go:embed` 静态文件 + 缓存头 |
| `internal/secretbox/secretbox.go` | 用户名静态加密:AES-256-GCM,密钥由 `sessionSecret` 经 HMAC-SHA256 派生;`Encrypt`/`Decrypt`/`DeriveKey` |
| `internal/store/store.go` | Config/State 结构、原子写(临时文件+rename)、配置目录(~/.config/narthex);`LoadState` 按 kind 去重并丢弃未知 kind |
| `internal/termios/*` | 无回显密码读取(unix termios + 其他平台回退) |
| `web/web.go` | `//go:embed` 前端全部资产 |
| `web/index.html`、`app.css`、`app.js` | 单页前端:登录/仪表盘/卡片/弹窗,玻璃拟态 + 响应式;添加弹窗无搜索步骤,kind 列表由 `meta.apps` + 已添加卡片过滤;选择 mdBook 类型后先进入「项目选择」步骤(在 `mdbook.dirs` 下选已有书籍或新建一本),再进入名称/图标/背景;VS Code/VSCodium 卡片还提供「临时打开」按钮,打开 URL 时不带 `?tkn=` 令牌 |
| `web/i18n.js` | 前端消息目录(en/zh-CN/zh-TW)、`t()`、`applyI18n()`、`data-i18n` 应用 |
| `web/assets/icons/*.svg` | 37 个 Lucide 图标(ISC) |
| `web/assets/backgrounds/*.jpg` | 8 张背景图(picsum/Unsplash) |
| `scripts/download-assets.sh` | 下载素材(已提交仓库,构建离线可用) |
| `scripts/smoke-test.sh` | 端到端冒烟:local 全流程(含首次运行无配置自动 setup) + passwd/账号会话轮转 + comfyui/opencode/mdbook/vscode/vscodium/wetty 卡片生命周期 + mdbook projects API + autostart 状态/错误路径(用假的 .app bundle 与假 `opencode`/`mdbook`/`code`/`codium`/`wetty`,不触碰真实应用) |

## 4. 环境要求

- Go ≥ 1.26(`go.mod` 声明 `go 1.26`)
- 运行时:被管理的应用(`Comfy Desktop.app` 位于 `/Applications` 或 `~/Applications`,或经 `NARTHEX_COMFY_APP_DIR` 指定;`opencode` CLI 位于 PATH,或经 `NARTHEX_OPENCODE_BIN` 指定;`mdbook` CLI 位于 PATH 且 `mdbook.dirs` 非空,或经 `NARTHEX_MDBOOK_BIN` 指定;`code` CLI 位于 PATH,或经 `NARTHEX_VSCODE_BIN` 指定;`codium` CLI 位于 PATH,或经 `NARTHEX_VSCODIUM_BIN` 指定);构建与测试不需要
- macOS(开发环境)或 Linux;测试脚本依赖 `bash`、`curl`、`python3`

## 5. 复现步骤(从零到可用)

```bash
git clone <repo> && cd Narthex
go mod tidy              # 仅拉取 golang.org/x/crypto
make build               # → build/narthex(单一二进制,无需其他文件)
./build/narthex setup      # 交互式设置密码 → ~/.config/narthex/config.json
./build/narthex serve      # → http://127.0.0.1:9090
```

非交互: `./build/narthex setup --random --force`(打印随机密码);登录用户名默认 `narthex`,可用 `--username` 指定。
改密码/用户名:`./build/narthex passwd` 或 `reset-auth`(旋转 sessionSecret,重启 serve 生效)。
serve 启动时打印 `detected apps: ...`,只有检测到的应用才能在界面添加卡片。

## 6. 验证清单(改动后必须执行)

1. `gofmt -l .` 无输出
2. `go vet ./...` 无警告
3. `go test ./...` 全过:auth(哈希/会话/限速)、secretbox(加密往返/错误密钥/篡改/随机 nonce)、i18n(三语 key 完整性)、procman(桌面应用生命周期/进程扫描兜底/健康检查/内存/`.app` 检测/opencode 启停与报错/mdbook 检测·项目扫描·init/vscode·vscodium 检测与生命周期(含 `--server-data-dir` 与 Machine 设置补齐)/wetty 检测与生命周期)、store(默认值/规范化/原子写往返/按 kind 去重)、autostart(plist 渲染/路径解析/install/uninstall/status 经 fakes,不真调 launchctl)、api(fake Backend 全接口集成:鉴权/CRUD/生命周期/限速/meta(含 apps)/语言切换/会话 TTL/账号修改/kind 校验与去重/应用未安装拒绝/内部组网地址 URL 替换/mdbook 卡片与 projects 端点/vscode·vscodium 令牌展示/wetty 卡片)
4. `make smoke` 输出 `SMOKE OK`(local 全流程(含首次运行无配置自动 setup) → **passwd 会话轮转**(旧 Cookie 401、新密码登录)→ **账号修改**(改用户名、旧会话 401、新用户名登录、旧用户名 401)→ **comfyui 卡片**(假 `Comfy Desktop.app` + `NARTHEX_COMFY_APP_DIR`,建卡/启动/URL 8000/停止/删除,重复 kind 409)→ **opencode 卡片**(假 `opencode` CLI + PATH 注入,建卡/启动/健康/停止/删除、密码生成并持久化)→ **mdbook projects API + 卡片**(假 `mdbook` CLI + `mdbook.dirs`,列出/新建书籍,卡片启动/健康/停止/删除)→ **vscode 卡片**(假 `code` CLI,建卡/启动/健康/停止/删除、令牌生成并持久化、`--server-data-dir` 目录已创建)→ **vscodium 卡片**(假 `codium` CLI,建卡/启动/健康/停止/删除、独立令牌生成并持久化、`--server-data-dir` 目录已创建、Machine 设置补齐且保留已有键)→ **wetty 卡片**(假 `wetty` CLI,建卡/启动/健康/停止/删除、无凭据)→ **autostart CLI**(status / 缺少操作 / 未知操作 / 缺少配置 错误路径,不实际 launchctl load))
5. 前端改动需人工验证:登录页、卡片增删改、启动/停止、添加弹窗只列「已安装且未添加」的 kind、VS Code/VSCodium 卡片的「临时打开」按钮(不带令牌打开,停止时隐藏)、≤768px 移动端布局

## 7. 约定(必须遵守)

- **依赖铁律**:仅允许 `golang.org/x/crypto`(官方)。新增依赖前须在本节更新理由;优先标准库实现(参考 `internal/termios` 自实现无回显读取)
- **无注释噪音**:代码不加装饰性注释,只写解释「为什么」的注释
- **状态文件格式**:`state.json` 的 `Card` 字段变更属破坏性变更——新字段必须带 `omitempty` 或有向后兼容的默认值处理(`store.LoadState` 内);不要重命名已有 JSON 字段
- **API 变更**:所有 `/api/*`(除 login/session/logout)必须保持鉴权;新增端点注册在 `internal/server/server.go` 的 protected mux;前端与之同步
- **并发**:所有读写 `Service.State` 的处理器必须先 `s.mu.Lock()`;进程操作(Start/Stop)在持锁状态下执行,避免同卡片并发启停
- **进程操作唯一出口**:任何启动/停止/状态查询必须走 `procman.Backend` 接口,禁止在 api/server 层直接调 `os/exec` 或 `procman` 包内函数
- **改密码必轮转**:`passwd`/`reset-auth`/`/api/account` 必须同时旋转 `sessionSecret` 并**重新加密** `usernameEnc`(两者耦合,见 `secretbox`);只改 `passwordHash` 不改 secret 会被视为缺陷
- **用户名只存密文**:配置中禁止明文用户名;新账号/改账号一律 `secretbox.Encrypt`,读取走 `resolveUsername`/`secretbox.Decrypt`;用户名仅解密后驻留内存(api.Service.Username)
- **settings 语义**:`POST /api/settings` 逐字段应用——`language`/`pageBackground` 传空串视为"未设置"(不修改),`compact` 用指针区分显式 false;`slogan` 为 `*string`(`nil` 不修改,`""` 关闭,非空自定义 ≤80 字符);`internalAddress` 为 `*string`(`nil` 不修改,空串清除,自定义 ≤253 字符);`nil` 且未设置时按当前语言解析 `i18n.T(lang, "slogan.default")`,自定义后不再随语言(见 `api.sloganValue`)
- **内部组网地址**:`Config.InternalAddress` 配置后,卡片「打开」URL(`api.instanceURL`)优先使用该地址代替请求主机;地址可含端口(经 `net.SplitHostPort` 判断,带端口则原样使用、不带端口则拼接卡片端口);前端「设置」弹窗的输入框占位符显示当前访问主机(`window.location.host`),输入防抖保存(见 `web/app.js`)
- **i18n 铁律**:所有用户可见文案(CLI、API 错误、前端)必须经 `i18n.T`/`t()`,禁止硬编码;`internal/i18n` 的完整性单测会自动发现缺译(三种语言必须齐全)
- **上传安全**:`/api/uploads` 仅接受 jpg/png/webp、≤15MB、文件名随机 hex;`/uploads/` 静态服务必须拒绝路径穿越与未知扩展名(见 `server.uploadsHandler` 与 `api.UploadExt`)
- **新增语言步骤**:在 `internal/i18n` 三个语言 map 补齐全部 key → `web/i18n.js` 三字典补齐 → `i18n.Parse` 常量白名单加入新语言 → `index.html` 语言下拉加选项 → 双语文档更新
- **kind 语义**:卡片以 `Kind` 为唯一标识(`comfyui`/`opencode`/`mdbook`/`vscode`/`vscodium`/`wetty`),每类至多一张;创建时校验应用已安装(400 `err.appNotFound`)与 kind 未重复(409 `err.duplicateKind`);卡片**不存储路径**,安装目录在启动时由检测函数解析(**例外**:mdBook 卡片将所选书籍项目存入 `Card.Dir`,创建时须通过 `mdbook.dirs` 下检测到的项目校验)
- **comfyui 服务器启动**:经 `procman.startComfyUI`(spawn `<python> main.py --port <n> --listen <host>`,cwd=安装代码目录,Setpgid);`appInstalled` 要求 `DetectComfyUIApp()` 与 `DetectComfyUIInstalls()` 均非空;监听地址与端口范围来自 `Config.ComfyUI`(`0.0.0.0` 默认,局域网可访问)
- **opencode web 启动**:经 `procman.startOpencode`(spawn `opencode web --port <n> --hostname <host>`,cwd=用户主目录,Setpgid);`appInstalled` 要求 `DetectOpencode()` 非空;**必须设置 `OPENCODE_SERVER_PASSWORD`**——服务绑定 `0.0.0.0`,无密码即局域网裸奔。密码首次 serve 时生成并持久化到 `Config.Opencode.Password`,经 `CardView.Password` 展示在卡片上。监听地址与端口范围来自 `Config.Opencode`(`0.0.0.0`/`[4300, 4499]` 默认)
- **mdBook 服务器启动**:经 `procman.startMdbook`(spawn `mdbook serve --port <n> --hostname <host>`,cwd=书籍项目目录,Setpgid);`appInstalled` 要求 `DetectMdbook()` 非空**且 `Config.Mdbook.Dirs` 非空**——目录列表为空时 mdBook 类型整体隐藏;创建卡片要求 `dir` 属于 `DetectMdbookProjects(Config.Mdbook.Dirs)`;新建书籍仅允许在配置目录下经 `CreateMdbookProject`(`mdbook init --force --ignore none`,非交互)。监听地址与端口范围来自 `Config.Mdbook`(`0.0.0.0`/`[4500, 4699]` 默认)
- **VS Code web 启动**:经 `procman.startVSCode`(spawn `code serve-web --host <h> --port <n> --connection-token <t> --server-data-dir <d> --accept-server-license-terms --disable-telemetry`,cwd=用户主目录,Setpgid,子进程 PATH 前置 no-open 目录);`appInstalled` 要求 `DetectVSCode()` 非空;**必须带 `--connection-token`**——服务绑定 `0.0.0.0`,无令牌即局域网裸奔。令牌首次 serve 时生成并持久化到 `Config.Vscode.ConnectionToken`,经 `CardView.Password` 展示在卡片上(无用户名);卡片「打开」URL 经 `api.instanceURL`/`api.codeToken` 内嵌令牌(`?tkn=`),因为无令牌时服务端返回 403(浏览器首次加载时种下认证 Cookie)。`--server-data-dir` 来自 `Config.Vscode.DataDir`(默认 `<配置目录>/vscode-data`,会展开 `~/`)并固定服务端数据:浏览器*用户*设置存在浏览器(IndexedDB)且可能被淘汰(隐私窗口、Safari ITP),所以需要持久的设置应写入服务端 Machine 设置;设置 `Config.Vscode.MachineSettingsFile` 后,其键会在启动前补齐到 `<data-dir>/data/Machine/settings.json`(仅补缺失键,远程设置的改动优先)。把 `DataDir` 指向另一个服务端的目录(如 `~/.vscodium-server`)可与手动启动的实例共用服务端状态。监听地址与端口范围来自 `Config.Vscode`(`0.0.0.0`/`[4700, 4899]` 默认)
- **VSCodium web 启动**:经 `procman.startVSCodium`(spawn `codium serve-web --host <h> --port <n> --connection-token <t> --server-data-dir <d> --accept-server-license-terms --disable-telemetry`,cwd=用户主目录,Setpgid,子进程 PATH 前置 no-open 目录);`appInstalled` 要求 `DetectVSCodium()` 非空;令牌首次 serve 时生成并持久化到 `Config.Vscodium.ConnectionToken`,经 `CardView.Password` 展示在卡片上(无用户名),「打开」URL 同样携带令牌。它与 VS Code 是完全独立的类型(独立配置/令牌/端口分配/`--server-data-dir` 取自 `Config.Vscodium.DataDir`/`Config.Vscodium.MachineSettingsFile`),可同时运行。监听地址与端口范围来自 `Config.Vscodium`(`0.0.0.0`/`[4700, 4899]` 默认,启动时自动挑空闲端口)
- **WeTTY 终端启动**:经 `procman.startWetty`(spawn `wetty --port <n> --host <h> [--ssh-host <h>] [--ssh-port <n>] [--ssh-user <u>]`,cwd=用户主目录,Setpgid);`appInstalled` 要求 `DetectWetty()` 非空。WeTTY **没有 HTTP 层鉴权**:浏览器以 `Config.Wetty.SSHHost`(默认 `localhost`)的 SSH 账号登录,SSH 目标只来自配置,绝不由 URL 参数决定(`--allow-remote-hosts`/`--allow-remote-command` 保持关闭)。卡片不展示任何凭据。监听地址、端口范围与 SSH 目标来自 `Config.Wetty`(`0.0.0.0`/`[4900, 5099]` 默认)
- **端口约定**:comfyui 端口从 `comfyui.portRange`(默认 [4100, 4299])探测分配,opencode 端口从 `opencode.portRange`(默认 [4300, 4499])探测分配,mdbook 端口从 `mdbook.portRange`(默认 [4500, 4699])探测分配,vscode 端口从 `vscode.portRange`(默认 [4700, 4899])探测分配,vscodium 端口从 `vscodium.portRange`(默认 [4700, 4899])探测分配,wetty 端口从 `wetty.portRange`(默认 [4900, 5099])探测分配,禁止写死。健康检查统一拨测 `127.0.0.1`

## 8. 常见扩展点

- **新增受管应用**:在 `procman` 增加检测函数(`detectAppBundle`)+ 启动分支 + `store` 增加 kind 常量;`api.kindLabel`/`defaultIcon` 补充默认名/图标;前端 `availableKinds` 列表补充 kind 与 `modal.kind<kind>Desc` 文案(遵守 §7 kind 语义)
- **新增素材**:加入 `web/assets/`,重新 build(embed 自动包含);图标/背景清单由 `/api/meta` 运行时读取
- **中/英文文档**:改根 `README.md`(英文)与 `docs/README.zh-CN.md`(中文)成对更新;`docs/agents/AGENTS.*` 同理;根 `AGENTS.md` 仅做指针

## 9. 已知限制

- 只管理由 Narthex 启动的实例进程;外部自行启动的同端口服务不在卡片状态内
- 内存值取进程 RSS,不含子进程总和
- 会话为无状态签名 Cookie,注销后 30 天(默认,`sessionTTLHours` 可调)内签名仍有效(属可接受的权衡;如需立即失效可旋转 `sessionSecret`)
- 改密码后需重启 `narthex serve` 才生效(sessionSecret/passwordHash 启动时读入内存);页面内改账号(`/api/account`)即时生效无需重启
- ComfyUI 服务器由 Narthex 直接以 `python main.py` 启动,与 ComfyUI Desktop 窗口相互独立;若用户同时运行桌面应用,其内嵌服务器端口(8000–9000 区间)不受 Narthex 管理