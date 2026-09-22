"use strict";

const $ = (sel, root = document) => root.querySelector(sel);

const loginView = $("#login-view");
const loginForm = $("#login-form");
const usernameInput = $("#username");
const passwordInput = $("#password");
const loginError = $("#login-error");
const loginBtn = $("#login-btn");
const appView = $("#app-view");
const accountView = $("#account-view");
const accountForm = $("#account-form");
const acctUsername = $("#acct-username");
const acctCurPw = $("#acct-cur-pw");
const acctNewPw = $("#acct-new-pw");
const acctConfirmPw = $("#acct-confirm-pw");
const acctMsg = $("#acct-msg");
const acctSaveBtn = $("#btn-account-save");
const acctBackBtn = $("#btn-account-back");
const grid = $("#grid");
const empty = $("#empty");
const btnAdd = $("#btn-add");
const btnLogout = $("#btn-logout");
const btnSettings = $("#btn-settings");
const btnAccount = $("#btn-account");
const btnMenu = $("#btn-menu");
const menuIcon = $("#menu-icon");
const topbar = $(".topbar");
const topbarActions = $("#topbar-actions");
const sloganEl = $("#slogan");
const modalRoot = $("#modal-root");
const bgEl = $("#bg");

let meta = { icons: [], backgrounds: [], apps: {}, lang: "en", pageBackground: "bg-05", slogan: "", internalAddress: "", gatewayPort: 0 };
let cardEls = new Map();
let bgMap = new Map();
let lastCards = [];

/* ---------- remembered username ---------- */

const REMEMBERED_USERNAME = "narthex.username";

function rememberUsername(name) {
  try {
    if (name) localStorage.setItem(REMEMBERED_USERNAME, name);
  } catch (e) { /* storage unavailable (private mode) */ }
}

function rememberedUsername() {
  try { return localStorage.getItem(REMEMBERED_USERNAME) || ""; } catch (e) { return ""; }
}

/* ---------- api ---------- */

async function api(path, opts = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...opts,
  });
  if ((res.status === 401 || res.status === 403) && !path.endsWith("/login") && !path.endsWith("/session")) {
    showLogin(true);
    throw new Error("unauthorized");
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(data.error || res.statusText);
    err.status = res.status;
    throw err;
  }
  return data;
}

async function uploadImage(file) {
  const fd = new FormData();
  fd.append("file", file);
  const res = await fetch("/api/uploads", { method: "POST", body: fd });
  if (res.status === 401 || res.status === 403) {
    showLogin(true);
    throw new Error("unauthorized");
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(data.error || res.statusText);
    err.status = res.status;
    throw err;
  }
  return data;
}

async function refreshMeta() {
  meta = await api("/api/meta");
  bgMap = new Map((meta.backgrounds || []).map((b) => [b.id, b.url]));
}

/* ---------- view switching ---------- */

// showLogin returns to the login view. expired=true shows a "session
// expired" notice (used when a live session is invalidated, not on a fresh
// visit or a manual logout). It resets every other view so the page is
// never left blank.
function showLogin(expired = false) {
  polling = false;
  const wasHidden = loginView.hidden;
  appView.hidden = true;
  accountView.hidden = true;
  modalRoot.hidden = true;
  loginView.hidden = false;
  if (wasHidden) {
    usernameInput.value = rememberedUsername();
    passwordInput.value = "";
  }
  if (expired) {
    loginError.hidden = false;
    loginError.textContent = t("login.sessionExpired");
  } else {
    loginError.hidden = true;
  }
}

function showApp() {
  loginView.hidden = true;
  accountView.hidden = true;
  appView.hidden = false;
}

/* ---------- formatting ---------- */

function fmtMem(kb) {
  if (!kb || kb <= 0) return "";
  if (kb >= 1024 * 1024) return (kb / 1024 / 1024).toFixed(1) + " GB";
  return (kb / 1024).toFixed(0) + " MB";
}

function fmtUptime(sec) {
  if (!sec || sec < 0) return "";
  if (sec < 60) return fmtTpl(t("uptime.s"), sec);
  if (sec < 3600) return fmtTpl(t("uptime.m"), Math.floor(sec / 60));
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  return fmtTpl(t("uptime.hm"), h, m);
}

// copyText copies value to the clipboard and shows brief feedback on the
// clicked row (a label swap) and a toast.
function copyText(value, row) {
  const ok = () => {
    if (row) {
      const label = row.querySelector(".pw-label");
      const prev = label.dataset.prev || label.textContent;
      label.dataset.prev = prev;
      label.textContent = t("card.copied");
      setTimeout(() => {
        label.textContent = label.dataset.prev || "";
      }, 1200);
    }
  };
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(value).then(ok).catch(() => fallbackCopy(value, ok));
  } else {
    fallbackCopy(value, ok);
  }
}

function fallbackCopy(value, done) {
  const ta = document.createElement("textarea");
  ta.value = value;
  ta.style.position = "fixed";
  ta.style.opacity = "0";
  document.body.appendChild(ta);
  ta.select();
  try {
    document.execCommand("copy");
  } catch (e) {
    /* clipboard unavailable */
  }
  document.body.removeChild(ta);
  done();
}

/* ---------- background helpers ---------- */

const BG_CACHE = "narthex.background";

function bgUrl(id) {
  return bgMap.get(id) || `/assets/backgrounds/${id}.jpg`;
}

// cachedPageBg returns the last rendered background ({id,url}) so the page
// can paint it immediately, before the session request returns.
function cachedPageBg() {
  try { return JSON.parse(localStorage.getItem(BG_CACHE) || "null"); } catch (e) { return null; }
}

function rememberPageBg(id, url) {
  try { localStorage.setItem(BG_CACHE, JSON.stringify({ id, url })); } catch (e) { /* storage unavailable */ }
}

// applyPageBgUrl sets the shared #bg element (login and dashboard use the
// same one) and preloads the image so the browser caches it.
function applyPageBgUrl(url) {
  if (!url) return;
  bgEl.style.backgroundImage =
    `linear-gradient(rgba(8, 10, 16, 0.10), rgba(8, 10, 16, 0.30)), url("${url}")`;
  const img = new Image();
  img.src = url;
}

function applyPageBg(id) {
  const url = bgUrl(id);
  applyPageBgUrl(url);
  rememberPageBg(id, url);
}

// applySessionBg applies the background from /api/session, which carries a
// resolved URL (uploads included) so the login page can share the
// dashboard's background before the visitor is authenticated.
function applySessionBg(data) {
  const url = data.pageBackgroundUrl || bgUrl(data.pageBackground || "bg-05");
  applyPageBgUrl(url);
  if (data.pageBackground) rememberPageBg(data.pageBackground, url);
}

function updateSlogan() {
  sloganEl.hidden = !meta.slogan;
  sloganEl.textContent = meta.slogan || "";
}

