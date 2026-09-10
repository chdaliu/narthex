# Narthex — 轻量应用启动面板

输入用户名与密码后,在 Web 端一键启动/停止本机的应用(**ComfyUI 服务器**、**opencode web**、**mdBook 文档**、**VS Code/VSCodium web**、**WeTTY 终端**)并直达其网页界面。每类应用最多一张卡片,以应用类型为唯一标识:可查看运行状态与内存占用,一键启动/停止;卡片图标与背景可编辑。界面为简约玻璃拟态设计(透明 + 背景图),适配各种设备屏幕。

## 特性

- **超级轻量**:单一静态二进制,前端与素材经 `go:embed` 内嵌,空闲内存 ~10 MB;除 `golang.org/x/crypto` 外零依赖
- **卡片管理**:图标(37 个 Lucide 图标)与背景(8 张免费图片)可编辑;每类应用一张卡片,以类型(`comfyui` / `opencode` / `mdbook` / `vscode` / `vscodium` / `wetty`)为唯一标识(仅 mdBook 卡片含项目目录)
- **应用检测**:启动时自动检测已安装的应用;**未安装的不显示**添加入口,无法创建/启动对应卡片
  - `comfyui` 卡片启动 **ComfyUI 服务器**(`python main.py --port <n> --listen 0.0.0.0`),代码目录与解释器从 ComfyUI Desktop 的安装记录解析(`installations.json`,可用 `NARTHEX_COMFY_DESKTOP_DIR` 覆盖);端口在可配置范围(默认 4100–4299)内自动分配,监听 `0.0.0.0` **支持局域网访问**——「打开」按钮本机为 `http://127.0.0.1:<port>/`,其它设备为 `http://<面板主机>:<port>/`
  - `opencode` 卡片启动 **opencode web 界面**(`opencode web --port <n> --hostname 0.0.0.0`),在浏览器中进行 AI 编程;端口在可配置范围(默认 4300–4499)内自动分配,监听 `0.0.0.0` **支持局域网访问**。因服务绑定非回环地址,首次启动会**自动生成并持久化一个随机密码**(`OPENCODE_SERVER_PASSWORD`,用户名 `opencode`),**用户名与密码都显示在卡片上、可点击复制**,serve 首次生成时也会打印;启动时**不会在宿主机自动弹出浏览器**——通过「打开」按钮进入
  - `mdbook` 卡片启动 **mdBook 文档服务器**(`mdbook serve --port <n> --hostname 0.0.0.0`);该类型仅在 **`mdbook` CLI 已安装且配置了 `mdbook.dirs`** 时显示。选择该类型后,会列出各配置目录下**一层深度**内识别到的书籍项目(含 `book.toml` 的目录),也可在配置目录下**新建书籍**(`mdbook init`);所选项目目录存入卡片
  - `vscode` 卡片启动 **VS Code Web 服务器**(`code serve-web --host 0.0.0.0 --port <n>`);该类型在 **`code` CLI** 已安装时显示。因服务绑定非回环地址,首次启动会**自动生成并持久化一个连接令牌**(`vscode.connectionToken`),**显示在卡片上、可点击复制**,且「打开」URL 会带上令牌(`?tkn=`),首次加载浏览器即自动认证;不预先选择文件夹,浏览器中可浏览服务器文件系统打开文件夹
  - `vscodium` 卡片启动 **VSCodium Web 服务器**(`codium serve-web --host 0.0.0.0 --port <n>`),与 VS Code 相互独立,两者可同时建卡运行;该类型在 **`codium` CLI** 已安装时显示,令牌独立生成并持久化(`vscodium.connectionToken`),「打开」URL 同样携带令牌
  - `wetty` 卡片启动 **WeTTY 终端网页服务器**(`wetty --port <n> --host 0.0.0.0`),在浏览器中使用终端;该类型在 **`wetty` CLI** 已安装时显示。WeTTY **没有 HTTP 层鉴权**:浏览器会要求输入 `wetty.sshHost`(默认 `localhost`)对应的 **SSH 账号**,因此宿主机需开启 SSH 登录。监听地址、端口范围与 SSH 目标来自 `wetty.*`;卡片不显示任何凭据
  - `comfyui` 检测位置:`/Applications` 与 `~/Applications`,可用 `NARTHEX_COMFY_APP_DIR` 覆盖;`opencode` 通过 PATH 检测 `opencode` CLI(可用 `NARTHEX_OPENCODE_BIN` 覆盖);`mdbook` 通过 PATH 检测 `mdbook` CLI(可用 `NARTHEX_MDBOOK_BIN` 覆盖);`vscode` 通过 PATH 检测 `code` CLI(可用 `NARTHEX_VSCODE_BIN` 覆盖);`vscodium` 通过 PATH 检测 `codium` CLI(可用 `NARTHEX_VSCODIUM_BIN` 覆盖);`wetty` 通过 PATH 检测 `wetty` CLI(可用 `NARTHEX_WETTY_BIN` 覆盖)
