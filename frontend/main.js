import { Accounts } from "./bindings/github.com/gongjs/agym/internal/appservice/index.js";
import { getLang, t, toggleLang } from "./i18n.js";

const MODEL_PREFS_KEY = "agym.modelPrefs";
const THINKING_PREFS_KEY = "agym.thinkingPrefs";

function mustQuery(selector) {
  const el = document.querySelector(selector);
  if (!el) {
    throw new Error(`Missing required DOM element: ${selector}`);
  }
  return el;
}

function collectElements(selectors) {
  return Object.fromEntries(
    Object.entries(selectors).map(([key, selector]) => [key, mustQuery(selector)])
  );
}

const {
  list,
  emptyState,
  count,
  summary,
  status,
  addForm,
  emailInput,
  addButton,
  refreshButton,
  langToggle,
  oauthStart,
  oauthPanel,
  oauthMessage,
  oauthLink,
  oauthCodeForm,
  oauthCode,
  oauthSubmit,
  oauthCancel,
} = collectElements({
  list: "#account-list",
  emptyState: "#empty-state",
  count: "#account-count",
  summary: "#summary-text",
  status: "#status",
  addForm: "#add-form",
  emailInput: "#email-input",
  addButton: "#add-button",
  refreshButton: "#refresh",
  langToggle: "#lang-toggle",
  oauthStart: "#oauth-start",
  oauthPanel: "#oauth-panel",
  oauthMessage: "#oauth-message",
  oauthLink: "#oauth-link",
  oauthCodeForm: "#oauth-code-form",
  oauthCode: "#oauth-code",
  oauthSubmit: "#oauth-submit",
  oauthCancel: "#oauth-cancel",
});

let oauthAttemptID = "";
let oauthPollTimer = null;
let handledOAuthAddedAttemptID = "";
let latestAccounts = [];
let latestSummary = { key: "summary.loading", params: {} };
let latestStatus = { key: "", params: {}, raw: "", isError: false };
let latestOAuthView = null;