/* ---------- cards ---------- */

// isTokenKind reports whether a kind authenticates with a single
// connection token (no username) instead of basic-auth credentials.
function isTokenKind(kind) {
  return kind === "vscode" || kind === "vscodium";
}

// credentialRows builds the copyable credential rows of a card. VS Code
// and VSCodium show a single "Token" row (the server has no username);
// opencode shows a username, a password and the direct API base URL (for
// remote clients such as `opencode attach` or the SDK).
function credentialRows(card) {
  const rows = [];
  if (card.username) {
    rows.push(`<button type="button" class="pw-row" data-copy="${esc(card.username)}"><span class="pw-label">${t("card.user")}</span><code>${esc(card.username)}</code></button>`);
  }
  if (card.password) {
    const label = isTokenKind(card.kind) ? t("card.token") : t("card.password");
    rows.push(`<button type="button" class="pw-row" data-copy="${esc(card.password)}"><span class="pw-label">${label}</span><code>${esc(card.password)}</code></button>`);
  }
  if (card.apiUrl) {
    rows.push(`<button type="button" class="pw-row" data-copy="${esc(card.apiUrl)}"><span class="pw-label">${t("card.api")}</span><code>${esc(card.apiUrl)}</code></button>`);
  }
  return rows.join("");
}

function cardEl(card) {
  const el = document.createElement("article");
  el.className = "card";
  el.dataset.id = card.id;
  el.innerHTML = `
    <div class="card-cover">
      <img class="cover-img" src="${bgUrl(card.background)}" alt="" draggable="false">
      <img class="card-icon" src="/assets/icons/${card.icon}.svg" alt="" draggable="false">
    </div>
    <div class="card-body">
      <div class="card-name" title="${esc(card.name)}">${esc(card.name)}</div>
      <div class="status-row">
        <span class="dot"></span>
        <span class="status-spin spinner" hidden></span>
        <span class="status-text"></span>
        <span class="status-mem"></span>
        <span class="status-uptime"></span>
      </div>
      <div class="card-actions">
        <button class="btn start-btn"></button>
        <a class="btn open-btn" target="_blank" rel="noopener"></a>
      </div>
    </div>`;

  const refs = {
    root: el,
    icon: $(".card-icon", el),
    cover: $(".cover-img", el),
    name: $(".card-name", el),
    dot: $(".dot", el),
    statusSpin: $(".status-spin", el),
    statusText: $(".status-text", el),
    mem: $(".status-mem", el),
    uptime: $(".status-uptime", el),
    startBtn: $(".start-btn", el),
    openBtn: $(".open-btn", el),
  };

  refs.startBtn.addEventListener("click", () => toggleCard(card.id, refs.card ? refs.card.running : card.running));
  refs.root.addEventListener("click", (e) => {
    if (e.target.closest("button, a")) return;
    openCardModal(refs.card || card);
  });

  cardEls.set(card.id, refs);
  updateCardEl(refs, card);
  return el;
}

function updateCardEl(r, card) {
  r.card = card;
  r.icon.src = `/assets/icons/${card.icon}.svg`;
  r.cover.src = bgUrl(card.background);
  r.name.textContent = card.name;
  r.name.title = card.name;

  r.dot.classList.toggle("running", card.running);
  r.root.classList.toggle("running", card.running);
  r.statusText.textContent = card.running
    ? (card.healthy ? t("status.running") : t("status.starting"))
    : t("status.stopped");
  r.mem.textContent = card.running ? fmtMem(card.memoryKB) : "";
  r.uptime.textContent = card.running ? fmtUptime(card.uptime) : "";

  if (card.running && !card.healthy) {
    r.statusSpin.hidden = false;
    r.startBtn.className = "btn stop";
    r.startBtn.textContent = t("card.stop");
    r.startBtn.disabled = false;
    r.openBtn.className = "btn open-btn";
    r.openBtn.href = card.url || "#";
    r.openBtn.textContent = t("card.open");
    r.openBtn.style.display = "";
    r.openBtn.disabled = true;
    r.openBtn.title = t("status.starting");
    return;
  }
  r.statusSpin.hidden = true;

  if (card.running) {
    r.startBtn.className = "btn stop";
    r.startBtn.textContent = t("card.stop");
    r.startBtn.disabled = false;
    r.openBtn.className = "btn open-btn";
    r.openBtn.href = card.url || "#";
    r.openBtn.textContent = t("card.open");
    r.openBtn.style.display = "";
    r.openBtn.disabled = !card.healthy;
    r.openBtn.title = "";
  } else {
    r.startBtn.className = "btn start";
    r.startBtn.textContent = t("card.start");
    r.startBtn.disabled = false;
    r.openBtn.style.display = "none";
    r.openBtn.href = "#";
  }
}

function render(cards) {
  const seen = new Set();
  for (const card of cards) {
    seen.add(card.id);
    let refs = cardEls.get(card.id);
    if (!refs) {
      grid.appendChild(cardEl(card));
    } else {
      updateCardEl(refs, card);
    }
  }
  for (const [id, refs] of cardEls) {
    if (!seen.has(id)) {
      refs.root.remove();
      cardEls.delete(id);
    }
  }
  // Keep the DOM order in sync with the server order. Only touch the DOM
  // when it actually differs so the rise animation is not replayed on
  // every poll.
  const want = cards.map((c) => c.id).filter((id) => cardEls.has(id));
  const have = Array.from(grid.children)
    .filter((el) => el.classList.contains("card") && el.dataset.id)
    .map((el) => el.dataset.id);
  if (!activeDrag && !orderPending && want.length === have.length && want.some((id, i) => id !== have[i])) {
    for (const id of want) grid.appendChild(cardEls.get(id).root);
  }
  empty.hidden = cards.length > 0;
}

async function loadCards() {
  const seq = ++cardsReqSeq;
  try {
    const data = await api("/api/cards");
    if (seq !== cardsReqSeq) return; // superseded by a newer request
    lastCards = data.cards || [];
    render(lastCards);
  } catch (e) { /* unauthorized handled by api() */ }
}

// Poll every 2 s while an instance is starting, 5 s otherwise, so start
// and stop feedback appears quickly. Only runs while authenticated.
let polling = false;

async function poll() {
  if (!polling) return;
  if (activeDrag || orderPending) {
    setTimeout(poll, 1000);
    return;
  }
  await loadCards();
  const busy = lastCards.some((c) => c.running && !c.healthy);
  setTimeout(poll, busy ? 2000 : 5000);
}

function alertErr(e) {
  alert(e.status ? e.message : t("err.offline"));
}

async function toggleCard(id, running) {
  const refs = cardEls.get(id);
  if (refs) {
    refs.startBtn.disabled = true;
    refs.startBtn.innerHTML =
      `<span class="spinner"></span><span>${t(running ? "card.stoppingBtn" : "card.startingBtn")}</span>`;
  }
  try {
    await api(`/api/cards/${id}/${running ? "stop" : "start"}`, { method: "POST" });
  } catch (e) {
    alertErr(e);
  }
  await loadCards();
}