- **实例管理**:实例进程由 Narthex 托管(独立进程组,可整树终止);状态灯 + 健康检查(TCP)+ 运行时长 + 内存(RSS);Narthex 重启后自动重新关联
- **改密码**:`narthex passwd` 重设密码并轮转会话密钥,所有会话即刻失效
- **账号管理**:顶栏「账号」独立页面或 `reset-auth` 命令可修改用户名(登录名,默认 `narthex`)与密码;用户名在配置中以 AES-256-GCM **加密存储**,密码仅存 argon2id 哈希
- **三语界面**:en(默认)/ 简体中文 / 繁體中文,启动参数或页面内下拉一键切换,CLI 提示同步三语
- **安全**:argon2id 密码哈希、HMAC 签名会话 Cookie(默认 30 天,可配置 `sessionTTLHours`)、登录限速(5 次/分钟,成功即重置)
- **响应式**:卡片网格自适应,移动端单列大触控区

## 环境要求

- Go ≥ 1.26(仅构建需要)
- 被管理应用的安装位置:
  - `comfyui` 卡片需要已安装 **ComfyUI Desktop**(`/Applications/Comfy Desktop.app` 或用户级安装,可用 `NARTHEX_COMFY_APP_DIR` 覆盖),且桌面应用里有至少一个**已安装的 ComfyUI 实例**(`installations.json`,支持目录可用 `NARTHEX_COMFY_DESKTOP_DIR` 覆盖)
- `opencode` 卡片需要已安装 **opencode CLI**(位于 PATH,可用 `NARTHEX_OPENCODE_BIN` 覆盖)
- `mdbook` 卡片需要已安装 **mdbook CLI**(位于 PATH,可用 `NARTHEX_MDBOOK_BIN` 覆盖),且配置中 `mdbook.dirs` 非空
- `vscode` 卡片需要已安装 **VS Code(`code`)CLI**(位于 PATH,可用 `NARTHEX_VSCODE_BIN` 覆盖)
- `vscodium` 卡片需要已安装 **VSCodium(`codium`)CLI**(位于 PATH,可用 `NARTHEX_VSCODIUM_BIN` 覆盖)
- `wetty` 卡片需要已安装 **WeTTY CLI**(位于 PATH,可用 `NARTHEX_WETTY_BIN` 覆盖),且 `wetty.sshHost`(默认 `localhost`)上有可登录的 SSH 服务
- macOS 或 Linux(内存读取:macOS 用 `ps`,Linux 用 `/proc/<pid>/status`)

## 快速开始

```bash
make build            # 构建 build/narthex
./build/narthex serve   # 启动,默认 http://127.0.0.1:9090
```

**首次运行无需手动 setup**:若未找到配置,`serve` 会自动生成一份并设置**随机密码**(打印在终端),用户名默认 `narthex`。也可以先手动初始化:

```bash
./build/narthex setup   # 交互式设置密码,生成 ~/.config/narthex/config.json
./build/narthex serve   # 启动,默认 http://127.0.0.1:9090
```

浏览器打开 `http://127.0.0.1:9090`,输入用户名(`narthex`)与密码即可。

### setup 选项

| 选项 | 说明 |
|---|---|
| `--config <path>` | 配置文件路径(默认 `~/.config/narthex/config.json`) |
| `--password <pw>` | 非交互式设置密码 |
| `--random` | 生成随机密码并打印 |
| `--force` | 覆盖已有配置 |
| `--username <name>` | 登录用户名(默认 `narthex`),加密写入配置 |