function readPrefMap(storageKey) {
  try {
    const raw = window.localStorage.getItem(storageKey);
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

function writePrefValue(storageKey, email, value) {
  try {
    const map = readPrefMap(storageKey);
    map[email] = value;
    window.localStorage.setItem(storageKey, JSON.stringify(map));
  } catch {
    // Ignore storage access errors in restricted webviews.
  }
}

function setStatusKey(key, params = {}, isError = false) {
  latestStatus = { key, params, raw: "", isError };
  status.textContent = key ? t(key, params) : "";
  status.title = status.textContent;
  status.classList.toggle("error", isError);
}

function setStatusRaw(message, isError = false) {
  latestStatus = { key: "", params: {}, raw: message, isError };
  status.textContent = message;
  status.title = message;
  status.classList.toggle("error", isError);
}

function setSummary(key, params = {}) {
  latestSummary = { key, params };
  summary.textContent = t(key, params);
}

function applyStaticTranslations() {
  document.documentElement.lang = getLang();
  for (const el of document.querySelectorAll("[data-i18n]")) {
    el.textContent = t(el.dataset.i18n);
  }
  for (const el of document.querySelectorAll("[data-i18n-placeholder]")) {
    el.placeholder = t(el.dataset.i18nPlaceholder);
  }
  for (const el of document.querySelectorAll("[data-i18n-title]")) {
    el.title = t(el.dataset.i18nTitle);
  }
  for (const el of document.querySelectorAll("[data-i18n-aria]")) {
    el.setAttribute("aria-label", t(el.dataset.i18nAria));
  }
}

function rerenderCurrentView() {
  applyStaticTranslations();
  setSummary(latestSummary.key, latestSummary.params);
  if (latestStatus.key) {
    setStatusKey(latestStatus.key, latestStatus.params, latestStatus.isError);
  } else {
    setStatusRaw(latestStatus.raw, latestStatus.isError);
  }
  renderAccountList(latestAccounts);
  if (latestOAuthView) {
    renderOAuthStatus(latestOAuthView);
  }
}

function makeBadge(label, className = "", title = "") {
  const badge = document.createElement("span");
  badge.className = `badge ${className}`.trim();
  badge.textContent = label;
  if (title) badge.title = title;
  return badge;
}

function quotaLabel(account) {
  const quota = account.quota;
  if (!quota) return t("quota.unfetched");
  const known = new Set(["ok", "stale", "no_keychain", "ineligible", "unsupported_region", "auth_failed", "timeout", "parse_err", "error"]);
  return known.has(quota.status) ? t(`quota.status.${quota.status}`) : t("quota.status.unknown");
}

function renderQuota(account, container) {
  const quota = account.quota;
  const row = document.createElement("div");
  row.className = "quota-row";
  if (!quota) {
    row.append(makeBadge(t("quota.unfetched"), "muted"));
    container.append(row);
    return;
  }
  const bucketKeys = ["gw", "g5", "cw", "c5"];
  let shown = 0;
  for (const key of bucketKeys) {
    const bucket = quota.buckets?.[key];
    if (!bucket || bucket.percent < 0) continue;
    row.append(
      makeBadge(
        `${t(`quota.bucket.${key}`)} ${bucket.percent}%`,
        "quota",
        bucket.reset ? t("quota.reset", { reset: bucket.reset }) : ""
      )
    );
    shown += 1;
  }
  if (!shown) {
    row.append(
      makeBadge(
        quotaLabel(account),
        quota.status === "error" || quota.status === "timeout" ? "missing" : "muted",
        quota.error ?? ""
      )
    );
  }
  if (quota.status === "stale") {
    row.append(makeBadge(t("badge.stale"), "muted", quota.error ?? t("badge.staleTitle")));
  }
  if (quota.status === "ineligible" && quota.error?.startsWith("https://")) {
    const verify = document.createElement("a");
    verify.className = "verify-link";
    verify.href = quota.error;
    verify.target = "_blank";
    verify.rel = "noreferrer";
    verify.textContent = t("quota.verify");
    row.append(verify);
  }
  container.append(row);
}

function renderControlsRow(account, container) {
  const row = document.createElement("div");
  row.className = "controls-row";

  const modelWrap = document.createElement("div");
  modelWrap.className = "select-wrap";
  const modelSelect = document.createElement("select");
  modelSelect.className = "pill-select";
  modelSelect.setAttribute("aria-label", t("model.selectAria", { email: account.email }));

  const models = account.availableModels ?? [];
  const savedModels = readPrefMap(MODEL_PREFS_KEY);
  const preferredModel = savedModels[account.email] || account.defaultModel || "";

  if (!account.modelsKnown || !models.length) {
    const emptyOpt = document.createElement("option");
    emptyOpt.value = "";
    emptyOpt.textContent = t("model.empty");
    modelSelect.append(emptyOpt);
    modelSelect.disabled = true;
  } else {
    for (const model of models) {
      const opt = document.createElement("option");
      opt.value = model.id;
      const suffix = model.tag ? ` · ${model.tag}` : (model.id === account.defaultModel ? ` · ${t("model.defaultTag")}` : "");
      opt.textContent = `${model.name}${suffix}`;
      if (model.id === preferredModel) opt.selected = true;
      modelSelect.append(opt);
    }
    modelSelect.addEventListener("change", () => {
      writePrefValue(MODEL_PREFS_KEY, account.email, modelSelect.value);
      setStatusKey("status.modelSaved", { email: account.email });
    });
  }
  modelWrap.append(modelSelect);

  const thinkingWrap = document.createElement("div");
  thinkingWrap.className = "select-wrap";
  const thinkingSelect = document.createElement("select");
  thinkingSelect.className = "pill-select";
  thinkingSelect.setAttribute("aria-label", t("thinking.selectAria", { email: account.email }));

  const savedThinking = readPrefMap(THINKING_PREFS_KEY)[account.email] || "default";
  for (const level of ["default", "low", "medium", "high"]) {
    const opt = document.createElement("option");
    opt.value = level;
    opt.textContent = t(`thinking.${level}`);
    if (level === savedThinking) opt.selected = true;
    thinkingSelect.append(opt);
  }
  thinkingSelect.addEventListener("change", () => {
    writePrefValue(THINKING_PREFS_KEY, account.email, thinkingSelect.value);
    setStatusKey("status.thinkingSaved", { email: account.email });
  });
  thinkingWrap.append(thinkingSelect);

  row.append(modelWrap, thinkingWrap);
  container.append(row);
}

function renderAccount(account) {
  const row = document.createElement("article");
  row.className = "account-row";
  const needsAddCurrent = account.current && !account.inRoster;

  const top = document.createElement("div");
  top.className = "row-top";

  const avatar = document.createElement("span");
  avatar.className = "avatar";
  avatar.textContent = account.email.slice(0, 1);

  const meta = document.createElement("div");
  meta.className = "account-meta";
  const email = document.createElement("div");
  email.className = "email";
  email.textContent = account.email;
  email.title = account.email;
  meta.append(email);

  const badges = document.createElement("div");
  badges.className = "row-badges";
  if (account.current) badges.append(makeBadge(t("badge.current"), "current"));
  if (!account.hasSavedLogin) badges.append(makeBadge(t("badge.noLogin"), "missing"));
  if (account.quota?.tierName) badges.append(makeBadge(account.quota.tierName, "tier", account.quota.tierId || t("badge.tierTitle")));
  if (account.quota?.regionCode) badges.append(makeBadge(account.quota.regionCode, "region", t("badge.regionTitle")));

  const actions = document.createElement("div");
  actions.className = "row-actions";

  const rotationLabel = document.createElement("label");
  rotationLabel.className = "rotation";
  rotationLabel.title = t("account.rotationTitle");
  const rotation = document.createElement("input");
  rotation.type = "checkbox";
  rotation.checked = account.rotation;
  rotation.setAttribute("aria-label", t("account.rotationAria", { email: account.email }));
  rotation.addEventListener("change", async () => {
    rotation.disabled = true;
    try {
      await Accounts.SetRotation(account.email, rotation.checked);
      setStatusKey("status.rotationSaved");
    } catch (error) {
      rotation.checked = !rotation.checked;
      setStatusRaw(error?.message ?? String(error), true);
    } finally {
      rotation.disabled = false;
    }
  });
  rotationLabel.append(rotation, document.createTextNode(t("account.rotation")));
  rotationLabel.hidden = needsAddCurrent;

  const switchButton = document.createElement("button");
  switchButton.className = account.current ? "switch-button current" : "switch-button";
  switchButton.type = "button";
  switchButton.textContent = account.current ? t("account.switchCurrent") : t("account.switch");
  switchButton.disabled = account.current || !account.hasSavedLogin;
  switchButton.setAttribute(
    "aria-label",
    account.current
      ? t("account.switchCurrentAria", { email: account.email })
      : t("account.switchAria", { email: account.email })
  );
  switchButton.addEventListener("click", async () => {
    switchButton.disabled = true;
    setStatusKey("status.switching");
    try {
      await Accounts.SwitchAccount(account.email);
      setStatusKey("status.switched", { email: account.email });
      await loadAccounts();
    } catch (error) {
      setStatusRaw(error?.message ?? String(error), true);
      switchButton.disabled = false;
    }
  });
  switchButton.hidden = needsAddCurrent;

  const remove = document.createElement("button");
  remove.className = "remove-button";
  remove.type = "button";
  remove.title = t("account.removeTitle");
  remove.setAttribute("aria-label", t("account.removeAria", { email: account.email }));
  remove.textContent = "×";
  remove.addEventListener("click", async () => {
    if (!window.confirm(t("account.removeConfirm", { email: account.email }))) return;
    remove.disabled = true;
    try {
      await Accounts.RemoveAccount(account.email);
      setStatusKey("status.removed");
      await loadAccounts();
    } catch (error) {
      setStatusRaw(error?.message ?? String(error), true);
      remove.disabled = false;
    }
  });
  remove.hidden = needsAddCurrent;

  const addCurrentButton = document.createElement("button");
  addCurrentButton.className = "current-add-button";
  addCurrentButton.type = "button";
  addCurrentButton.textContent = t("account.saveAndAdd");
  addCurrentButton.hidden = !needsAddCurrent;
  addCurrentButton.setAttribute("aria-label", t("account.saveAndAddAria", { email: account.email }));
  addCurrentButton.addEventListener("click", async () => {
    addCurrentButton.disabled = true;
    setStatusKey("status.savingCurrent");
    try {
      const added = await Accounts.AddCurrentAccount(account.email);
      if (added) {
        setStatusKey("status.savedCurrent");
        await reloadAndRefreshQuotas();
      } else {
        setStatusKey("status.alreadyInRoster");
        await loadAccounts();
      }
    } catch (error) {
      setStatusRaw(error?.message ?? String(error), true);
      addCurrentButton.disabled = false;
    }
  });

  actions.append(rotationLabel, switchButton, remove, addCurrentButton);
  top.append(avatar, meta, actions);
  row.append(top);
  if (badges.childElementCount > 0) row.append(badges);
  renderQuota(account, row);
  renderControlsRow(account, row);
  return row;
}

function renderAccountList(accounts) {
  latestAccounts = accounts;
  list.replaceChildren(...accounts.map(renderAccount));
  count.textContent = String(accounts.length);
  emptyState.hidden = accounts.length !== 0;
}

function updateSummaryFromAccounts(accounts) {
  const active = accounts.find((account) => account.current);
  if (active) {
    setSummary("summary.current", { email: active.email });
  } else {
    setSummary("summary.none");
  }
}

async function loadAccounts() {
  setStatusKey("");
  setSummary("summary.loading");
  try {
    const accounts = await Accounts.ListAccounts();
    renderAccountList(accounts);
    updateSummaryFromAccounts(accounts);
  } catch (error) {
    renderAccountList([]);
    setSummary("summary.error");
    setStatusRaw(error?.message ?? String(error), true);
  }
}

addForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const email = emailInput.value.trim();
  if (!email) return;
  addButton.disabled = true;
  setStatusKey("status.adding");
  try {
    const added = await Accounts.AddExistingAccount(email);
    emailInput.value = "";
    if (added) {
      setStatusKey("status.added");
      await reloadAndRefreshQuotas();
    } else {
      setStatusKey("status.exists");
      await loadAccounts();
    }
  } catch (error) {
    setStatusRaw(error?.message ?? String(error), true);
  } finally {
    addButton.disabled = false;
  }
});