async function removeCard(card) {
  if (!confirm(fmtTpl(t("confirm.delete"), card.name))) return;
  try {
    await api(`/api/cards/${card.id}`, { method: "DELETE" });
  } catch (e) {
    alertErr(e);
  }
  await loadCards();
}

/* ---------- card drag sorting ---------- */

// Cards can be reordered by dragging. A single pointer-events state machine
// covers mouse, pen and touch: on mouse/pen the drag starts once the pointer
// moves past a small threshold; on touch it starts after a short long-press
// so the page still scrolls normally. While dragging, a fixed-position ghost
// follows the pointer and the real card is moved through the DOM as a
// placeholder; on release the DOM order is persisted.

const DRAG_LONG_PRESS_MS = 300;
const DRAG_MOVE_THRESHOLD = 6;
const DRAG_TOUCH_CANCEL_DIST = 12;

let pendingDrag = null;
let activeDrag = null;
let orderPending = false;
let cardsReqSeq = 0;
let suppressCardClick = false;

function cardOrder() {
  return Array.from(grid.children)
    .filter((el) => el.classList.contains("card") && el.dataset.id)
    .map((el) => el.dataset.id);
}

function gridColumns() {
  const cols = getComputedStyle(grid).gridTemplateColumns;
  if (!cols || cols === "none") return 1;
  return cols.split(" ").filter(Boolean).length || 1;
}

async function persistCardOrder(ids) {
  if (!ids.length) return;
  // Block polling and order-sync until the server confirms the new order,
  // otherwise a GET issued before the POST commits would revert the grid.
  orderPending = true;
  cardsReqSeq++; // drop any in-flight list request carrying the old order
  try {
    await api("/api/cards/reorder", { method: "POST", body: JSON.stringify({ ids }) });
    // Reorder the cached list too, so the next render keeps the new order
    // even before the following poll refreshes card status.
    const byId = new Map(lastCards.map((c) => [c.id, c]));
    const reordered = ids.map((id) => byId.get(id)).filter(Boolean);
    for (const c of lastCards) if (!reordered.includes(c)) reordered.push(c);
    lastCards = reordered;
  } catch (e) {
    orderPending = false;
    alertErr(e);
    await loadCards(); // fall back to the server's order
    return;
  }
  orderPending = false;
}

function beginDrag(e) {
  const p = pendingDrag;
  if (!p) return;
  pendingDrag = null;
  if (p.timer) clearTimeout(p.timer);

  const el = p.el;
  const rect = el.getBoundingClientRect();
  const ghost = el.cloneNode(true);
  ghost.removeAttribute("data-id");
  ghost.classList.add("card-drag-ghost");
  ghost.style.width = `${rect.width}px`;
  ghost.style.height = `${rect.height}px`;
  ghost.style.left = `${rect.left}px`;
  ghost.style.top = `${rect.top}px`;
  document.body.appendChild(ghost);

  el.classList.add("card-dragging");
  document.body.classList.add("card-drag-active");

  activeDrag = {
    el,
    ghost,
    pointerId: p.pointerId,
    offsetX: e.clientX - rect.left,
    offsetY: e.clientY - rect.top,
    multiColumn: gridColumns() > 1,
  };
  suppressCardClick = true;
  try { el.setPointerCapture(p.pointerId); } catch (err) { /* not supported */ }
  moveDrag(e);
}

function moveDrag(e) {
  const d = activeDrag;
  if (!d || e.pointerId !== d.pointerId) return;
  if (e.cancelable) e.preventDefault();
  d.ghost.style.left = `${e.clientX - d.offsetX}px`;
  d.ghost.style.top = `${e.clientY - d.offsetY}px`;

  const under = document.elementFromPoint(e.clientX, e.clientY);
  const target = under && under.closest ? under.closest(".card") : null;
  if (!target || target === d.el || !grid.contains(target)) return;
  const rect = target.getBoundingClientRect();
  // In a multi-column grid the horizontal position within the target card
  // decides the slot; in a single-column (phone) layout it is vertical.
  const before = d.multiColumn
    ? e.clientX < rect.left + rect.width / 2
    : e.clientY < rect.top + rect.height / 2;
  if (before) target.before(d.el);
  else target.after(d.el);
}

function cancelPendingDrag() {
  if (pendingDrag && pendingDrag.timer) clearTimeout(pendingDrag.timer);
  pendingDrag = null;
}

function finishDrag() {
  const d = activeDrag;
  if (!d) return;
  activeDrag = null;
  d.el.classList.remove("card-dragging");
  document.body.classList.remove("card-drag-active");
  d.ghost.remove();
  try { d.el.releasePointerCapture(d.pointerId); } catch (err) { /* not supported */ }
  persistCardOrder(cardOrder());
  setTimeout(() => { suppressCardClick = false; }, 400);
}

grid.addEventListener("pointerdown", (e) => {
  if (activeDrag || pendingDrag) return;
  if (e.pointerType === "mouse" && e.button !== 0) return;
  if (e.target.closest("button, a")) return;
  const el = e.target.closest(".card");
  if (!el || !grid.contains(el) || !el.dataset.id) return;

  pendingDrag = {
    el,
    pointerId: e.pointerId,
    startX: e.clientX,
    startY: e.clientY,
    touch: e.pointerType === "touch",
    timer: null,
  };
  if (pendingDrag.touch) {
    pendingDrag.timer = setTimeout(() => beginDrag(e), DRAG_LONG_PRESS_MS);
  }
});

document.addEventListener("pointermove", (e) => {
  if (activeDrag) { moveDrag(e); return; }
  if (!pendingDrag || e.pointerId !== pendingDrag.pointerId) return;
  const dx = e.clientX - pendingDrag.startX;
  const dy = e.clientY - pendingDrag.startY;
  const dist = Math.hypot(dx, dy);
  if (pendingDrag.touch) {
    if (dist > DRAG_TOUCH_CANCEL_DIST) cancelPendingDrag();
  } else if (dist > DRAG_MOVE_THRESHOLD) {
    beginDrag(e);
  }
}, { passive: false });

document.addEventListener("pointerup", (e) => {
  if (activeDrag && e.pointerId === activeDrag.pointerId) finishDrag();
  else if (pendingDrag && e.pointerId === pendingDrag.pointerId) cancelPendingDrag();
});

document.addEventListener("pointercancel", (e) => {
  if (activeDrag && e.pointerId === activeDrag.pointerId) finishDrag();
  else if (pendingDrag && e.pointerId === pendingDrag.pointerId) cancelPendingDrag();
});