### serve 选项

| 选项 | 说明 |
|---|---|
| `--config <path>` | 配置文件路径 |
| `--port <n>` | 覆盖配置中的监听端口 |
| `--hostname <h>` | 覆盖监听地址(`0.0.0.0` 供局域网访问) |
| `--lang <lang>` | 本次运行的语言:`en`(默认)/`zh-CN`/`zh-TW`,覆盖配置值 |

启动时会打印检测到的应用,例如 `detected apps: ComfyUI, opencode, MdBook, VS Code, VSCodium, WeTTY`;未安装的应用不会显示在界面中。

> 常驻运行:macOS 用 `narthex autostart install`(launchd),见[开机自启](#开机自启);Linux 用 systemd 或 `nohup ./build/narthex serve &`。
> 重复启动:若端口上已有 narthex 在运行,serve 会提示「已在运行」并正常退出,不会报错。

### 语言切换

- **启动参数**:`narthex serve --lang zh-TW`(所有子命令均支持 `--lang`;`setup` 会把所选语言写入配置)
- **页面内切换**:顶栏右侧语言下拉(English / 简体中文 / 繁體中文),选择后立即生效并写回配置,无需刷新
- 默认 **en**;命令行提示信息(setup/passwd/serve)同样三语

### 修改账号(用户名 / 密码)

```bash
./build/narthex passwd           # 交互式输入新密码
./build/narthex passwd --random  # 生成随机密码并打印
./build/narthex reset-auth --username myname --password newpass  # 一键重设用户名与密码
```

修改密码会同时**旋转 `sessionSecret`**,所有已登录会话即刻失效;需重启 `narthex serve` 生效。

登录用户名与密码也可以在顶栏「账号」页面中修改(需输入当前密码),修改同样轮转会话密钥,且会用新密钥**重新加密**存储的用户名。

## 开机自启

`narthex autostart` 在 macOS 上安装 launchd 任务,让 narthex 开机/登录时自动启动,无需手写 plist。在需要自启的机器上:

```bash
narthex autostart install                # serve 登录时自启(用户级 LaunchAgent)
narthex autostart status                 # 查看安装状态
narthex autostart uninstall              # 卸载
```

launchd 任务指向当前二进制(`os.Executable()`),移动或重新构建二进制后需重装一次。参数:`--config <path>`(默认 `~/.config/narthex/config.json`)、`--lang`。日志写入 `~/.config/narthex/serve.log`。

面板「设置」弹窗也有「登录时自启」开关,一键管理用户级 serve 任务。

> Linux:`narthex autostart` 会返回「仅支持 macOS」提示,请改用 systemd 或 `nohup`。

## 使用

1. **添加卡片**:点击「添加卡片」→ 选择应用(**ComfyUI** / **opencode** / **MdBook** / **VS Code** / **VSCodium** / **WeTTY**,仅显示**已安装且尚未添加**的)→ 设置名称/图标/背景 → 创建。没有路径、没有搜索——应用目录由后台自动检测(**MdBook** 类型会先让你在 `mdbook.dirs` 下选择书籍项目,或新建一本;所选项目目录存入卡片)
2. **启动/停止**:卡片上点「启动」或「停止」;启动中的实例显示「启动中…」,就绪后显示绿色状态灯、内存占用与运行时长
3. **打开**:点「打开」在新标签打开网页界面(ComfyUI / opencode / mdBook / VS Code / VSCodium / WeTTY 均为 `http://<主机>:<端口>/`,VS Code / VSCodium 的链接会带上连接令牌 `?tkn=`,浏览器自动认证);均监听 `0.0.0.0`,其它设备可用面板的同一主机名直达。若在「设置」里配置了**内部组网地址**,跳转地址改用该地址(详见[配置](#配置))。**opencode 卡片显示网页登录的用户名与密码**(用户名 `opencode`),**VS Code / VSCodium 卡片显示连接令牌**,点击即可复制;浏览器首次访问输入一次后会记住,之后自动携带。**WeTTY 卡片不显示凭据**:浏览器会要求输入 `wetty.sshHost`(默认 `localhost`)的 SSH 账号
4. **编辑**:改名称、图标、背景;删除卡片会先停止应用
5. **设置**:顶栏「设置」→ 语言 / 页面背景(含上传)/ 标语(开关与自定义)/ **内部组网地址** / 登录时自启开关(macOS)全部即时生效
   - **内部组网地址**:配置后,卡片「打开」按钮的跳转地址改用该主机(IP 或域名,可含端口),而不是访问面板所用的地址;留空则沿用当前访问地址(输入框占位符会显示当前地址)
6. **账号**:顶栏「账号」→ 独立页面修改用户名与密码(需当前密码),保存后其它设备会话失效
7. **紧凑模式**:顶栏「紧凑模式/标准模式」按钮切换,偏好持久化到配置
8. **背景导入**:
   - **页面背景**:顶栏「背景」按钮 → 选择预设或上传本地图片(jpg/png/webp,≤15MB)即生效
   - **卡片背景**:添加/编辑卡片弹窗的背景选择器末尾「＋」上传,导入后自动选中
   - 导入的图片存于 `~/.config/narthex/uploads/`,可在选择器中删除

## 配置

`~/.config/narthex/config.json`:

```json
{
  "hostname": "0.0.0.0",
  "port": 9090,
  "sessionSecret": "<自动生成>",
  "passwordHash": "argon2id$v=19$m=65536,t=3,p=2$…",
  "usernameEnc": "<AES-256-GCM 加密的用户名>",
  "language": "en",
  "sessionTTLHours": 720,
  "compactCards": false,
  "pageBackground": "bg-05",
  "slogan": "万物始于无,亦终于无",
  "internalAddress": "",
  "comfyui": { "hostname": "0.0.0.0", "portRange": [4100, 4299] },
  "opencode": { "hostname": "0.0.0.0", "portRange": [4300, 4499], "password": "<自动生成>" },
  "mdbook": { "hostname": "0.0.0.0", "portRange": [4500, 4699], "dirs": ["~/books"] },
  "vscode": { "hostname": "0.0.0.0", "portRange": [4700, 4899], "connectionToken": "<自动生成>" },
  "vscodium": { "hostname": "0.0.0.0", "portRange": [4700, 4899], "connectionToken": "<自动生成>" },
  "wetty": { "hostname": "0.0.0.0", "portRange": [4900, 5099], "sshHost": "localhost" }
}
```

- `hostname`:面板监听地址,默认 `0.0.0.0`(外网可访问);仅本机使用改为 `127.0.0.1`
- `passwordHash`:密码的 argon2id 哈希(单向,明文密码永不落盘)
- `usernameEnc`:登录用户名的 AES-256-GCM 密文,密钥由 `sessionSecret` 经 HMAC-SHA256 派生——只防随手翻看配置文件,不防拿到整个文件的人;两者须同步轮转(改密码/用户名时会自动处理)
- `language`:界面与 CLI 语言(`en`/`zh-CN`/`zh-TW`,默认 `en`);`--lang` 参数优先
- `sessionTTLHours`:登录会话有效期(小时,默认 720 = 30 天)
- `compactCards`:卡片紧凑模式(页面顶栏可切换并写回)
- `pageBackground`:页面背景(预设名如 `bg-05`,或导入图片 id)
- `slogan`:顶栏标语;未设置时随界面语言显示默认文案(en/zh-CN/zh-TW),`""` 表示关闭,自定义最长 80 字符(可在「设置」弹窗调整)
- `internalAddress`:内部组网地址(主机 IP 或域名,可含端口);配置后卡片「打开」URL 使用它代替访问面板所用的地址,便于其它设备经内部网络直达服务;留空则沿用当前访问地址(可在「设置」弹窗调整)
- `comfyui.hostname`:ComfyUI 服务器监听地址,默认 `0.0.0.0`(局域网可访问;仅本机用改 `127.0.0.1`)
- `comfyui.portRange`:ComfyUI 服务器端口分配范围,默认 [4100, 4299]
- `opencode.hostname`:opencode web 监听地址,默认 `0.0.0.0`(局域网可访问;仅本机用改 `127.0.0.1`)
- `opencode.portRange`:opencode web 端口分配范围,默认 [4300, 4499]
- `opencode.password`:opencode web 的 basic-auth 密码,首次 serve 自动生成并持久化;显示在 opencode 卡片上,可在浏览器中手动修改(修改后需重启对应实例)
- `mdbook.hostname`:mdBook 服务器监听地址,默认 `0.0.0.0`(局域网可访问;仅本机用改 `127.0.0.1`)
- `mdbook.portRange`:mdBook 服务器端口分配范围,默认 [4500, 4699]
- `mdbook.dirs`:识别 mdBook 项目(含 `book.toml` 的目录)并在其下新建书籍的目录列表;为空时 mdBook 卡片类型整体隐藏
- `vscode.hostname`:VS Code Web 服务器监听地址,默认 `0.0.0.0`(局域网可访问;仅本机用改 `127.0.0.1`)
- `vscode.portRange`:VS Code Web 服务器端口分配范围,默认 [4700, 4899](启动时自动挑空闲端口)
- `vscode.connectionToken`:浏览器访问 VS Code Web 界面时所需的连接令牌,首次 serve 自动生成并持久化;显示在 VS Code 卡片上,可在配置中修改(修改后需重启对应实例)
- `vscodium.hostname` / `vscodium.portRange` / `vscodium.connectionToken`:同上三个配置项,但作用于 **VSCodium** 卡片,与 `vscode` 完全独立,可同时运行
- `wetty.hostname`:WeTTY 监听地址,默认 `0.0.0.0`(局域网可访问;仅本机用改 `127.0.0.1`)
- `wetty.portRange`:WeTTY 端口分配范围,默认 [4900, 5099]
- `wetty.sshHost`:WeTTY 连接的 SSH 服务器(`--ssh-host`),默认 `localhost`(即 narthex 宿主机);改为远端地址可连接远程主机
- `wetty.sshPort`:SSH 服务器端口(`--ssh-port`);省略则用 WeTTY 默认值(22)
- `wetty.sshUser`:默认 SSH 用户名(`--ssh-user`);省略则在浏览器中提示输入。WeTTY **没有 HTTP 层鉴权**,SSH 账号是唯一保护
- 数据文件:状态 `state.json`(按应用类型存卡片,每类至多一张)、每实例日志 `logs/<cardID>.log`、导入图片 `uploads/` 与配置同目录

## 外部访问(默认开启)

默认监听 **`0.0.0.0`**,面板可直接从其它设备访问:

- 本机访问:`http://127.0.0.1:9090`
- 局域网/外网访问:`http://<本机 IP>:9090`(如 `http://192.168.1.100:9090`)

**只在本机使用时,建议收紧**:

```bash
narthex serve --hostname 127.0.0.1           # 临时
# 或 config.json → "hostname": "127.0.0.1"    # 持久
```

**安全警告**(启动时也会打印):

- HTTP 下密码以**明文传输**;外网暴露务必在前面加 HTTPS 反向代理(Caddy/Nginx),代理 9090 端口
- 被管理的桌面应用由本机启动、供本机使用
- macOS 首次局域网访问可能弹出「允许传入连接」,需允许 `narthex`;Linux 注意放行端口
- 暴露后登录限速(5 次/分钟)与强密码是最后防线

## API 摘要

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/login` | 用户名 + 密码登录,下发会话 Cookie |
| POST | `/api/logout` | 退出 |
| GET | `/api/session` | 会话是否有效 + 界面语言(登录页据此显示语言) |
| POST | `/api/account` | 修改用户名/密码(需当前密码),轮转会话密钥 |
| GET | `/api/cards` | 卡片列表(含 running/healthy/memoryKB/uptime/url) |
| POST | `/api/cards` | 创建卡片(`kind`: `comfyui`/`opencode`/`mdbook`/`vscode`/`vscodium`/`wetty`;应用未安装 400,同类重复 409;`mdbook` 还需 `dir` 指向可识别项目) |
| PATCH | `/api/cards/{id}` | 编辑名称/图标/背景 |
| DELETE | `/api/cards/{id}` | 删除(先停应用) |
| POST | `/api/cards/{id}/start` | 启动应用(目录启动时自动检测) |
| POST | `/api/cards/{id}/stop` | 停止应用 |
| GET | `/api/meta` | 图标/背景(预设+导入)、检测到的应用(`apps`,含 installed/label)与当前设置(含 `internalAddress`) |
| GET | `/api/mdbook/projects` | 列出配置的 `mdbook.dirs` 及其下识别到的书籍项目(`{dirs, projects:[{name,path}]}`) |
| POST | `/api/mdbook/projects` | 在配置目录下新建书籍(`{dir, name}`,执行 `mdbook init`) |
| POST | `/api/settings` | 修改设置:`language` / `compact` / `pageBackground` / `slogan` / `internalAddress`,写回配置 |
| GET | `/api/autostart` | 查询 `narthex serve` 的 macOS launchd 自启状态(`{supported, installed, loaded, label, plistPath}`) |
| POST | `/api/autostart` | 安装(`{action:"install"}`)或卸载(`{action:"uninstall"}`)用户级 serve 自启任务 |
| POST | `/api/uploads` | 上传背景图片(multipart `file`,jpg/png/webp ≤15MB) |
| DELETE | `/api/uploads/{id}` | 删除导入图片 |
| GET | `/uploads/<file>` | 访问导入图片(静态) |

除 login/session/logout 外均需会话 Cookie;`/uploads/` 为公开静态文件(随机文件名)。

## 常见问题

- **浏览器输入 `http://0.0.0.0:9090` 打不开/空白?**`0.0.0.0` 是服务端监听地址,不是访问地址;Firefox 等浏览器会直接拒绝它。请用 `http://127.0.0.1:9090`(本机)或 `http://<本机局域网 IP>:9090`(其它设备,如 `http://192.168.0.102:9090`)
- **其它设备访问空白?** 大概率是 macOS 防火墙未放行:系统设置 → 网络 → 防火墙,允许 `narthex` 传入连接;Linux 放行对应端口
- **添加卡片时看不到某类应用?** Narthex 只显示**已安装**的应用。确认对应的桌面应用(`Comfy Desktop.app`)已安装,且位于 `/Applications` 或 `~/Applications`(或通过 `NARTHEX_COMFY_APP_DIR` 指定);`opencode` 则需要 `opencode` CLI 在 PATH 中(或用 `NARTHEX_OPENCODE_BIN` 指定);`mdbook` 需要 `mdbook` CLI 在 PATH 中(或用 `NARTHEX_MDBOOK_BIN` 指定)**且 `mdbook.dirs` 非空**;`vscode` 需要 `code` 在 PATH 中(或用 `NARTHEX_VSCODE_BIN` 指定);`vscodium` 需要 `codium` 在 PATH 中(或用 `NARTHEX_VSCODIUM_BIN` 指定);`wetty` 需要 `wetty` 在 PATH 中(或用 `NARTHEX_WETTY_BIN` 指定);每类应用**只有一张卡片**,已添加的类型不会再出现在列表中
- **ComfyUI 卡片启动失败?** 需要已安装 **ComfyUI Desktop** 且其中至少有一个已安装实例(代码目录含 `main.py`,由 `installations.json` 解析;`NARTHEX_COMFY_DESKTOP_DIR` 可覆盖)。启动的是 ComfyUI **服务器**(`python main.py --port <n> --listen 0.0.0.0`),端口在 `comfyui.portRange`(默认 4100–4299)内自动分配;「打开」按钮本机为 `http://127.0.0.1:<端口>/`,其它设备用 `http://<面板主机>:<端口>/`。需要浏览器能访问的 API/队列请在 ComfyUI 界面中确认已就绪
- **opencode 卡片启动失败?** 需要已安装 **opencode CLI**(`which opencode`,或设置 `NARTHEX_OPENCODE_BIN`)。启动的是 `opencode web --port <n> --hostname 0.0.0.0`,在用户主目录下运行;端口在 `opencode.portRange`(默认 4300–4499)内自动分配。**浏览器访问时输入卡片上显示的密码**(用户名 `opencode`);现代浏览器对 basic auth 的记忆是**按站点**的,首次输入后同一浏览器/设备无需重复输入
- **mdBook 卡片启动失败?** 需要已安装 **mdbook CLI**(`which mdbook`,或设置 `NARTHEX_MDBOOK_BIN`)且配置了 `mdbook.dirs`。卡片指向添加流程中选定/新建的书籍项目(含 `book.toml` 的目录)。启动的是 `mdbook serve --port <n> --hostname 0.0.0.0`,在项目目录下运行;端口在 `mdbook.portRange`(默认 4500–4699)内自动分配。若书籍构建失败,卡片会一直停留在「启动中…」——请查看 `logs/<cardID>.log`
- **VS Code 卡片启动失败?** 需要已安装 **VS Code(`code`)CLI**(或设置 `NARTHEX_VSCODE_BIN`)。启动的是 `code serve-web --host 0.0.0.0 --port <n> --connection-token <token>`,在用户主目录下运行;端口在 `vscode.portRange`(默认 4700–4899)内自动分配。**浏览器访问时输入卡片上显示的连接令牌**即可连接
- **VSCodium 卡片启动失败?** 与 VS Code 相同,但使用 **`codium` CLI**(或 `NARTHEX_VSCODIUM_BIN`):启动 `codium serve-web --host 0.0.0.0 --port <n> --connection-token <token>`;端口在 `vscodium.portRange`(默认 4700–4899)内自动分配。它是独立类型,可与 VS Code 卡片同时运行
- **WeTTY 卡片启动失败?** 需要已安装 **`wetty` CLI**(`npm -g i wetty`,或设置 `NARTHEX_WETTY_BIN`)。启动的是 `wetty --port <n> --host 0.0.0.0 --ssh-host <host>`,在用户主目录下运行;端口在 `wetty.portRange`(默认 4900–5099)内自动分配。浏览器会要求输入 `wetty.sshHost`(默认 `localhost`)的 **SSH 用户名/密码**,因此宿主机需开启 SSH 服务(macOS:系统设置 → 通用 → 共享 → 远程登录)。WeTTY 自身没有 HTTP 登录,任何能访问该端口的人都会看到 SSH 登录提示——可将 `wetty.hostname` 改为 `127.0.0.1` 仅限本机

## 安全说明

- 默认监听 `0.0.0.0`,服务启动时会打印安全警告;仅本机使用建议改回 `127.0.0.1`(见上文)
- HTTP 明文传输密码;公网访问请在面板前加 HTTPS 反向代理(Caddy/Nginx)
- 用户名在配置中加密存储,密码为 argon2id 单向哈希——配置文件落盘泄露不会直接暴露明文密码;但拿到整个配置文件即可自行解密用户名,请像保护密码一样保护 `~/.config/narthex`
- 忘记密码:重新运行 `./build/narthex setup --force`,或 `./build/narthex reset-auth`(可一并重设用户名);正常修改密码请用 `./build/narthex passwd`(会轮转会话密钥)

## 测试

```bash
go test ./...            # 单元测试
make smoke               # 端到端冒烟测试(使用假的 .app 假体,不触碰真实应用)
```

## 素材来源

- 图标:[Lucide](https://lucide.dev),ISC 许可证
- 背景图:[Lorem Picsum](https://picsum.photos)(来自 Unsplash,可免费使用)
- 素材已提交进仓库,构建离线可用;刷新素材运行 `scripts/download-assets.sh`

## 目录结构

```
cmd/narthex/       入口(serve / setup / passwd / reset-auth / autostart 子命令)
internal/api/      HTTP 处理器与卡片业务逻辑
internal/auth/     argon2id 密码、HMAC 会话、限速
internal/autostart/  macOS launchd 安装/卸载/查询(serve)
internal/i18n/     CLI/API 消息目录(en / zh-CN / zh-TW)
internal/procman/  Backend 抽象、本地实例生命周期(ComfyUI 服务器,opencode/mdbook/VS Code/VSCodium/WeTTY web)、.app 检测
internal/secretbox/ 用户名静态加密(AES-256-GCM,密钥派生自 sessionSecret)
internal/server/   路由、鉴权中间件、静态文件
internal/store/    配置与状态持久化
internal/termios/  无回显密码读取
web/               内嵌前端(HTML/CSS/JS/i18n.js + 图标/背景)
scripts/           素材下载、冒烟测试
docs/              用户与 Agent 文档
```