async function refreshQuotas() {
  if (refreshButton.disabled) return;
  refreshButton.disabled = true;
  setSummary("summary.refreshing");
  setStatusKey("status.refreshing");
  try {
    const accounts = await Accounts.RefreshQuotas();
    renderAccountList(accounts);
    updateSummaryFromAccounts(accounts);
    setStatusKey("status.refreshed");
  } catch (error) {
    setStatusRaw(error?.message ?? String(error), true);
    await loadAccounts();
  } finally {
    refreshButton.disabled = false;
  }
}

async function reloadAndRefreshQuotas() {
  await loadAccounts();
  await refreshQuotas();
}

refreshButton.addEventListener("click", refreshQuotas);
langToggle.addEventListener("click", () => {
  toggleLang();
  rerenderCurrentView();
});

function formatOAuthMessage(view) {
  if (view.state === "error" && view.message) return view.message;
  const key = `oauth.msg.${view.state}`;
  const translated = t(key);
  if (translated !== key) return translated;
  return view.message || t("oauth.msg.waiting");
}

function renderOAuthStatus(view) {
  latestOAuthView = view;
  if (!view || view.state === "idle") {
    oauthPanel.hidden = true;
    return;
  }
  oauthAttemptID = view.attemptId ?? oauthAttemptID;
  oauthPanel.hidden = false;
  oauthMessage.textContent = formatOAuthMessage(view);
  oauthLink.hidden = view.state !== "authorize" || !view.url;
  if (view.url) oauthLink.href = view.url;
  oauthCodeForm.hidden = view.state !== "authorize";
  oauthCancel.hidden = !["starting", "waiting", "authorize"].includes(view.state);
  oauthStart.disabled = ["starting", "waiting", "authorize"].includes(view.state);
  if (view.state === "added") {
    oauthStart.disabled = false;
    if (oauthPollTimer) clearInterval(oauthPollTimer);
    oauthPollTimer = null;
    if (oauthAttemptID && handledOAuthAddedAttemptID !== oauthAttemptID) {
      handledOAuthAddedAttemptID = oauthAttemptID;
      void reloadAndRefreshQuotas();
    }
  } else if (["error", "cancelled"].includes(view.state)) {
    oauthStart.disabled = false;
    if (oauthPollTimer) clearInterval(oauthPollTimer);
    oauthPollTimer = null;
  }
}