// Touch Events still fire alongside Pointer Events; preventing the default on
// touchmove is what actually stops the page from scrolling once a touch drag
// is underway (changing touch-action mid-gesture has no effect).
grid.addEventListener("touchmove", (e) => {
  if (activeDrag && e.cancelable) e.preventDefault();
}, { passive: false });

grid.addEventListener("contextmenu", (e) => {
  if (activeDrag) e.preventDefault();
});

// A drag ends with a click event; swallow it so the card modal does not open.
document.addEventListener("click", (e) => {
  if (!suppressCardClick) return;
  suppressCardClick = false;
  e.stopPropagation();
  e.preventDefault();
}, true);

/* ---------- modal ---------- */

function openModal(content) {
  modalRoot.innerHTML = "";
  const modal = document.createElement("div");
  modal.className = "modal";
  modal.innerHTML = content;
  modalRoot.appendChild(modal);
  modalRoot.hidden = false;

  const close = $(".modal-close", modal);
  if (close) close.addEventListener("click", closeModal);
  modalRoot.addEventListener("click", (e) => {
    if (e.target === modalRoot) closeModal();
  });
  const onKey = (e) => {
    if (e.key === "Escape") { closeModal(); document.removeEventListener("keydown", onKey); }
  };
  document.addEventListener("keydown", onKey);
  return modal;
}

function closeModal() {
  modalRoot.hidden = true;
  modalRoot.innerHTML = "";
}

/* ---------- card detail modal ---------- */

// detailRows builds the non-secret info rows of the detail modal, skipping
// any field the card does not carry.
function detailRows(card) {
  const rows = [];
  const status = card.running
    ? (card.healthy ? t("status.running") : t("status.starting"))
    : t("status.stopped");
  rows.push(`<div class="detail-row"><span class="detail-label">${t("card.statusLabel")}</span><span class="detail-value"><span class="dot${card.running ? " running" : ""}"></span>${esc(status)}</span></div>`);

  const app = (meta.apps && meta.apps[card.kind]) || {};
  rows.push(`<div class="detail-row"><span class="detail-label">${t("card.typeLabel")}</span><span class="detail-value">${esc(app.label || card.kind)}</span></div>`);

  if (card.running && card.uptime > 0) {
    rows.push(`<div class="detail-row"><span class="detail-label">${t("card.uptimeLabel")}</span><span class="detail-value">${esc(fmtUptime(card.uptime))}</span></div>`);
  }
  if (card.running && card.memoryKB > 0) {
    rows.push(`<div class="detail-row"><span class="detail-label">${t("card.memoryLabel")}</span><span class="detail-value">${esc(fmtMem(card.memoryKB))}</span></div>`);
  }
  if (card.dir) {
    rows.push(`<div class="detail-row"><span class="detail-label">${t("card.dirLabel")}</span><span class="detail-value"><code>${esc(card.dir)}</code></span></div>`);
  }
  if (card.port) {
    rows.push(`<div class="detail-row"><span class="detail-label">${t("card.portLabel")}</span><span class="detail-value"><code>${esc(card.port)}</code></span></div>`);
  }
  if (card.url) {
    rows.push(`<div class="detail-row"><span class="detail-label">${t("card.addressLabel")}</span><span class="detail-value"><a class="detail-link" href="${esc(card.url)}" target="_blank" rel="noopener">${esc(card.url)}</a></span></div>`);
  }
  return rows.join("");
}

// openCardModal shows the card's details (status, runtime info and the
// copyable credentials) on card click. Credentials and the edit/delete
// actions are no longer rendered on the card itself.
function openCardModal(card) {
  const creds = credentialRows(card);
  const modal = openModal(`
    <h2><span class="detail-title"><img class="detail-icon" src="/assets/icons/${card.icon}.svg" alt="">${esc(card.name)}</span> <button type="button" class="modal-close" title="${t("modal.close")}"><img src="/assets/icons/x.svg" alt="${t("modal.close")}"></button></h2>
    <div class="detail-list">${detailRows(card)}</div>
    ${creds ? `<div class="detail-creds"><div class="field-label">${t("card.credsLabel")}</div>${creds}</div>` : ""}
    <div class="modal-actions">
      ${card.running ? `<span class="muted" style="margin-right:auto;font-size:12px;align-self:center">${t("card.stopToDelete")}</span>` : ""}
      <button type="button" class="btn ghost" id="btn-detail-delete">${t("card.delete")}</button>
      <button type="button" class="btn primary" id="btn-detail-edit">${t("card.edit")}</button>
    </div>`);

  modal.querySelectorAll(".pw-row").forEach((row) => {
    row.addEventListener("click", () => copyText(row.dataset.copy, row));
  });
  $("#btn-detail-edit", modal).addEventListener("click", () => openEditModal(card));
  const delBtn = $("#btn-detail-delete", modal);
  if (card.running) {
    delBtn.disabled = true;
    delBtn.title = t("card.stopToDelete");
  }
  delBtn.addEventListener("click", () => {
    closeModal();
    removeCard(card);
  });
}

function iconPickerGrid(selected) {
  const gridEl = document.createElement("div");
  gridEl.className = "picker-grid";
  let value = selected;
  for (const icon of meta.icons) {
    const item = document.createElement("button");
    item.type = "button";
    item.className = "picker-item" + (icon === value ? " selected" : "");
    item.innerHTML = `<img src="/assets/icons/${icon}.svg" alt="${icon}">`;
    item.addEventListener("click", () => {
      gridEl.querySelectorAll(".picker-item").forEach((el) => el.classList.remove("selected"));
      item.classList.add("selected");
      value = icon;
    });
    gridEl.appendChild(item);
  }
  return { el: gridEl, get: () => value };
}

function bgPickerGrid(container, selected) {
  const gridEl = document.createElement("div");
  gridEl.className = "picker-grid bg-grid";
  let value = selected;
  container.innerHTML = "";

  function rebuild() {
    gridEl.innerHTML = "";
    const items = meta.backgrounds || [];
    for (const bg of items) {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "picker-item" + (bg.id === value ? " selected" : "");
      item.innerHTML = `<img src="${bg.url}" alt="${bg.id}">`;
      item.addEventListener("click", () => {
        gridEl.querySelectorAll(".picker-item").forEach((el) => el.classList.remove("selected"));
        item.classList.add("selected");
        value = bg.id;
      });
      gridEl.appendChild(item);
    }
    const upload = document.createElement("button");
    upload.type = "button";
    upload.className = "picker-item upload";
    upload.title = t("modal.upload");
    upload.innerHTML = `<img src="/assets/icons/plus.svg" alt="${t("modal.upload")}">`;
    upload.addEventListener("click", async () => {
      const input = document.createElement("input");
      input.type = "file";
      input.accept = "image/jpeg,image/png,image/webp";
      input.addEventListener("change", async () => {
        const file = input.files && input.files[0];
        if (!file) return;
        try {
          const up = await uploadImage(file);
          await refreshMeta();
          value = up.id;
          rebuild();
        } catch (e) {
          alertErr(e);
        }
      });
      input.click();
    });
    gridEl.appendChild(upload);
  }

  rebuild();
  container.appendChild(gridEl);
  return { get: () => value };
}

