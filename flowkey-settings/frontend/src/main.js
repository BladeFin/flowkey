// import './style.css';
// import './app.css';

import {
  LoadConfig,
  SaveConfig,
  TriggerReload,
  BrowseExecutable,
  IsDaemonRunning,
  GetLaunchAtLogin,
  SetLaunchAtLogin,
} from "../wailsjs/go/main/App";

// ---------- data model ----------
console.log("main.js");
let appsData = {};
//TODO: Implement launchAtLogin and trayIcon
let generalData = { launchAtLogin: true, trayIcon: false };
let originalAppsData = {};
let originalGeneralData = JSON.parse(JSON.stringify(generalData));

function configToAppsData(cfg) {
  const out = {};
  for (const [id, app] of Object.entries(cfg.apps || {})) {
    out[id] = {
      id,
      type: app.type,
      title: app.title,
      path: app.path,
      windowTitle: app.windowTitle || "",
      hotkey: app.hotkey,
      alwaysOnTop: !!app.alwaysOnTop,
    };
  }
  return out;
}

function appsDataToConfig() {
  const apps = {};
  for (const [id, app] of Object.entries(appsData)) {
    apps[id] = {
      type: app.type,
      title: app.title,
      path: app.path,
      hotkey: app.hotkey,
      ...(app.type === "mini"
        ? { windowTitle: app.windowTitle, alwaysOnTop: app.alwaysOnTop }
        : {}),
    };
  }
  return { apps };
}

async function init() {
  try {
    const raw = await LoadConfig();
    appsData = configToAppsData(JSON.parse(raw));
  } catch (err) {
    console.error("failed to load config:", err);
    appsData = {};
  }
  originalAppsData = JSON.parse(JSON.stringify(appsData));
  renderApps();
}

