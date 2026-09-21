// Package i18n provides the user-facing message catalogs for the CLI and
// the HTTP API. Supported languages: en (default), zh-CN, zh-TW.
//
// Every user-visible string in cmd/narthex and internal/api must come from
// here; hardcoded strings are a defect (see docs/agents/AGENTS.*).
package i18n

// Lang identifies a supported language.
type Lang string

const (
	EN   Lang = "en"
	ZHCN Lang = "zh-CN"
	ZHTW Lang = "zh-TW"
)

// Parse validates and normalizes a language string.
func Parse(s string) (Lang, bool) {
	switch Lang(s) {
	case EN, ZHCN, ZHTW:
		return Lang(s), true
	}
	return "", false
}

// T returns the message for key in lang, falling back to en. Unknown keys
// are returned verbatim so missing translations are easy to spot.
func T(lang Lang, key string) string {
	if m, ok := messages[key]; ok {
		if s, ok := m[lang]; ok {
			return s
		}
		if s, ok := m[EN]; ok {
			return s
		}
	}
	return key
}

var messages = map[string]map[Lang]string{
	// ---- CLI: usage -------------------------------------------------
	"usage": {
		EN: `narthex %s - lightweight desktop-app launcher

Usage:
  narthex setup    create the config and set the password
  narthex passwd   change the password (invalidates all sessions)
  narthex reset-auth  reset username and password (invalidates all sessions)
  narthex serve    start the web dashboard
  narthex autostart  install/uninstall a launchd job so narthex starts at login (macOS)
  narthex version  print the version

Flags (all commands):
  --config <path>  config file path (default: %s)
  --lang <lang>    language: en | zh-CN | zh-TW

Setup/passwd flags:
  --password <pw>  set the password non-interactively
  --random         generate a random password and print it
  --force          setup only: overwrite an existing config

Serve flags:
  --port <n>       override the configured listen port
  --hostname <h>   override the listen address (0.0.0.0 for LAN access)

Autostart flags:
  <action>         install | uninstall | status
`,
		ZHCN: `narthex %s - 轻量桌面应用启动面板

用法:
  narthex setup    创建配置并设置密码
  narthex passwd   修改密码(使所有会话失效)
  narthex reset-auth 重置账号与密码(使所有会话失效)
  narthex serve    启动 Web 面板
  narthex autostart  安装/卸载 launchd 任务,让 narthex 登录时自启(macOS)
  narthex version  打印版本号

通用参数:
  --config <path>  配置文件路径(默认: %s)
  --lang <lang>    语言: en | zh-CN | zh-TW

setup/passwd 参数:
  --password <pw>  非交互式设置密码
  --random         生成随机密码并打印
  --force          仅 setup:覆盖已有配置

serve 参数:
  --port <n>       覆盖配置中的监听端口
  --hostname <h>   覆盖监听地址(0.0.0.0 供局域网访问)

autostart 参数:
  <action>         install | uninstall | status
`,
		ZHTW: `narthex %s - 輕量桌面應用啟動面板

用法:
  narthex setup    建立設定檔並設定密碼
  narthex passwd   修改密碼(使所有工作階段失效)
  narthex reset-auth 重設帳號與密碼(使所有工作階段失效)
  narthex serve    啟動 Web 面板
  narthex autostart  安裝/解除 launchd 任務,讓 narthex 登入時自啟(macOS)
  narthex version  列印版本號

通用參數:
  --config <path>  設定檔路徑(預設: %s)
  --lang <lang>    語言: en | zh-CN | zh-TW

setup/passwd 參數:
  --password <pw>  非互動式設定密碼
  --random         產生隨機密碼並列印
  --force          僅 setup:覆寫既有設定檔

serve 參數:
  --port <n>       覆寫設定中的監聽連接埠
  --hostname <h>   覆寫監聽位址(0.0.0.0 供區域網路存取)

autostart 參數:
  <action>         install | uninstall | status
`,
	},

	// ---- CLI: prompts -------------------------------------------------
	"prompt.password": {
		EN:   "Set password: ",
		ZHCN: "设置密码: ",
		ZHTW: "設定密碼: ",
	},
	"prompt.passwordAgain": {
		EN:   "Repeat password: ",
		ZHCN: "再次输入: ",
		ZHTW: "再次輸入: ",
	},
	"msg.authReset": {
		EN:   "username and password reset; all sessions invalidated. Restart narthex serve to apply.",
		ZHCN: "账号与密码已重置,所有会话已失效。请重启 narthex serve 使其生效。",
		ZHTW: "帳號與密碼已重設,所有工作階段已失效。請重新啟動 narthex serve 使其生效。",
	},
	"msg.username": {
		EN:   "username: %s",
		ZHCN: "用户名: %s",
		ZHTW: "使用者名稱: %s",
	},

	// ---- CLI/API: errors ----------------------------------------------
	"err.passwordMismatch": {
		EN:   "passwords do not match",
		ZHCN: "两次输入不一致",
		ZHTW: "兩次輸入不一致",
	},
	"err.passwordTooShort": {
		EN:   "password must be at least 6 characters",
		ZHCN: "密码至少需要 6 个字符",
		ZHTW: "密碼至少需要 6 個字元",
	},
	"err.hashPassword": {
		EN:   "failed to hash password: %v",
		ZHCN: "哈希密码失败: %v",
		ZHTW: "密碼雜湊失敗: %v",
	},
	"err.saveConfig": {
		EN:   "failed to save config: %v",
		ZHCN: "保存配置: %v",
		ZHTW: "儲存設定失敗: %v",
	},
	"err.readPassword": {
		EN:   "failed to read password (try --random): %v",
		ZHCN: "读取密码失败(可尝试 --random): %v",
		ZHTW: "讀取密碼失敗(可嘗試 --random): %v",
	},
	"err.loadConfig": {
		EN:   "failed to load config (%s): %v — run narthex setup first",
		ZHCN: "加载配置失败 (%s): %v — 请先运行 narthex setup",
		ZHTW: "載入設定失敗 (%s): %v — 請先執行 narthex setup",
	},
	"err.noPassword": {
		EN:   "no password set, run narthex setup first",
		ZHCN: "未设置密码,请先运行 narthex setup",
		ZHTW: "未設定密碼,請先執行 narthex setup",
	},
	"err.appNotFound": {
		EN:   "the app for this service is not installed (%s) — install it to add or start the card",
		ZHCN: "该服务对应的应用未安装(%s) — 请先安装再添加或启动卡片",
		ZHTW: "此服務對應的應用未安裝(%s) — 請先安裝再新增或啟動卡片",
	},
	"err.duplicateKind": {
		EN:   "a card already exists for %s",
		ZHCN: "已存在 %s 的卡片",
		ZHTW: "已存在 %s 的卡片",
	},
	"err.loadState": {
		EN:   "failed to load state: %v",
		ZHCN: "读取状态失败: %v",
		ZHTW: "讀取狀態失敗: %v",
	},
	"err.configExists": {
		EN:   "config already exists (%s), use --force to overwrite",
		ZHCN: "配置已存在 (%s),使用 --force 覆盖",
		ZHTW: "設定檔已存在 (%s),使用 --force 覆寫",
	},
	"err.portBusy": {
		EN:   "port %s is already in use by another program",
		ZHCN: "端口 %s 已被其它程序占用",
		ZHTW: "連接埠 %s 已被其他程式佔用",
	},
	"err.invalidLang": {
		EN:   "unknown language %q (use en|zh-CN|zh-TW)",
		ZHCN: "未知的语言 %q (可选 en|zh-CN|zh-TW)",
		ZHTW: "未知的語言 %q (可選 en|zh-CN|zh-TW)",
	},
	"err.invalidRequest": {
		EN:   "invalid request",
		ZHCN: "无效的请求",
		ZHTW: "無效的請求",
	},
	"err.credentialsWrong": {
		EN:   "invalid username or password",
		ZHCN: "账号或密码错误",
		ZHTW: "帳號或密碼錯誤",
	},
	"err.usernameInvalid": {
		EN:   "username must be 1-32 characters",
		ZHCN: "用户名不能为空且最长 32 个字符",
		ZHTW: "使用者名稱不能為空且最長 32 個字元",
	},
	"err.usernameDecrypt": {
		EN:   "failed to decrypt the stored username, run narthex reset-auth",
		ZHCN: "解密存储的用户名失败,请运行 narthex reset-auth",
		ZHTW: "解密儲存的使用者名稱失敗,請執行 narthex reset-auth",
	},
	"err.tooManyAttempts": {
		EN:   "too many attempts, try again later",
		ZHCN: "尝试次数过多,请稍后再试",
		ZHTW: "嘗試次數過多,請稍後再試",
	},
	"err.noPasswordConfigured": {
		EN:   "no password configured, run narthex setup first",
		ZHCN: "未配置密码,请先运行 narthex setup",
		ZHTW: "未設定密碼,請先執行 narthex setup",
	},
	"err.saveFailed": {
		EN:   "save failed: %s",
		ZHCN: "保存失败: %s",
		ZHTW: "儲存失敗: %s",
	},
	"err.cardNotFound": {
		EN:   "card not found",
		ZHCN: "卡片不存在",
		ZHTW: "卡片不存在",
	},
	"err.startFailed": {
		EN:   "start failed: %s",
		ZHCN: "启动失败: %s",
		ZHTW: "啟動失敗: %s",
	},
	"err.uploadInvalid": {
		EN:   "invalid upload",
		ZHCN: "无效的上传",
		ZHTW: "無效的上傳",
	},
	"err.uploadTooLarge": {
		EN:   "file too large (max 15 MB)",
		ZHCN: "文件过大(最大 15 MB)",
		ZHTW: "檔案過大(最大 15 MB)",
	},
	"err.uploadType": {
		EN:   "unsupported image type (use jpg/png/webp)",
		ZHCN: "不支持的图片类型(仅支持 jpg/png/webp)",
		ZHTW: "不支援的圖片類型(僅支援 jpg/png/webp)",
	},
	"err.uploadNotFound": {
		EN:   "upload not found",
		ZHCN: "上传不存在",
		ZHTW: "上傳不存在",
	},
	"err.kindUnsupported": {
		EN:   "unsupported service type: %s",
		ZHCN: "不支持的服务类型: %s",
		ZHTW: "不支援的服務類型: %s",
	},
	"err.invalidBackground": {
		EN:   "invalid background",
		ZHCN: "无效的背景",
		ZHTW: "無效的背景",
	},
	"err.gatewayUnavailable": {
		EN:   "opencode is not running",
		ZHCN: "opencode 未运行",
		ZHTW: "opencode 未執行",
	},
	"err.sloganTooLong": {
		EN:   "slogan must be 80 characters or fewer",
		ZHCN: "标语最长 80 个字符",
		ZHTW: "標語最長 80 個字元",
	},
	"err.addressTooLong": {
		EN:   "internal address must be 253 characters or fewer",
		ZHCN: "内部组网地址最长 253 个字符",
		ZHTW: "內部組網地址最長 253 個字元",
	},
	"err.invalidGatewayPort": {
		EN:   "invalid gateway port (1-65535, and not the dashboard port)",
		ZHCN: "无效的网关端口(1-65535,且不能是面板端口)",
		ZHTW: "無效的閘道連接埠(1-65535,且不能是面板連接埠)",
	},
	"err.mdbookNoProject": {
		EN:   "the selected directory is not a recognized mdBook project (create the book first)",
		ZHCN: "所选目录不是可识别的 mdBook 项目(请先创建书籍)",
		ZHTW: "所選目錄不是可識別的 mdBook 專案(請先建立書籍)",
	},
	"err.mdbookInvalidDir": {
		EN:   "the directory is not in the configured mdBook project directories (mdbook.dirs)",
		ZHCN: "该目录不在配置的 mdBook 项目目录列表(mdbook.dirs)中",
		ZHTW: "該目錄不在設定的 mdBook 專案目錄清單(mdbook.dirs)中",
	},
	"err.mdbookNoDirs": {
		EN:   "no mdBook project directories configured (add mdbook.dirs to the config)",
		ZHCN: "未配置 mdBook 项目目录(请在配置中添加 mdbook.dirs)",
		ZHTW: "未設定 mdBook 專案目錄(請在設定中新增 mdbook.dirs)",
	},
	"err.mdbookNameInvalid": {
		EN:   "invalid book name (1-64 characters, no path separators, not hidden)",
		ZHCN: "无效的书籍名称(1-64 个字符,不含路径分隔符,不以点开头)",
		ZHTW: "無效的書籍名稱(1-64 個字元,不含路徑分隔符,不以點開頭)",
	},
	"err.mdbookExists": {
		EN:   "a book with this name already exists in that directory",
		ZHCN: "该目录下已存在同名书籍",
		ZHTW: "該目錄下已存在同名書籍",
	},
	"err.mdbookCreateFailed": {
		EN:   "failed to create the book: %s",
		ZHCN: "创建书籍失败: %s",
		ZHTW: "建立書籍失敗: %s",
	},
	"slogan.default": {
		EN:   "From nothing, to nothing.",
		ZHCN: "万物始于无,亦终于无",
		ZHTW: "萬物始於無,亦終於無",
	},

	// ---- CLI: messages --------------------------------------------------
	"msg.passwordGenerated": {
		EN:   "generated password (keep it safe): %s",
		ZHCN: "已生成密码(请妥善保存): %s",
		ZHTW: "已產生密碼(請妥善保存): %s",
	},
	"msg.opencodePassword": {
		EN:   "OpenCode web password (shown on the OpenCode card): %s",
		ZHCN: "OpenCode web 密码(显示在 OpenCode 卡片上): %s",
		ZHTW: "OpenCode web 密碼(顯示在 OpenCode 卡片上): %s",
	},
	"msg.opencodeGateway": {
		EN:   "opencode gateway on %s",
		ZHCN: "opencode 网关监听 %s",
		ZHTW: "opencode 閘道監聽 %s",
	},
	"msg.vscodeToken": {
		EN:   "VS Code web connection token (shown on the VS Code card): %s",
		ZHCN: "VS Code web 连接令牌(显示在 VS Code 卡片上): %s",
		ZHTW: "VS Code web 連線權杖(顯示在 VS Code 卡片上): %s",
	},
	"msg.vscodiumToken": {
		EN:   "VSCodium web connection token (shown on the VSCodium card): %s",
		ZHCN: "VSCodium web 连接令牌(显示在 VSCodium 卡片上): %s",
		ZHTW: "VSCodium web 連線權杖(顯示在 VSCodium 卡片上): %s",
	},
	"msg.newPasswordGenerated": {
		EN:   "new password (keep it safe): %s",
		ZHCN: "新密码(请妥善保存): %s",
		ZHTW: "新密碼(請妥善保存): %s",
	},
	"msg.configWritten": {
		EN:   "config written to %s",
		ZHCN: "配置已写入 %s",
		ZHTW: "設定檔已寫入 %s",
	},
	"msg.appsDetected": {
		EN:   "detected apps: %s",
		ZHCN: "检测到的应用: %s",
		ZHTW: "偵測到的應用: %s",
	},
	"msg.appsNone": {
		EN:   "none",
		ZHCN: "无",
		ZHTW: "無",
	},
	"msg.firstRunSetup": {
		EN:   "no config found at %s — created one with a random password",
		ZHCN: "未找到配置 %s — 已自动生成一份,并设置了随机密码",
		ZHTW: "未找到設定 %s — 已自動建立一份,並設定隨機密碼",
	},
	"msg.startServer": {
		EN:   "start with: narthex serve --config %s",
		ZHCN: "启动服务: narthex serve --config %s",
		ZHTW: "啟動服務: narthex serve --config %s",
	},
	"msg.passwordUpdated": {
		EN:   "password updated, all sessions invalidated. Restart narthex serve to apply.",
		ZHCN: "密码已更新,所有会话已失效。请重启 narthex serve 使其生效。",
		ZHTW: "密碼已更新,所有工作階段已失效。請重新啟動 narthex serve 使其生效。",
	},
	"msg.listening": {
		EN:   "narthex %s listening on http://%s",
		ZHCN: "narthex %s 正在监听 http://%s",
		ZHTW: "narthex %s 正在監聽 http://%s",
	},
	"msg.alreadyRunning": {
		EN:   "narthex is already running at http://%s, no need to start again",
		ZHCN: "narthex 已在 http://%s 运行,无需重复启动",
		ZHTW: "narthex 已在 http://%s 執行,無需重複啟動",
	},
	"msg.autostartInstalled": {
		EN:   "autostart installed: %s will start at %s (%s)",
		ZHCN: "已设置自启:%s 将在%s启动(%s)",
		ZHTW: "已設定自啟:%s 將在%s啟動(%s)",
	},
	"msg.autostartUninstalled": {
		EN:   "autostart removed for %s",
		ZHCN: "已取消 %s 的自启",
		ZHTW: "已取消 %s 的自啟",
	},
	"msg.autostartStatusOn": {
		EN:   "%s autostart: installed and loaded (plist: %s)",
		ZHCN: "%s 自启:已安装并加载(plist: %s)",
		ZHTW: "%s 自啟:已安裝並載入(plist: %s)",
	},
	"msg.autostartStatusOff": {
		EN:   "%s autostart: not installed",
		ZHCN: "%s 自启:未安装",
		ZHTW: "%s 自啟:未安裝",
	},
	"msg.autostartLogin": {
		EN:   "login",
		ZHCN: "登录",
		ZHTW: "登入",
	},
	"err.autostartMissingAction": {
		EN:   "missing action: install | uninstall | status",
		ZHCN: "缺少操作:install | uninstall | status",
		ZHTW: "缺少操作:install | uninstall | status",
	},
	"err.autostartUnknownAction": {
		EN:   "unknown action %q (use install | uninstall | status)",
		ZHCN: "未知操作 %q (可选 install | uninstall | status)",
		ZHTW: "未知操作 %q (可選 install | uninstall | status)",
	},
	"err.autostartMissingConfig": {
		EN:   "config file not found (%s), run narthex setup first",
		ZHCN: "配置文件不存在 (%s),请先运行 narthex setup",
		ZHTW: "設定檔不存在 (%s),請先執行 narthex setup",
	},
	"err.autostartInstallFailed": {
		EN:   "autostart install failed: %v",
		ZHCN: "自启安装失败:%v",
		ZHTW: "自啟安裝失敗:%v",
	},
	"err.autostartUnsupportedOS": {
		EN:   "autostart is only supported on macOS",
		ZHCN: "自启仅支持 macOS",
		ZHTW: "自啟僅支援 macOS",
	},
	"err.restartUnsupported": {
		EN:   "restart is not available on this host",
		ZHCN: "此主机不支持重启",
		ZHTW: "此主機不支援重啟",
	},
	"warn.insecureHTTP": {
		EN:   "warning: serving HTTP on %s — passwords travel in plaintext; use on trusted networks only",
		ZHCN: "警告: 以 HTTP 监听 %s,密码将以明文传输,仅建议在可信网络使用",
		ZHTW: "警告: 以 HTTP 監聽 %s,密碼將以明文傳輸,僅建議在可信網路使用",
	},
}