/* ---------- add card modal ---------- */

// kindIcon returns the default icon asset for a kind.
function kindIcon(kind) {
  switch (kind) {
    case "opencode": return "braces";
    case "mdbook": return "book-open";
    case "vscode": return "code";
    case "vscodium": return "code";
    case "wetty": return "terminal";
    default: return "palette";
  }
}

// ALL_KINDS is the canonical order of the app types.
const ALL_KINDS = ["comfyui", "opencode", "mdbook", "vscode", "vscodium", "wetty"];

// kindEntries returns every known kind with whether it can be added and,
// when it cannot, the i18n key of the reason to show. A kind is
// unavailable when it already has a card or the app is not installed
// (meta.apps[kind].reason carries the specific detection reason).
function kindEntries() {
  return ALL_KINDS.map((k) => {
    const app = (meta.apps && meta.apps[k]) || {};
    if (lastCards.some((c) => c.kind === k)) {
      return { kind: k, app, available: false, reason: "add.reason.alreadyAdded" };
    }
    if (!app.installed) {
      return { kind: k, app, available: false, reason: app.reason || "add.reason.notInstalled" };
    }
    return { kind: k, app, available: true, reason: "" };
  });
}

async function openAddModal() {
  const entries = kindEntries();
  let chosenKind = "";
  let chosenDir = "";         // mdbook project path
  let chosenProjectName = ""; // mdbook project name
  const modal = openModal(`
    <h2>${t("modal.addTitle")} <button type="button" class="modal-close" title="${t("modal.close")}"><img src="/assets/icons/x.svg" alt="${t("modal.close")}"></button></h2>
    <div id="step-kind">
      <div class="field-label">${t("modal.stepKind")}</div>
      <div class="kind-list" id="kind-list"></div>
      <div class="modal-actions">
        <button type="button" class="btn primary" id="btn-kind-next" disabled>${t("modal.next")}</button>
      </div>
    </div>
    <div id="step-project" hidden>
      <div class="field-label">${t("modal.projectStep")}</div>
      <div class="kind-list" id="project-list"></div>
      <p id="project-empty" class="muted" hidden></p>
      <div class="project-create">
        <div class="field-label">${t("modal.createProject")}</div>
        <div class="project-create-row">
          <select id="project-parent" class="btn ghost lang-select"></select>
          <input type="text" id="project-name" placeholder="${t("modal.bookNameLabel")}">
          <button type="button" class="btn primary" id="btn-project-create">${t("modal.projectCreate")}</button>
        </div>
      </div>
      <div class="modal-actions">
        <button type="button" class="btn ghost" id="btn-project-back">${t("modal.backToKinds")}</button>
        <button type="button" class="btn primary" id="btn-project-next" disabled>${t("modal.next")}</button>
      </div>
    </div>
    <div id="step-pickers" hidden>
      <div>
        <div class="field-label">${t("modal.nameLabel")}</div>
        <input type="text" id="card-name">
      </div>
      <div>
        <div class="field-label">${t("modal.iconLabel")}</div>
        <div id="icon-grid"></div>
      </div>
      <div>
        <div class="field-label">${t("modal.bgLabel")}</div>
        <div id="bg-grid"></div>
      </div>
      <div class="modal-actions">
        <button type="button" class="btn ghost" id="btn-pick-again">${t("modal.backToKinds")}</button>
        <button type="button" class="btn primary" id="btn-create">${t("modal.create")}</button>
      </div>
    </div>`);

  const stepKind = $("#step-kind", modal);
  const stepProject = $("#step-project", modal);
  const stepPickers = $("#step-pickers", modal);
  const kindList = $("#kind-list", modal);
  const nextBtn = $("#btn-kind-next", modal);
  let iconPicker, bgPicker;

  for (const entry of entries) {
    const kind = entry.kind;
    const item = document.createElement("button");
    item.type = "button";
    item.id = "kind-" + kind;
    item.className = "kind-item" + (entry.available ? "" : " disabled");
    item.disabled = !entry.available;
    item.innerHTML = `
      <img src="/assets/icons/${kindIcon(kind)}.svg" alt="">
      <span>
        <div>${esc(entry.app.label || kind)}</div>
        <div class="kind-desc">${t("modal.kind" + kind + "Desc")}</div>
        ${entry.available ? "" : `<div class="kind-reason">${esc(t(entry.reason))}</div>`}
      </span>`;
    if (entry.available) {
      item.addEventListener("click", () => {
        chosenKind = kind;
        kindList.querySelectorAll(".kind-item").forEach((el) => el.classList.remove("selected"));
        item.classList.add("selected");
        nextBtn.disabled = false;
      });
    }
    kindList.appendChild(item);
  }

  // showPickers advances to the name/icon/background step with the given
  // default card name.
  function showPickers(name) {
    stepKind.hidden = true;
    stepProject.hidden = true;
    stepPickers.hidden = false;
    $("#card-name", modal).value = name;
    if (!iconPicker) {
      iconPicker = iconPickerGrid(kindIcon(chosenKind));
      $("#icon-grid", modal).appendChild(iconPicker.el);
      bgPicker = bgPickerGrid($("#bg-grid", modal), null);
    }
  }

  nextBtn.addEventListener("click", () => {
    if (!chosenKind) return;
    if (chosenKind === "mdbook") {
      // mdBook cards point at a project: let the user pick an existing
      // book or create a new one before the pickers step.
      stepKind.hidden = true;
      stepProject.hidden = false;
      loadProjects();
    } else {
      showPickers((meta.apps[chosenKind] || {}).label || chosenKind);
    }
  });

  /* ---- mdbook project step ---- */

  const projectList = $("#project-list", modal);
  const projectEmpty = $("#project-empty", modal);
  const projectParent = $("#project-parent", modal);
  const projectName = $("#project-name", modal);
  const projectCreate = $("#btn-project-create", modal);
  const projectNext = $("#btn-project-next", modal);
  let projects = [];

  function renderProjects() {
    projectList.innerHTML = "";
    projectEmpty.hidden = projects.length > 0;
    for (const p of projects) {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "kind-item";
      item.innerHTML = `
        <span>
          <div>${esc(p.name)}</div>
          <div class="project-path">${esc(p.path)}</div>
        </span>`;
      item.addEventListener("click", () => {
        projectList.querySelectorAll(".kind-item").forEach((el) => el.classList.remove("selected"));
        item.classList.add("selected");
        chosenDir = p.path;
        chosenProjectName = p.name;
        projectNext.disabled = false;
      });
      projectList.appendChild(item);
    }
  }

  async function loadProjects() {
    try {
      const data = await api("/api/mdbook/projects");
      projects = data.projects || [];
      projectParent.innerHTML = "";
      for (const d of data.dirs || []) {
        const opt = document.createElement("option");
        opt.value = d;
        opt.textContent = d;
        projectParent.appendChild(opt);
      }
      renderProjects();
    } catch (e) {
      alertErr(e);
      projects = [];
      renderProjects();
    }
  }

  projectCreate.addEventListener("click", async () => {
    const name = projectName.value.trim();
    if (!name) {
      alert(t("modal.bookNameRequired"));
      return;
    }
    projectCreate.disabled = true;
    try {
      const created = await api("/api/mdbook/projects", {
        method: "POST",
        body: JSON.stringify({ dir: projectParent.value, name }),
      });
      projectName.value = "";
      projects = projects.concat([created]);
      renderProjects();
      // The freshly created book is selected so the flow can continue.
      chosenDir = created.path;
      chosenProjectName = created.name;
      projectNext.disabled = false;
      const last = projectList.lastElementChild;
      if (last) last.classList.add("selected");
    } catch (e) {
      alertErr(e);
    } finally {
      projectCreate.disabled = false;
    }
  });

  $("#btn-project-back", modal).addEventListener("click", () => {
    stepProject.hidden = true;
    stepKind.hidden = false;
  });

  projectNext.addEventListener("click", () => {
    if (!chosenDir) return;
    showPickers(chosenProjectName);
  });

  $("#btn-pick-again", modal).addEventListener("click", () => {
    stepPickers.hidden = true;
    stepKind.hidden = false;
  });

  $("#btn-create", modal).addEventListener("click", async () => {
    const btn = $("#btn-create", modal);
    btn.disabled = true;
    const body = {
      kind: chosenKind,
      name: $("#card-name", modal).value.trim(),
      icon: iconPicker.get(),
      background: bgPicker.get(),
    };
    if (chosenKind === "mdbook") body.dir = chosenDir;
    try {
      await api("/api/cards", {
        method: "POST",
        body: JSON.stringify(body),
      });
      closeModal();
      await loadCards();
    } catch (e) {
      alertErr(e);
      btn.disabled = false;
    }
  });
}

