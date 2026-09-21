"use strict";

// Client-side message catalog. Supported languages: en (default), zh-CN,
// zh-TW. Static HTML text uses data-i18n attributes; dynamic strings go
// through t(). Keys must stay in sync with applyI18n consumers.
const I18N = {
  en: {
    "login.subtitle": "Enter your username and password to manage your services",
    "login.placeholder": "Password",
    "login.submit": "Enter",
    "login.errWrong": "Wrong password",
    "login.errTooMany": "Too many attempts, try again in 60 seconds",
    "login.errOffline": "Cannot reach the server or login failed",
    "login.errCredentials": "Invalid username or password",
    "login.username": "Username",


    "topbar.add": "＋ Add card",
    "topbar.logout": "Log out",
    "topbar.menu": "Menu",
    "topbar.lang": "Language",
    "topbar.settings": "Settings",

    "modal.stepKind": "Choose app",
    "modal.kindcomfyuiDesc": "Start a ComfyUI server and use it in the browser",
    "modal.kindopencodeDesc": "Start the OpenCode web interface (AI coding in the browser)",
    "modal.kindmdbookDesc": "Serve an mdBook documentation project in the browser",
    "modal.kindvscodeDesc": "Serve the VS Code web editor in the browser",
    "modal.kindvscodiumDesc": "Serve the VSCodium web editor in the browser",
    "modal.kindwettyDesc": "Open a terminal in the browser over SSH (WeTTY)",
    "modal.projectStep": "Book (project)",
    "modal.projectEmpty": "No mdBook projects found in the configured directories",
    "modal.createProject": "Create new book",
    "modal.bookNameLabel": "Book name",
    "modal.bookNameRequired": "Please enter a book name",
    "modal.projectCreate": "Create",
    "add.reason.notInstalled": "Not installed on this host",
    "add.reason.comfyNoInstall": "ComfyUI Desktop is installed but has no managed ComfyUI instance",
    "add.reason.mdbookNoDirs": "mdbook is installed but no book directory is configured (mdbook.dirs)",
    "add.reason.alreadyAdded": "Already has a card — only one card per app type",
    "modal.next": "Next",
    "modal.backToKinds": "Back to app selection",
    "modal.upload": "Upload image",
    "modal.uploadHint": "jpg / png / webp, up to 15 MB",
    "modal.deleteUpload": "Delete this image",

    "bgModal.title": "Page background",
    "bgModal.presetLabel": "Presets",
    "bgModal.uploadLabel": "Imported",

    "err.uploadFailed": "Upload failed",

    "modal.settingsTitle": "Settings",
    "modal.sloganLabel": "Slogan",
    "modal.sloganShow": "Show slogan",
    "modal.sloganHint": "Leave empty to hide",
    "modal.autostartLabel": "Start at login",
    "modal.autostartHint": "macOS only — installs a user-level launchd job",
    "modal.autostartUnsupported": "Auto-start is only supported on macOS. Use the CLI on this host.",
    "modal.autostartErr": "Auto-start action failed",
    "modal.restartLabel": "Restart Narthex",
    "modal.restartHint": "Reload the server after rebuilding the binary. Running apps stay up.",
    "modal.restartBtn": "Restart now",
    "modal.restarting": "Restarting…",
    "modal.restartFailed": "Restart timed out — reload the page manually.",
    "modal.internalAddressLabel": "Internal network address",
    "modal.internalAddressHint": "Host (IP or hostname) other devices use to reach the services. Leave empty to use the address you opened Narthex from.",
    "modal.gatewayPortLabel": "opencode gateway port",
    "modal.gatewayPortHint": "Fixed local port for the narthex gateway (the opencode \"Open\" address). A free port is used instead when it is occupied. Applied immediately.",

    "topbar.account": "Account",

    "account.title": "Account settings",
    "account.username": "Username",
    "account.currentPassword": "Current password",
    "account.newPassword": "New password",
    "account.confirmPassword": "Confirm new password",
    "account.save": "Save changes",
    "account.back": "Back",
    "account.updated": "Account updated; sessions on other devices have been invalidated.",
    "account.passwordMismatch": "Passwords do not match",


    "slogan.default": "From nothing, to nothing.",

    "empty": "No cards yet. Click \u201cAdd card\u201d to begin.",

    "status.running": "Running",
    "status.starting": "Starting\u2026",
    "status.stopped": "Stopped",
    "card.start": "Start",
    "card.startingBtn": "Starting…",
    "card.stoppingBtn": "Stopping…",
    "card.stop": "Stop",
    "card.open": "Open",
    "card.edit": "Edit",
    "card.delete": "Delete",
    "card.user": "User",
    "card.password": "Password",
    "card.token": "Token",
    "card.copied": "Copied",
    "card.copyHint": "Click to copy",
    "card.api": "API",
    "card.credsLabel": "Credentials",
    "card.statusLabel": "Status",
    "card.typeLabel": "Type",
    "card.uptimeLabel": "Uptime",
    "card.memoryLabel": "Memory",
    "card.dirLabel": "Directory",
    "card.portLabel": "Port",
    "card.addressLabel": "Address",

    "modal.addTitle": "Add card",
    "modal.editTitle": "Edit card",
    "modal.close": "Close",
    "modal.nameLabel": "Name",
    "modal.iconLabel": "Icon",
    "modal.bgLabel": "Background",
    "modal.create": "Create card",
    "modal.save": "Save",
    "modal.cancel": "Cancel",

    "confirm.delete": "Delete card \u201c{0}\u201d?",
    "confirm.stop": "The instance will be stopped.",

    "uptime.s": "{0} sec",
    "uptime.m": "{0} min",
    "uptime.hm": "{0} h {1} min",

    "err.offline": "Cannot reach the server",
    "err.loadFailed": "Failed to load, please refresh the page",
    "err.invalidGatewayPort": "Invalid gateway port (1-65535, and not the dashboard port)",
  },

  "zh-CN": {
    "login.subtitle": "输入用户名与密码以管理你的服务",
    "login.placeholder": "密码",
    "login.submit": "进入",
    "login.errWrong": "密码错误",
    "login.errTooMany": "尝试次数过多,请 60 秒后再试",
    "login.errOffline": "无法连接服务器或登录失败",
    "login.errCredentials": "账号或密码错误",
    "login.username": "用户名",


    "topbar.add": "＋ 添加卡片",
    "topbar.logout": "退出",
    "topbar.menu": "菜单",
    "topbar.lang": "语言",
    "topbar.settings": "设置",

    "modal.stepKind": "选择应用",
    "modal.kindcomfyuiDesc": "启动 ComfyUI 服务,通过浏览器使用",
    "modal.kindopencodeDesc": "启动 OpenCode web 界面,在浏览器中进行 AI 编程",
    "modal.kindmdbookDesc": "在浏览器中托管 mdBook 文档项目",
    "modal.kindvscodeDesc": "在浏览器中使用 VS Code Web 编辑器",
    "modal.kindvscodiumDesc": "在浏览器中使用 VSCodium Web 编辑器",
    "modal.kindwettyDesc": "通过 SSH 在浏览器中使用终端(WeTTY)",
    "modal.projectStep": "书籍(项目)",
    "modal.projectEmpty": "在配置的目录中未找到 mdBook 项目",
    "modal.createProject": "新建书籍",
    "modal.bookNameLabel": "书籍名称",
    "modal.bookNameRequired": "请输入书籍名称",
    "modal.projectCreate": "创建",
    "add.reason.notInstalled": "本机未安装",
    "add.reason.comfyNoInstall": "已安装 ComfyUI Desktop,但没有可用的 ComfyUI 实例",
    "add.reason.mdbookNoDirs": "已安装 mdbook,但未配置书籍目录(mdbook.dirs)",
    "add.reason.alreadyAdded": "已添加卡片 — 每类应用仅一张",
    "modal.next": "下一步",
    "modal.backToKinds": "返回选择应用",
    "modal.upload": "上传图片",
    "modal.uploadHint": "jpg / png / webp,最大 15 MB",
    "modal.deleteUpload": "删除此图",

    "bgModal.title": "页面背景",
    "bgModal.presetLabel": "预设",
    "bgModal.uploadLabel": "已导入",

    "err.uploadFailed": "上传失败",

    "modal.settingsTitle": "设置",
    "modal.sloganLabel": "标语",
    "modal.sloganShow": "显示标语",
    "modal.sloganHint": "留空则隐藏",
    "modal.autostartLabel": "登录时自启",
    "modal.autostartHint": "仅 macOS — 安装用户级 launchd 任务",
    "modal.autostartUnsupported": "自启仅支持 macOS。此主机请使用命令行。",
    "modal.autostartErr": "自启操作失败",
    "modal.restartLabel": "重启 Narthex",
    "modal.restartHint": "重新构建二进制后可在此重启服务。正在运行的应用不受影响。",
    "modal.restartBtn": "立即重启",
    "modal.restarting": "正在重启…",
    "modal.restartFailed": "重启超时——请手动刷新页面。",
    "modal.internalAddressLabel": "内部组网地址",
    "modal.internalAddressHint": "其它设备访问服务所用的主机(IP 或域名)。留空则沿用你打开 Narthex 时所用的地址。",
    "modal.gatewayPortLabel": "opencode 网关端口",
    "modal.gatewayPortHint": "narthex 网关(opencode「打开」地址)使用的固定本地端口;被占用时自动改用空闲端口,修改即时生效。",

    "topbar.account": "账号",

    "account.title": "账号设置",
    "account.username": "用户名",
    "account.currentPassword": "当前密码",
    "account.newPassword": "新密码",
    "account.confirmPassword": "确认新密码",
    "account.save": "保存修改",
    "account.back": "返回",
    "account.updated": "账号已更新,其它设备会话已失效。",
    "account.passwordMismatch": "两次输入不一致",


    "slogan.default": "万物始于无,亦终于无",

    "empty": "还没有卡片,点击「添加卡片」开始",

    "status.running": "运行中",
    "status.starting": "启动中…",
    "status.stopped": "已停止",
    "card.start": "启动",
    "card.startingBtn": "启动中…",
    "card.stoppingBtn": "停止中…",
    "card.stop": "停止",
    "card.open": "打开",
    "card.edit": "编辑",
    "card.delete": "删除",
    "card.user": "用户名",
    "card.password": "密码",
    "card.token": "令牌",
    "card.copied": "已复制",
    "card.copyHint": "点击复制",
    "card.api": "API",
    "card.credsLabel": "凭据",
    "card.statusLabel": "状态",
    "card.typeLabel": "类型",
    "card.uptimeLabel": "运行时长",
    "card.memoryLabel": "内存",
    "card.dirLabel": "目录",
    "card.portLabel": "端口",
    "card.addressLabel": "地址",

    "modal.addTitle": "添加卡片",
    "modal.editTitle": "编辑卡片",
    "modal.close": "关闭",
    "modal.nameLabel": "名称",
    "modal.iconLabel": "图标",
    "modal.bgLabel": "背景",
    "modal.create": "创建卡片",
    "modal.save": "保存",
    "modal.cancel": "取消",

    "confirm.delete": "删除卡片「{0}」?",
    "confirm.stop": "实例将被停止。",

    "uptime.s": "{0} 秒",
    "uptime.m": "{0} 分钟",
    "uptime.hm": "{0} 小时 {1} 分",

    "err.offline": "无法连接服务器",
    "err.loadFailed": "加载失败,请刷新页面",
    "err.invalidGatewayPort": "无效的网关端口(1-65535,且不能是面板端口)",
  },

  "zh-TW": {
    "login.subtitle": "輸入使用者名稱與密碼以管理你的服務",
    "login.placeholder": "密碼",
    "login.submit": "進入",
    "login.errWrong": "密碼錯誤",
    "login.errTooMany": "嘗試次數過多,請 60 秒後再試",
    "login.errOffline": "無法連接伺服器或登入失敗",
    "login.errCredentials": "帳號或密碼錯誤",
    "login.username": "使用者名稱",


    "topbar.add": "＋ 新增卡片",
    "topbar.logout": "登出",
    "topbar.menu": "選單",
    "topbar.lang": "語言",
    "topbar.settings": "設定",

    "modal.stepKind": "選擇應用",
    "modal.kindcomfyuiDesc": "啟動 ComfyUI 服務,透過瀏覽器使用",
    "modal.kindopencodeDesc": "啟動 OpenCode web 介面,在瀏覽器中進行 AI 程式設計",
    "modal.kindmdbookDesc": "在瀏覽器中託管 mdBook 文件專案",
    "modal.kindvscodeDesc": "在瀏覽器中使用 VS Code Web 編輯器",
    "modal.kindvscodiumDesc": "在瀏覽器中使用 VSCodium Web 編輯器",
    "modal.kindwettyDesc": "透過 SSH 在瀏覽器中使用終端機(WeTTY)",
    "modal.projectStep": "書籍(專案)",
    "modal.projectEmpty": "在設定的目錄中找不到 mdBook 專案",
    "modal.createProject": "新增書籍",
    "modal.bookNameLabel": "書籍名稱",
    "modal.bookNameRequired": "請輸入書籍名稱",
    "modal.projectCreate": "建立",
    "add.reason.notInstalled": "本機未安裝",
    "add.reason.comfyNoInstall": "已安裝 ComfyUI Desktop,但沒有可用的 ComfyUI 執行個體",
    "add.reason.mdbookNoDirs": "已安裝 mdbook,但未設定書籍目錄(mdbook.dirs)",
    "add.reason.alreadyAdded": "已新增卡片 — 每類應用僅一張",
    "modal.next": "下一步",
    "modal.backToKinds": "返回選擇應用",
    "modal.upload": "上傳圖片",
    "modal.uploadHint": "jpg / png / webp,最大 15 MB",
    "modal.deleteUpload": "刪除此圖片",

    "bgModal.title": "頁面背景",
    "bgModal.presetLabel": "預設",
    "bgModal.uploadLabel": "已匯入",

    "err.uploadFailed": "上傳失敗",

    "modal.settingsTitle": "設定",
    "modal.sloganLabel": "標語",
    "modal.sloganShow": "顯示標語",
    "modal.sloganHint": "留空則隱藏",
    "modal.autostartLabel": "登入時自啟",
    "modal.autostartHint": "僅 macOS — 安裝使用者層級 launchd 任務",
    "modal.autostartUnsupported": "自啟僅支援 macOS。此主機請使用命令列。",
    "modal.autostartErr": "自啟操作失敗",
    "modal.restartLabel": "重啟 Narthex",
    "modal.restartHint": "重新建置二進位檔後可在此重啟服務。正在執行的應用不受影響。",
    "modal.restartBtn": "立即重啟",
    "modal.restarting": "正在重啟…",
    "modal.restartFailed": "重啟逾時——請手動重新整理頁面。",
    "modal.internalAddressLabel": "內部組網地址",
    "modal.internalAddressHint": "其他裝置存取服務所用的主機(IP 或網域名稱)。留空則沿用你開啟 Narthex 時所用的位址。",
    "modal.gatewayPortLabel": "opencode 閘道連接埠",
    "modal.gatewayPortHint": "narthex 閘道(opencode「開啟」位址)使用的固定本機連接埠;被占用時自動改用空閒連接埠,修改即時生效。",

    "topbar.account": "帳號",

    "account.title": "帳號設定",
    "account.username": "使用者名稱",
    "account.currentPassword": "目前密碼",
    "account.newPassword": "新密碼",
    "account.confirmPassword": "確認新密碼",
    "account.save": "儲存變更",
    "account.back": "返回",
    "account.updated": "帳號已更新,其他裝置的工作階段已失效。",
    "account.passwordMismatch": "兩次輸入不一致",


    "slogan.default": "萬物始於無,亦終於無",

    "empty": "還沒有卡片,點選「新增卡片」開始",

    "status.running": "執行中",
    "status.starting": "啟動中…",
    "status.stopped": "已停止",
    "card.start": "啟動",
    "card.startingBtn": "啟動中…",
    "card.stoppingBtn": "停止中…",
    "card.stop": "停止",
    "card.open": "開啟",
    "card.edit": "編輯",
    "card.delete": "刪除",
    "card.user": "使用者",
    "card.password": "密碼",
    "card.token": "權杖",
    "card.copied": "已複製",
    "card.copyHint": "點擊複製",
    "card.api": "API",
    "card.credsLabel": "憑證",
    "card.statusLabel": "狀態",
    "card.typeLabel": "類型",
    "card.uptimeLabel": "執行時長",
    "card.memoryLabel": "記憶體",
    "card.dirLabel": "目錄",
    "card.portLabel": "連接埠",
    "card.addressLabel": "位址",

    "modal.addTitle": "新增卡片",
    "modal.editTitle": "編輯卡片",
    "modal.close": "關閉",
    "modal.nameLabel": "名稱",
    "modal.iconLabel": "圖示",
    "modal.bgLabel": "背景",
    "modal.create": "建立卡片",
    "modal.save": "儲存",
    "modal.cancel": "取消",

    "confirm.delete": "刪除卡片「{0}」?",
    "confirm.stop": "執行個體將被停止。",

    "uptime.s": "{0} 秒",
    "uptime.m": "{0} 分鐘",
    "uptime.hm": "{0} 小時 {1} 分",

    "err.offline": "無法連接伺服器",
    "err.loadFailed": "載入失敗,請重新整理頁面",
    "err.invalidGatewayPort": "無效的閘道連接埠(1-65535,且不能是面板連接埠)",
  },
};

let currentLang = "en";

function t(key) {
  const dict = I18N[currentLang] || I18N.en;
  return dict[key] ?? I18N.en[key] ?? key;
}

function fmtTpl(s, ...args) {
  return String(s).replace(/\{(\d+)\}/g, (_, i) => (args[i] !== undefined ? args[i] : ""));
}

function applyI18n(lang) {
  if (!I18N[lang]) lang = "en";
  currentLang = lang;
  document.documentElement.lang = lang;
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    el.textContent = t(el.dataset.i18n);
  });
  document.querySelectorAll("[data-i18n-placeholder]").forEach((el) => {
    el.placeholder = t(el.dataset.i18nPlaceholder);
  });
  document.querySelectorAll("[data-i18n-title]").forEach((el) => {
    el.title = t(el.dataset.i18nTitle);
  });
  if (window.onLangChanged) window.onLangChanged(lang);
}