async function pollOAuthStatus() {
  try {
    renderOAuthStatus(await Accounts.OAuthStatus());
  } catch (error) {
    setStatusRaw(error?.message ?? String(error), true);
  }
}

function beginOAuthPolling() {
  if (oauthPollTimer) clearInterval(oauthPollTimer);
  oauthPollTimer = setInterval(pollOAuthStatus, 650);
}

oauthStart.addEventListener("click", async () => {
  oauthStart.disabled = true;
  oauthPanel.hidden = false;
  oauthMessage.textContent = t("oauth.msg.starting");
  try {
    const view = await Accounts.StartOAuth();
    renderOAuthStatus(view);
    beginOAuthPolling();
  } catch (error) {
    oauthStart.disabled = false;
    setStatusRaw(error?.message ?? String(error), true);
    await pollOAuthStatus();
    beginOAuthPolling();
  }
});

oauthCodeForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const code = oauthCode.value.trim();
  if (!code || !oauthAttemptID) return;
  oauthSubmit.disabled = true;
  try {
    await Accounts.SubmitOAuthCode(oauthAttemptID, code);
    oauthCode.value = "";
    oauthMessage.textContent = t("oauth.msg.submitted");
  } catch (error) {
    oauthMessage.textContent = error?.message ?? String(error);
  } finally {
    oauthSubmit.disabled = false;
  }
});

oauthCancel.addEventListener("click", async () => {
  if (!oauthAttemptID) return;
  oauthCancel.disabled = true;
  try {
    await Accounts.CancelOAuth(oauthAttemptID);
    oauthMessage.textContent = t("oauth.msg.cancelling");
    await pollOAuthStatus();
  } catch (error) {
    oauthMessage.textContent = error?.message ?? String(error);
  } finally {
    oauthCancel.disabled = false;
  }
});

applyStaticTranslations();
await loadAccounts();