/* ---------- edit modal ---------- */

async function openEditModal(card) {
  const modal = openModal(`
    <h2>${t("modal.editTitle")} <button type="button" class="modal-close" title="${t("modal.close")}"><img src="/assets/icons/x.svg" alt="${t("modal.close")}"></button></h2>
    <div>
      <div class="field-label">${t("modal.nameLabel")}</div>
      <input type="text" id="edit-name" value="${esc(card.name)}">
    </div>
    <div>
      <div class="field-label">${t("modal.iconLabel")}</div>
      <div id="edit-icon-grid"></div>
    </div>
    <div>
      <div class="field-label">${t("modal.bgLabel")}</div>
      <div id="edit-bg-grid"></div>
    </div>
    ${card.kind === "opencode" ? `
    <div>
      <div class="field-label">${t("modal.gatewayPortLabel")}</div>
      <input type="number" id="edit-gateway-port" min="1" max="65535" placeholder="4399" style="margin-top:6px">
      <div class="muted" id="edit-gateway-port-hint" style="font-size:12px;margin-top:4px">${t("modal.gatewayPortHint")}</div>
    </div>` : ""}
    <div class="modal-actions">
      <button type="button" class="btn ghost" id="btn-cancel">${t("modal.cancel")}</button>
      <button type="button" class="btn primary" id="btn-save">${t("modal.save")}</button>
    </div>`);

  const iconPicker = iconPickerGrid(card.icon);
  $("#edit-icon-grid", modal).appendChild(iconPicker.el);
  const bgPicker = bgPickerGrid($("#edit-bg-grid", modal), card.background);
  const gwPortInput = card.kind === "opencode" ? $("#edit-gateway-port", modal) : null;
  if (gwPortInput) {
    gwPortInput.value = meta.gatewayPort || "";
    if (card.running) {
      $("#edit-gateway-port-hint", modal).textContent = t("modal.gatewayPortRunningHint");
    }
  }

  $("#btn-cancel", modal).addEventListener("click", closeModal);
  $("#btn-save", modal).addEventListener("click", async () => {
    const btn = $("#btn-save", modal);
    const gwValue = gwPortInput ? parseInt(gwPortInput.value, 10) : 0;
    if (gwPortInput && (!Number.isInteger(gwValue) || gwValue < 1 || gwValue > 65535)) {
      alert(t("err.invalidGatewayPort"));
      return;
    }
    btn.disabled = true;
    try {
      if (gwPortInput && gwValue !== meta.gatewayPort) {
        const res = await api("/api/settings", {
          method: "POST",
          body: JSON.stringify({ gatewayPort: gwValue }),
        });
        meta.gatewayPort = res.gatewayPort;
      }
      await api(`/api/cards/${card.id}`, {
        method: "PATCH",
        body: JSON.stringify({
          name: $("#edit-name", modal).value.trim(),
          icon: iconPicker.get(),
          background: bgPicker.get(),
        }),
      });
      closeModal();
      await loadCards();
    } catch (e) {
      alertErr(e);
      btn.disabled = false;
    }
  });
}

/* ---------- settings modal ---------- */

function settingsBgPicker(container) {
  let selected = meta.pageBackground;
  const gridEl = document.createElement("div");
  gridEl.className = "picker-grid bg-grid";
  container.innerHTML = "";

  function rebuild() {
    gridEl.innerHTML = "";
    for (const bg of meta.backgrounds || []) {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "picker-item" + (bg.id === selected ? " selected" : "");
      item.innerHTML = `<img src="${bg.url}" alt="${bg.id}">`;
      item.addEventListener("click", async () => {
        try {
          await api("/api/settings", {
            method: "POST",
            body: JSON.stringify({ pageBackground: bg.id }),
          });
          selected = bg.id;
          meta.pageBackground = bg.id;
          applyPageBg(bg.id);
          rebuild();
        } catch (e) {
          alertErr(e);
        }
      });
      gridEl.appendChild(item);
    }
    const upload = document.createElement("button");
    upload.type = "button";
    upload.className = "picker-item upload";
    upload.title = t("modal.upload");
    upload.innerHTML = `<img src="/assets/icons/plus.svg" alt="${t("modal.upload")}">`;
    upload.addEventListener("click", async () => {
      const input = document.createElement("input");
      input.type = "file";
      input.accept = "image/jpeg,image/png,image/webp";
      input.addEventListener("change", async () => {
        const file = input.files && input.files[0];
        if (!file) return;
        try {
          const up = await uploadImage(file);
          await refreshMeta();
          selected = up.id;
          meta.pageBackground = up.id;
          await api("/api/settings", {
            method: "POST",
            body: JSON.stringify({ pageBackground: up.id }),
          });
          applyPageBg(up.id);
          rebuild();
        } catch (e) {
          alertErr(e);
        }
      });
      input.click();
    });
    gridEl.appendChild(upload);
  }

  rebuild();
  container.appendChild(gridEl);
}