const gearSVG = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/>
<circle cx="12" cy="12" r="3"/>
</svg>`;

function escapeHTML(s) {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

// ---------- tooltip (shared by help buttons and hotkey-conflict warnings) ----------
const tooltipEl = document.getElementById("tooltip");
function showTooltip(targetEl, text, opts) {
  opts = opts || {};
  tooltipEl.textContent = text;
  tooltipEl.classList.toggle("warn", !!opts.warn);
  tooltipEl.style.display = "block";
  const rect = targetEl.getBoundingClientRect();
  const tw = tooltipEl.offsetWidth,
    th = tooltipEl.offsetHeight;
  let top = rect.top - th - 7;
  let left = rect.left + rect.width / 2 - tw / 2;
  if (top < 4) top = rect.bottom + 7;
  left = Math.max(4, Math.min(left, window.innerWidth - tw - 4));
  tooltipEl.style.top = top + "px";
  tooltipEl.style.left = left + "px";
  clearTimeout(tooltipEl._hideTimer);
  if (opts.duration) {
    tooltipEl._hideTimer = setTimeout(hideTooltip, opts.duration);
  }
}
function hideTooltip() {
  tooltipEl.style.display = "none";
}

function wireHelpButtons(root) {
  root.querySelectorAll(".help-btn").forEach((btn) => {
    btn.addEventListener("mouseenter", () =>
      showTooltip(btn, btn.dataset.help),
    );
    btn.addEventListener("mouseleave", hideTooltip);
    btn.addEventListener("click", (e) => e.stopPropagation());
  });
}
wireHelpButtons(document);

// ---------- hotkey conflict check ----------
function hotkeyOwner(letter, excludeId) {
  return Object.values(appsData).find(
    (a) => a.id !== excludeId && a.hotkey === letter,
  );
}

// ---------- render apps ----------
function renderApps() {
  const launchBody = document.getElementById("body-launch");
  const miniBody = document.getElementById("body-mini");
  launchBody.innerHTML = "";
  miniBody.innerHTML = "";
  let launchCount = 0,
    miniCount = 0;

  Object.values(appsData).forEach((app) => {
    const row = document.createElement("div");
    row.className = "row";
    const sub =
      app.type == "mini" ? `${app.path} · "${app.windowTitle}"` : app.path;
    row.innerHTML = `
    <div class="row-main">
        <div class="row-title">${escapeHTML(app.title)}</div>
        <div class="row-sub">${escapeHTML(sub)}</div>
    </div>
    <div class="icon-btn" data-role="props">${gearSVG}</div>
    <div class="hotkey-controls">
      <div class="keycap" data-role="keycap">${escapeHTML(app.hotkey || "—")}</div>
      <button class="icon-btn-sm" data-role="clear-hotkey" title="Remove hotkey">✕</button>
    </div>
    `;
    row
      .querySelector('[data-role="props"]')
      .addEventListener("click", () => openProps(app.id));
    const kc = row.querySelector('[data-role="keycap"]');
    kc.addEventListener("click", () =>
      beginListen(kc, (letter, original, el) => {
        if (!/^[A-Z]$/.test(letter)) {
          el.textContent = original;
          el.classList.add("conflict");
          showTooltip(el, "Only A–Z keys are supported right now", {
            warn: true,
            duration: 1600,
          });
          setTimeout(() => el.classList.remove("conflict"), 900);
          return;
        }
        const owner = hotkeyOwner(letter, app.id);
        if (owner) {
          el.textContent = original;
          el.classList.add("conflict");
          showTooltip(el, `${letter} is already used by ${owner.title}`, {
            warn: true,
            duration: 1600,
          });
          setTimeout(() => el.classList.remove("conflict"), 900);
          return;
        }
        el.textContent = letter;
        appsData[app.id].hotkey = letter;
        recomputeDirty();
      }),
    );
    row
      .querySelector('[data-role="clear-hotkey"]')
      .addEventListener("click", (e) => {
        e.stopPropagation();
        appsData[app.id].hotkey = "";
        kc.textContent = "—";
        recomputeDirty();
      });

    if (app.type === "launch") {
      launchBody.appendChild(row);
      launchCount++;
    } else {
      miniBody.appendChild(row);
      miniCount++;
    }
  });

  document.getElementById("count-launch").textContent = launchCount;
  document.getElementById("count-mini").textContent = miniCount;
}

// ---------- nav ----------
document.querySelectorAll(".nav-item").forEach((item) => {
  item.addEventListener("click", () => {
    document
      .querySelectorAll(".nav-item")
      .forEach((n) => n.classList.remove("active"));
    document
      .querySelectorAll(".page")
      .forEach((p) => p.classList.remove("active"));
    item.classList.add("active");
    document
      .getElementById("page-" + item.dataset.page)
      .classList.add("active");
  });
});

// ---------- collapsible sections ----------
document.querySelectorAll(".section-head").forEach((head) => {
  head.addEventListener("click", () => {
    head.closest(".section").classList.toggle("collapsed");
  });
});
document.querySelectorAll(".section-head-btn").forEach((btn) => {
  btn.addEventListener("click", (e) => e.stopPropagation());
});
// ---------- dirty diffing ----------
function recomputeDirty() {
  const dirty =
    JSON.stringify(appsData) !== JSON.stringify(originalAppsData) ||
    JSON.stringify(generalData) !== JSON.stringify(originalGeneralData);
  document.getElementById("revert-btn").classList.toggle("dirty", dirty);
  document.getElementById("revert-btn").disabled = !dirty;
  document.getElementById("save-btn").classList.toggle("dirty", dirty);
  document.getElementById("save-btn").disabled = !dirty;
  document.getElementById("unsaved-dot").classList.toggle("show", dirty);
  return dirty;
}

document.getElementById("revert-btn").addEventListener("click", async () => {
  appsData = JSON.parse(JSON.stringify(originalAppsData));
  generalData = JSON.parse(JSON.stringify(originalGeneralData));
  renderApps();
  recomputeDirty();
  flashStatus("changes reverted");
});
document.getElementById("save-btn").addEventListener("click", async () => {
  try {
    console.log("appsData.calculator:", appsData.calculator);
    const payload = JSON.stringify(appsDataToConfig(), null, 2);
    console.log("saving:", payload);
    await SaveConfig(JSON.stringify(appsDataToConfig(), null, 2));
    originalAppsData = JSON.parse(JSON.stringify(appsData));
    originalGeneralData = JSON.parse(JSON.stringify(generalData));
    recomputeDirty();
    flashStatus("config saved · reloaded");
  } catch (err) {
    console.error("save failed:", err);
    flashStatus("save failed - see console");
  }
});

function flashStatus(msg) {
  const el = document.getElementById("status-text");
  const original = el.textContent;
  el.textContent = msg;
  setTimeout(() => {
    el.textContent = original;
  }, 1400);
}
async function manualReload() {
  try {
    await TriggerReload();
    flashStatus("reload signal sent");
  } catch (err) {
    console.error(err);
    flashStatus("reload failed - see console");
  }
}

async function pollDaemonStatus() {
  let running = false;
  try {
    running = await IsDaemonRunning();
  } catch (err) {
    console.error("daemon status check failed:", err);
  }
  document.querySelector(".status-dot").classList.toggle("offline", !running);
  document.getElementById("status-text").textContent = running
    ? "daemon running"
    : "daemon not running";
}
pollDaemonStatus();
setInterval(pollDaemonStatus, 3000);

document.getElementById("reload-btn").addEventListener("click", manualReload);

// ---------- general toggles ----------
const loginToggle = document.getElementById("toggle-login");

async function initLaunchAtLoginToggle() {
  try {
    const enabled = await GetLaunchAtLogin();
    setToggleState(loginToggle, enabled);
  } catch (err) {
    console.error("failed to read launch-at-login state:", err);
    loginToggle.classList.add("disabled");
  }
}

function setToggleState(el, on) {
  el.classList.toggle("on", on);
  el.querySelector(".toggle-label").textContent = on ? "ON" : "OFF";
}

loginToggle.addEventListener("click", async () => {
  if (
    loginToggle.classList.contains("disabled") ||
    loginToggle.classList.contains("pending")
  ) {
    return;
  }

  const next = !loginToggle.classList.contains("on");
  loginToggle.classList.add("pending");

  try {
    await SetLaunchAtLogin(next);
    setToggleState(loginToggle, next);
  } catch (err) {
    console.error("failed to set launch-at-login: ", err);
  } finally {
    loginToggle.classList.remove("pending");
  }
});

initLaunchAtLoginToggle();

// ---------- shared keycap listen: captures a key, or cancels on click-away ----------
function beginListen(keycapEl, onResolved) {
  const original = keycapEl.textContent;
  keycapEl.textContent = "";
  keycapEl.classList.add("listening");

  function cleanup() {
    keycapEl.classList.remove("listening");
    document.removeEventListener("keydown", onKey);
    document.removeEventListener("mousedown", onClickAway, true);
  }
  function onKey(e) {
    e.preventDefault();
    if (e.key === "Escape") {
      cleanup();
      keycapEl.textContent = original;
      return;
    }
    const letter = e.key.length === 1 ? e.key.toUpperCase() : e.key;
    cleanup();
    onResolved(letter, original, keycapEl);
  }
  function onClickAway(e) {
    if (e.target === keycapEl) return;
    cleanup();
    keycapEl.textContent = original;
  }
  document.addEventListener("keydown", onKey);
  // deferred so the click that opened listening mode doesn't immediately cancel it
  setTimeout(
    () => document.addEventListener("mousedown", onClickAway, true),
    0,
  );
}

// ---------- properties modal ----------
let modalDraft = null;
let modalAppId = null;
let isNewApp = false;

const modalKeycapEl = document.getElementById("modal-keycap");
modalKeycapEl.addEventListener("click", () => {
  beginListen(modalKeycapEl, (letter, original, el) => {
    if (!/^[A-Z]$/.test(letter)) {
      el.textContent = original;
      el.classList.add("conflict");
      showTooltip(el, "Only A–Z keys are supported right now", {
        warn: true,
        duration: 1600,
      });
      setTimeout(() => el.classList.remove("conflict"), 900);
      return;
    }
    const owner = hotkeyOwner(letter, modalAppId);
    if (owner) {
      el.textContent = original;
      el.classList.add("conflict");
      showTooltip(el, `${letter} is already used by ${owner.title}`, {
        warn: true,
        duration: 1600,
      });
      setTimeout(() => el.classList.remove("conflict"), 900);
      return;
    }
    el.textContent = letter;
    modalDraft.hotkey = letter;
  });
});

function openProps(id) {
  modalAppId = id;
  modalDraft = JSON.parse(JSON.stringify(appsData[id]));
  isNewApp = false;
  fillModal();
}

function fillModal() {
  document.getElementById("modal-title").textContent =
    (isNewApp ? "New - " : "Properties - ") + modalDraft.title;
  document.getElementById("modal-name").value = modalDraft.title;
  document.getElementById("modal-path").value = modalDraft.path;
  document.getElementById("modal-window").value = modalDraft.windowTitle;
  document.getElementById("modal-window-field").style.display =
    modalDraft.type == "mini" ? "block" : "none";
  document.getElementById("modal-aot-field").style.display =
    modalDraft.type == "mini" ? "block" : "none";

  const aotToggle = document.getElementById("modal-aot-toggle");
  aotToggle.classList.toggle("on", modalDraft.alwaysOnTop);
  aotToggle.querySelector(".toggle-label").textContent = modalDraft.alwaysOnTop
    ? "ON"
    : "OFF";

  const kc = document.getElementById("modal-keycap");
  kc.classList.remove("listening", "conflict");
  kc.textContent = modalDraft.hotkey || "—";

  deleteBtn.style.display = isNewApp ? "none" : "inline-block";
  resetDeleteBtn();

  document.getElementById("modal-name").oninput = (e) => {
    modalDraft.title = e.target.value;
  };
  document.getElementById("modal-path").oninput = (e) => {
    modalDraft.path = e.target.value;
  };
  document.getElementById("modal-window").oninput = (e) => {
    modalDraft.windowTitle = e.target.value;
  };

  document.getElementById("scrim").classList.add("show");
}

function closeProps() {
  document.getElementById("scrim").classList.remove("show");
  modalDraft = null;
  modalAppId = null;
  resetDeleteBtn();
}

function applyProps() {
  appsData[modalAppId] = modalDraft;
  isNewApp = false;
  renderApps();
  recomputeDirty();
  closeProps();
}

document
  .querySelector("#section-launch .section-head-btn")
  .addEventListener("click", () => startAddApp("launch"));
document
  .querySelector("#section-mini .section-head-btn")
  .addEventListener("click", () => startAddApp("mini"));
document
  .getElementById("modal-browse-btn")
  .addEventListener("click", async () => {
    try {
      const path = await BrowseExecutable();
      if (!path) return;
      modalDraft.path = path;
      document.getElementById("modal-path").value = path;
    } catch (err) {
      console.error("browse failed:", err);
      flashStatus("browse failed - see console");
    }
  });
document
  .getElementById("modal-close-btn")
  .addEventListener("click", closeProps);
document
  .getElementById("modal-cancel-btn")
  .addEventListener("click", closeProps);
document
  .getElementById("modal-apply-btn")
  .addEventListener("click", applyProps);

document
  .getElementById("modal-aot-toggle")
  .addEventListener("click", function () {
    modalDraft.alwaysOnTop = !modalDraft.alwaysOnTop;
    this.classList.toggle("on");
    this.querySelector(".toggle-label").textContent = modalDraft.alwaysOnTop
      ? "ON"
      : "OFF";
  });

document.addEventListener("keydown", (e) => {
  if (e.key !== "Escape") return;
  if (!document.getElementById("scrim").classList.contains("show")) return;
  if (modalKeycapEl.classList.contains("listening")) return; //see beginListen
  closeProps();
});

document.getElementById("modal-hotkey-clear").addEventListener("click", () => {
  modalDraft.hotkey = "";
  const kc = document.getElementById("modal-keycap");
  kc.classList.remove("listening", "conflict");
  kc.textContent = "—";
});

document.getElementById("scrim").addEventListener("click", (e) => {
  if (e.target.id === "scrim") closeProps();
});

async function startAddApp(type) {
  let path;
  try {
    path = await BrowseExecutable();
  } catch (err) {
    console.error("browse failed:", err);
    flashStatus("browse failed - see console");
    return;
  }
  if (!path) return; //user cancelled the dialog

  const base = path
    .split(/[\\/]/)
    .pop()
    .replace(/\.exe$/i, "");
  let id =
    base
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "") || "app";
  let candidate = id,
    n = 1;
  while (appsData[candidate]) candidate = `${id}-${n++}`;
  id = candidate;

  modalAppId = id;
  modalDraft = {
    id,
    type,
    title: base,
    path,
    windowTitle: "",
    hotkey: "",
    alwaysOnTop: false,
  };
  isNewApp = true;
  fillModal();
}

const deleteBtn = document.getElementById("modal-delete-btn");
let deleteArmed = false;
let deleteArmTimer = null;

function resetDeleteBtn() {
  deleteArmed = false;
  deleteBtn.textContent = "Remove App";
  deleteBtn.classList.remove("confirm");
  clearTimeout(deleteArmTimer);
}

deleteBtn.addEventListener("click", () => {
  if (!deleteArmed) {
    deleteArmed = true;
    deleteBtn.textContent = "Confirm Remove";
    deleteBtn.classList.add("confirm");
    deleteArmTimer = setTimeout(resetDeleteBtn, 2500);
    return;
  }
  delete appsData[modalAppId];
  resetDeleteBtn();
  renderApps();
  recomputeDirty();
  closeProps();
});

init();