// restartNarthex asks the server to restart itself and reloads the page
// once a new process (a different bootId) answers. The connection drops
// while the server is restarting, so session polling tolerates errors.
async function restartNarthex(btn) {
  let prevBoot = "";
  try {
    const s = await api("/api/session");
    prevBoot = (s && s.bootId) || "";
  } catch (_) { /* ignore */ }
  const label = btn.textContent;
  btn.disabled = true;
  btn.textContent = t("modal.restarting");
  try {
    await api("/api/restart", { method: "POST", body: "{}" });
  } catch (_) {
    // The old process may drop the connection mid-restart; the poll below
    // is the source of truth.
  }
  const deadline = Date.now() + 30000;
  while (Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, 500));
    try {
      const s = await api("/api/session");
      if (s && s.bootId && s.bootId !== prevBoot) {
        location.reload();
        return;
      }
    } catch (_) { /* server still restarting */ }
  }
  btn.disabled = false;
  btn.textContent = label;
  alert(t("modal.restartFailed"));
}

function buildSettingsModal() {
  const modal = openModal(`
    <h2>${t("modal.settingsTitle")} <button type="button" class="modal-close" title="${t("modal.close")}"><img src="/assets/icons/x.svg" alt="${t("modal.close")}"></button></h2>
    <div>
      <div class="field-label">${t("topbar.lang")}</div>
      <select id="set-lang" class="btn ghost lang-select" style="width:100%">
        <option value="en">English</option>
        <option value="zh-CN">简体中文</option>
        <option value="zh-TW">繁體中文</option>
      </select>
    </div>
    <div>
      <div class="field-label">${t("bgModal.title")}</div>
      <div id="set-bg-grid"></div>
      <div class="muted" style="font-size:12px;margin-top:6px">${t("modal.uploadHint")}</div>
    </div>
    <div>
      <div class="field-label">${t("modal.sloganLabel")}</div>
      <label class="slogan-toggle">
        <input type="checkbox" id="set-slogan-on"> ${t("modal.sloganShow")}
      </label>
      <input type="text" id="set-slogan-text" maxlength="80" placeholder="${t("modal.sloganLabel")}" style="margin-top:6px">
      <div class="muted" style="font-size:12px;margin-top:4px">${t("modal.sloganHint")}</div>
    </div>
    <div>
      <div class="field-label">${t("modal.internalAddressLabel")}</div>
      <input type="text" id="set-internal-addr" maxlength="253" placeholder="${window.location.host}" style="margin-top:6px">
      <div class="muted" style="font-size:12px;margin-top:4px">${t("modal.internalAddressHint")}</div>
    </div>
    <div>
      <div class="field-label">${t("modal.autostartLabel")}</div>
      <label class="slogan-toggle">
        <input type="checkbox" id="set-autostart-on"> ${t("modal.autostartLabel")}
      </label>
      <div class="muted" style="font-size:12px;margin-top:4px" id="set-autostart-hint">${t("modal.autostartHint")}</div>
    </div>
    <div>
      <div class="field-label">${t("modal.restartLabel")}</div>
      <button type="button" class="btn ghost" id="btn-restart-now">${t("modal.restartBtn")}</button>
      <div class="muted" style="font-size:12px;margin-top:4px">${t("modal.restartHint")}</div>
    </div>
    <div class="modal-actions">
      <button type="button" class="btn ghost" id="btn-settings-close">${t("modal.cancel")}</button>
    </div>`);

  const langSelect = $("#set-lang", modal);
  langSelect.value = meta.lang || "en";
  langSelect.addEventListener("change", async () => {
    const want = langSelect.value;
    try {
      const res = await api("/api/settings", {
        method: "POST",
        body: JSON.stringify({ language: want }),
      });
      meta.lang = want;
      meta.slogan = res.slogan || "";
      applyI18n(want);
      updateSlogan();
      closeModal();
      buildSettingsModal();
    } catch (e) {
      langSelect.value = currentLang;
      alertErr(e);
    }
  });

  settingsBgPicker($("#set-bg-grid", modal));

  const sloganOn = $("#set-slogan-on", modal);
  const sloganText = $("#set-slogan-text", modal);
  sloganOn.checked = !!meta.slogan;
  sloganText.value = meta.slogan || "";
  sloganText.disabled = !sloganOn.checked;

  let sloganTimer = null;
  // keepInput: when unchecking, hide the slogan but leave the text in the
  // input untouched, so re-enabling brings it back.
  const applySlogan = async (value, keepInput) => {
    try {
      await api("/api/settings", {
        method: "POST",
        body: JSON.stringify({ slogan: value }),
      });
      meta.slogan = value;
      sloganOn.checked = !!value;
      if (!keepInput) sloganText.value = value;
      sloganText.disabled = !value;
      updateSlogan();
    } catch (e) {
      alertErr(e);
    }
  };

  sloganOn.addEventListener("change", () => {
    if (!sloganOn.checked) {
      clearTimeout(sloganTimer);
      applySlogan("", true);
    } else {
      sloganText.disabled = false;
      let value = sloganText.value.trim();
      if (!value) {
        // Nothing left to restore: fill the localized default slogan.
        value = t("slogan.default");
        sloganText.value = value;
      }
      applySlogan(value);
      sloganText.focus();
    }
  });

  sloganText.addEventListener("input", () => {
    clearTimeout(sloganTimer);
    sloganTimer = setTimeout(() => applySlogan(sloganText.value.trim()), 400);
  });

  const internalAddrInput = $("#set-internal-addr", modal);
  internalAddrInput.value = meta.internalAddress || "";
  let addrTimer = null;
  internalAddrInput.addEventListener("input", () => {
    clearTimeout(addrTimer);
    const value = internalAddrInput.value.trim();
    addrTimer = setTimeout(async () => {
      try {
        await api("/api/settings", {
          method: "POST",
          body: JSON.stringify({ internalAddress: value }),
        });
        meta.internalAddress = value;
      } catch (e) {
        alertErr(e);
      }
    }, 400);
  });

  const autoOn = $("#set-autostart-on", modal);
  const autoHint = $("#set-autostart-hint", modal);
  autoOn.disabled = true;
  // Fetch current state asynchronously so the modal opens instantly.
  (async () => {
    try {
      const st = await api("/api/autostart");
      if (!st.supported) {
        autoOn.disabled = true;
        autoOn.checked = false;
        autoHint.textContent = t("modal.autostartUnsupported");
        return;
      }
      autoOn.disabled = false;
      autoOn.checked = !!st.installed;
    } catch (e) {
      autoHint.textContent = t("modal.autostartUnsupported");
    }
  })();
  autoOn.addEventListener("change", async () => {
    const action = autoOn.checked ? "install" : "uninstall";
    autoOn.disabled = true;
    try {
      const st = await api("/api/autostart", {
        method: "POST",
        body: JSON.stringify({ action }),
      });
      autoOn.checked = !!(st && st.installed);
    } catch (e) {
      alertErr(e);
      // Revert to last known state.
      try {
        const st = await api("/api/autostart");
        autoOn.checked = !!(st && st.installed);
      } catch (_) { /* ignore */ }
    } finally {
      autoOn.disabled = false;
    }
  });

  const restartBtn = $("#btn-restart-now", modal);
  restartBtn.addEventListener("click", () => restartNarthex(restartBtn));

  $("#btn-settings-close", modal).addEventListener("click", closeModal);
}

/* ---------- init ---------- */

function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

loginForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  loginBtn.disabled = true;
  loginError.hidden = true;
  try {
    const user = usernameInput.value.trim();
    await api("/api/login", {
      method: "POST",
      body: JSON.stringify({
        username: user,
        password: passwordInput.value,
      }),
    });
    rememberUsername(user);
    usernameInput.value = "";
    passwordInput.value = "";
    await enterApp();
  } catch (err) {
    loginError.hidden = false;
    if (err.status === 429) {
      loginError.textContent = t("login.errTooMany");
    } else if (err.status === 401) {
      loginError.textContent = t("login.errCredentials");
    } else {
      loginError.textContent = t("login.errOffline");
    }
  }
  loginBtn.disabled = false;
});

btnLogout.addEventListener("click", async () => {
  try { await api("/api/logout", { method: "POST" }); } catch (e) { /* ignore */ }
  showLogin();
});

btnAdd.addEventListener("click", () => {
  if (!meta.icons.length) { alert(t("err.loadFailed")); return; }
  openAddModal();
});

btnSettings.addEventListener("click", () => {
  if (!(meta.backgrounds || []).length) { alert(t("err.loadFailed")); return; }
  closeMenu();
  buildSettingsModal();
});

/* ---------- account page ---------- */

function showAccount() {
  closeMenu();
  appView.hidden = true;
  loginView.hidden = true;
  modalRoot.hidden = true;
  accountView.hidden = false;
  acctUsername.value = meta.username || "";
  acctCurPw.value = "";
  acctNewPw.value = "";
  acctConfirmPw.value = "";
  acctMsg.hidden = true;
}

function hideAccount() {
  accountView.hidden = true;
  appView.hidden = false;
}

btnAccount.addEventListener("click", showAccount);

acctBackBtn.addEventListener("click", hideAccount);

accountForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  acctMsg.hidden = true;
  if (acctNewPw.value !== acctConfirmPw.value) {
    acctMsg.hidden = false;
    acctMsg.className = "acct-msg bad";
    acctMsg.textContent = t("account.passwordMismatch");
    return;
  }
  acctSaveBtn.disabled = true;
  try {
    const res = await api("/api/account", {
      method: "POST",
      body: JSON.stringify({
        currentPassword: acctCurPw.value,
        username: acctUsername.value.trim() || undefined,
        newPassword: acctNewPw.value || undefined,
      }),
    });
    meta.username = res.username;
    rememberUsername(res.username);
    acctUsername.value = res.username;
    acctCurPw.value = "";
    acctNewPw.value = "";
    acctConfirmPw.value = "";
    acctMsg.hidden = false;
    acctMsg.className = "acct-msg ok";
    acctMsg.textContent = t("account.updated");
  } catch (err) {
    acctMsg.hidden = false;
    acctMsg.className = "acct-msg bad";
    acctMsg.textContent = err.status ? err.message : t("err.offline");
  }
  acctSaveBtn.disabled = false;
});

/* ---------- mobile topbar menu ---------- */

function menuOpen() {
  return topbar.classList.contains("open");
}

function openMenu() {
  topbar.classList.add("open");
  menuIcon.src = "/assets/icons/x.svg";
  topbar.classList.remove("hide");
}

function closeMenu() {
  topbar.classList.remove("open");
  menuIcon.src = "/assets/icons/menu.svg";
}

btnMenu.addEventListener("click", (e) => {
  e.stopPropagation();
  menuOpen() ? closeMenu() : openMenu();
});

document.addEventListener("click", (e) => {
  if (menuOpen() && !topbar.contains(e.target)) closeMenu();
});

document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && menuOpen()) closeMenu();
});

topbarActions.addEventListener("click", () => {
  if (menuOpen()) closeMenu();
});

/* ---------- mobile scroll auto-hide ---------- */

(() => {
  const mq = window.matchMedia("(max-width: 768px)");
  let lastY = window.scrollY;
  window.addEventListener("scroll", () => {
    if (!mq.matches || menuOpen()) {
      lastY = window.scrollY;
      return;
    }
    const y = window.scrollY;
    const delta = y - lastY;
    if (y < 80) {
      topbar.classList.remove("hide");
    } else if (delta > 4 && y > 60) {
      topbar.classList.add("hide");
    } else if (delta < -4) {
      topbar.classList.remove("hide");
    }
    lastY = y;
  }, { passive: true });
})();

window.onLangChanged = () => {
  if (lastCards.length) render(lastCards);
};

async function enterApp() {
  showApp();
  try {
    await refreshMeta();
  } catch (e) { /* keep defaults */ }
  rememberUsername(meta.username || "");
  applyI18n(meta.lang || "en");
  applyPageBg(meta.pageBackground || "bg-05");
  updateSlogan();
  await loadCards();
  if (!polling) {
    polling = true;
    poll();
  }
}

// revalidateSession re-checks the session without touching the current
// view unless something changed: an invalidated session drops to the login
// page (with a notice), a valid session restores the app after a bfcache
// restore or a server restart. It is a no-op when already on the login
// page.
async function revalidateSession() {
  try {
    const data = await api("/api/session");
    if (data.authed) {
      if (!loginView.hidden) {
        applyI18n(data.lang || "en");
        applySessionBg(data);
        await enterApp();
      }
    } else if (loginView.hidden) {
      showLogin(true);
    }
  } catch (e) { /* server unreachable; keep the current view */ }
}

async function init() {
  const cached = cachedPageBg();
  if (cached && cached.url) applyPageBgUrl(cached.url);
  try {
    const data = await api("/api/session");
    applyI18n(data.lang || "en");
    applySessionBg(data);
    if (data.authed) await enterApp();
    else showLogin();
  } catch (e) {
    applyI18n("en");
    showLogin();
  } finally {
    // Never leave the page with every view hidden (blank).
    if (loginView.hidden && appView.hidden && accountView.hidden) showLogin();
  }
}

window.addEventListener("pageshow", (e) => {
  if (e.persisted) revalidateSession();
});
window.addEventListener("focus", revalidateSession);
document.addEventListener("visibilitychange", () => {
  if (!document.hidden) revalidateSession();
});

init();
