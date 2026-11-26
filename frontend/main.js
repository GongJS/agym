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
  tabAccounts,
  tabProjects,
  viewAccounts,
  viewProjects,
  projectList,
  projectEmpty,
} = collectElements({
  list: "#account-list",
  emptyState: "#empty-state",
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
  tabAccounts: "#tab-accounts",
  tabProjects: "#tab-projects",
  viewAccounts: "#view-accounts",
  viewProjects: "#view-projects",
  projectList: "#project-list",
  projectEmpty: "#project-empty",
});

let currentTab = "accounts";
let latestProjects = [];
const expandedProjects = new Set();
let hasInitializedProjectsExpand = false;
let projectPollTimer = null;

let oauthAttemptID = "";
let oauthPollTimer = null;
let handledOAuthAddedAttemptID = "";
let latestAccounts = [];
let latestSummary = { key: "summary.loading", params: {} };
let latestStatus = { key: "", params: {}, raw: "", isError: false };
let latestOAuthView = null;
const expandedEmails = new Set();
let hasInitializedExpand = false;

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
  renderProjectList(latestProjects);
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

function formatResetCountdown(bucket) {
  if (!bucket) return "";
  if (bucket.reset) {
    const ms = Date.parse(bucket.reset);
    if (!Number.isNaN(ms)) {
      const diffMs = ms - Date.now();
      if (diffMs <= 0) return t("quota.resetsSoon");
      const totalMins = Math.floor(diffMs / 60000);
      const hours = Math.floor(totalMins / 60);
      const mins = totalMins % 60;
      if (bucket.percent >= 100 && hours >= 24) {
        return t("quota.available");
      }
      return t("quota.refreshesIn", { time: `${hours}h ${mins}m` });
    }
  }
  return bucket.percent <= 0 ? t("quota.exhausted") : t("quota.available");
}

function tierClassForPercent(percent) {
  if (percent < 20) return "low";
  if (percent < 50) return "warn";
  return "";
}

function formatCollapsedSummary(account) {
  const quota = account.quota;
  if (!quota?.buckets) return quotaLabel(account);
  const parts = [];
  const g5 = quota.buckets.g5;
  const gw = quota.buckets.gw;
  const gPct = g5 && g5.percent >= 0 ? g5.percent : (gw && gw.percent >= 0 ? gw.percent : -1);
  if (gPct >= 0) parts.push(`G ${gPct}%`);

  const c5 = quota.buckets.c5;
  const cw = quota.buckets.cw;
  const cPct = c5 && c5.percent >= 0 ? c5.percent : (cw && cw.percent >= 0 ? cw.percent : -1);
  if (cPct >= 0) parts.push(`C ${cPct}%`);

  return parts.length ? parts.join(" · ") : quotaLabel(account);
}

function renderQuotaWindow(labelKey, bucket) {
  const pct = Math.max(0, Math.min(100, bucket.percent));
  const tierClass = tierClassForPercent(pct);

  const wrap = document.createElement("div");
  wrap.className = "quota-window";
  if (bucket.reset) wrap.title = t("quota.reset", { reset: bucket.reset });

  const top = document.createElement("div");
  top.className = "quota-window-top";
  const label = document.createElement("span");
  label.className = "quota-window-label";
  label.textContent = t(labelKey);
  const pctEl = document.createElement("span");
  pctEl.className = "quota-window-pct";
  pctEl.textContent = `${pct}%`;
  top.append(label, pctEl);

  const track = document.createElement("div");
  track.className = "quota-bar-track";
  const fill = document.createElement("div");
  fill.className = `quota-bar-fill ${tierClass}`.trim();
  fill.style.width = `${pct}%`;
  track.append(fill);

  const reset = document.createElement("div");
  reset.className = `quota-window-reset ${tierClass}`.trim();
  reset.textContent = formatResetCountdown(bucket);

  wrap.append(top, track, reset);
  return wrap;
}

function renderQuota(account, container) {
  const quota = account.quota;
  const panel = document.createElement("div");
  panel.className = "quota-panel";
  if (!quota) {
    const row = document.createElement("div");
    row.className = "quota-status-row";
    row.append(makeBadge(t("quota.unfetched"), "muted"));
    panel.append(row);
    container.append(panel);
    return;
  }

  const groups = [
    {
      titleKey: "quota.group.gemini",
      members: quota.members?.gemini,
      windows: [
        { key: "g5", labelKey: "quota.window.5h" },
        { key: "gw", labelKey: "quota.window.weekly" },
      ],
    },
    {
      titleKey: "quota.group.claude",
      members: quota.members?.claude,
      windows: [
        { key: "c5", labelKey: "quota.window.5h" },
        { key: "cw", labelKey: "quota.window.weekly" },
      ],
    },
  ];

  let renderedGroups = 0;
  for (const grp of groups) {
    const activeWindows = grp.windows.filter(({ key }) => {
      const b = quota.buckets?.[key];
      return b && b.percent >= 0;
    });
    if (!activeWindows.length) continue;
    renderedGroups += 1;

    const card = document.createElement("div");
    card.className = "quota-group";

    const head = document.createElement("div");
    head.className = "quota-group-head";
    const title = document.createElement("div");
    title.className = "quota-group-title";
    title.textContent = t(grp.titleKey);
    head.append(title);
    if (grp.members?.length) {
      const models = document.createElement("div");
      models.className = "quota-group-models";
      models.textContent = grp.members.join(", ");
      models.title = models.textContent;
      head.append(models);
    }
    card.append(head);

    const grid = document.createElement("div");
    grid.className = "quota-grid";
    for (const { key, labelKey } of activeWindows) {
      grid.append(renderQuotaWindow(labelKey, quota.buckets[key]));
    }
    card.append(grid);
    panel.append(card);
  }

  if (!renderedGroups || quota.status === "stale" || (quota.status === "ineligible" && quota.error?.startsWith("https://"))) {
    const statusRow = document.createElement("div");
    statusRow.className = "quota-status-row";
    if (!renderedGroups) {
      statusRow.append(
        makeBadge(
          quotaLabel(account),
          quota.status === "error" || quota.status === "timeout" ? "missing" : "muted",
          quota.error ?? ""
        )
      );
    }
    if (quota.status === "stale") {
      statusRow.append(makeBadge(t("badge.stale"), "muted", quota.error ?? t("badge.staleTitle")));
    }
    if (quota.status === "ineligible" && quota.error?.startsWith("https://")) {
      const verify = document.createElement("a");
      verify.className = "verify-link";
      verify.href = quota.error;
      verify.target = "_blank";
      verify.rel = "noreferrer";
      verify.textContent = t("quota.verify");
      statusRow.append(verify);
    }
    panel.append(statusRow);
  }

  container.append(panel);
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
  const isExpanded = expandedEmails.has(account.email);
  const card = document.createElement("article");
  card.className = `account-card${account.current ? " is-current" : ""}${isExpanded ? " expanded" : ""}`;
  const needsAddCurrent = account.current && !account.inRoster;

  const header = document.createElement("div");
  header.className = "card-header";

  const top = document.createElement("div");
  top.className = "row-top";

  // Radio 单选切换账户
  const radioLabel = document.createElement("label");
  radioLabel.className = "account-radio";
  radioLabel.title = account.current ? t("account.switchCurrentAria", { email: account.email }) : t("account.selectAria", { email: account.email });
  const radioInput = document.createElement("input");
  radioInput.type = "radio";
  radioInput.name = "active-account";
  radioInput.checked = account.current;
  radioInput.disabled = !account.hasSavedLogin && !account.current;
  radioInput.setAttribute("aria-label", t("account.selectAria", { email: account.email }));
  radioInput.addEventListener("click", (e) => e.stopPropagation());
  radioInput.addEventListener("change", async () => {
    if (account.current || !radioInput.checked) return;
    radioInput.disabled = true;
    setStatusKey("status.switching");
    try {
      await Accounts.SwitchAccount(account.email);
      setStatusKey("status.switched", { email: account.email });
      expandedEmails.add(account.email);
      await loadAccounts();
    } catch (error) {
      setStatusRaw(error?.message ?? String(error), true);
      await loadAccounts();
    }
  });
  const radioDot = document.createElement("span");
  radioDot.className = "radio-dot";
  radioLabel.append(radioInput, radioDot);

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

  const headerRight = document.createElement("div");
  headerRight.className = "header-right";

  const addCurrentButton = document.createElement("button");
  addCurrentButton.className = "current-add-button";
  addCurrentButton.type = "button";
  addCurrentButton.textContent = t("account.saveAndAdd");
  addCurrentButton.hidden = !needsAddCurrent;
  addCurrentButton.setAttribute("aria-label", t("account.saveAndAddAria", { email: account.email }));
  addCurrentButton.addEventListener("click", async (e) => {
    e.stopPropagation();
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

  const chevronBtn = document.createElement("button");
  chevronBtn.className = "chevron-btn";
  chevronBtn.type = "button";
  chevronBtn.setAttribute("aria-expanded", String(isExpanded));
  chevronBtn.setAttribute("aria-label", t("account.expandAria", { email: account.email }));
  const chevronIcon = document.createElement("span");
  chevronIcon.className = "chevron-icon";
  chevronIcon.textContent = "›";
  chevronBtn.append(chevronIcon);

  headerRight.append(addCurrentButton, chevronBtn);
  top.append(radioLabel, avatar, meta, headerRight);

  const sub = document.createElement("div");
  sub.className = "header-sub";
  const badges = document.createElement("div");
  badges.className = "row-badges";
  if (account.current) badges.append(makeBadge(t("badge.current"), "current"));
  if (!account.hasSavedLogin) badges.append(makeBadge(t("badge.noLogin"), "missing"));
  if (account.quota?.tierName) {
    badges.append(makeBadge(account.quota.tierName, "tier", account.quota.tierId || t("badge.tierTitle")));
  } else if (account.quota) {
    badges.append(makeBadge(t("badge.free"), "free", account.quota.tierId || t("badge.tierTitle")));
  }
  if (account.quota?.regionCode) badges.append(makeBadge(account.quota.regionCode, "region", t("badge.regionTitle")));
  const rotationBadge = makeBadge(t("account.rotation"), "rotation-pill");
  rotationBadge.hidden = !account.rotation || needsAddCurrent;
  badges.append(rotationBadge);

  const quotaSummary = document.createElement("span");
  quotaSummary.className = "quota-summary";
  quotaSummary.textContent = formatCollapsedSummary(account);

  sub.append(badges, quotaSummary);
  header.append(top, sub);

  // 展开区（带平滑动画）
  const detailsWrap = document.createElement("div");
  detailsWrap.className = "card-details-wrap";
  const detailsInner = document.createElement("div");
  detailsInner.className = "card-details-inner";
  const detailsBody = document.createElement("div");
  detailsBody.className = "card-details-body";

  renderQuota(account, detailsBody);
  renderControlsRow(account, detailsBody);

  const detailsFooter = document.createElement("div");
  detailsFooter.className = "details-footer";
  detailsFooter.hidden = needsAddCurrent;

  const switchLabel = document.createElement("label");
  switchLabel.className = "switch-label";
  switchLabel.title = t("account.rotationTitle");
  const switchSpan = document.createElement("span");
  switchSpan.className = "switch";
  const rotation = document.createElement("input");
  rotation.type = "checkbox";
  rotation.checked = account.rotation;
  rotation.setAttribute("aria-label", t("account.rotationAria", { email: account.email }));
  rotation.addEventListener("change", async () => {
    rotation.disabled = true;
    try {
      await Accounts.SetRotation(account.email, rotation.checked);
      account.rotation = rotation.checked;
      rotationBadge.hidden = !account.rotation || needsAddCurrent;
      setStatusKey("status.rotationSaved");
    } catch (error) {
      rotation.checked = !rotation.checked;
      setStatusRaw(error?.message ?? String(error), true);
    } finally {
      rotation.disabled = false;
    }
  });
  const slider = document.createElement("span");
  slider.className = "switch-slider";
  switchSpan.append(rotation, slider);
  switchLabel.append(switchSpan, document.createTextNode(t("account.rotation")));

  const remove = document.createElement("button");
  remove.className = "remove-text-button";
  remove.type = "button";
  remove.title = t("account.removeTitle");
  remove.setAttribute("aria-label", t("account.removeAria", { email: account.email }));
  remove.textContent = getLang() === "zh-CN" ? "移除" : "Remove";
  remove.addEventListener("click", async () => {
    if (!window.confirm(t("account.removeConfirm", { email: account.email }))) return;
    remove.disabled = true;
    try {
      await Accounts.RemoveAccount(account.email);
      expandedEmails.delete(account.email);
      setStatusKey("status.removed");
      await loadAccounts();
    } catch (error) {
      setStatusRaw(error?.message ?? String(error), true);
      remove.disabled = false;
    }
  });

  detailsFooter.append(switchLabel, remove);
  detailsBody.append(detailsFooter);
  detailsInner.append(detailsBody);
  detailsWrap.append(detailsInner);

  const toggleExpand = () => {
    const next = !card.classList.contains("expanded");
    card.classList.toggle("expanded", next);
    chevronBtn.setAttribute("aria-expanded", String(next));
    if (next) {
      expandedEmails.add(account.email);
    } else {
      expandedEmails.delete(account.email);
    }
  };
  header.addEventListener("click", toggleExpand);
  chevronBtn.addEventListener("click", (e) => {
    e.stopPropagation();
    toggleExpand();
  });

  card.append(header, detailsWrap);
  return card;
}

function renderAccountList(accounts) {
  latestAccounts = accounts;
  if (!hasInitializedExpand && accounts.length > 0) {
    hasInitializedExpand = true;
    const active = accounts.find((a) => a.current) ?? accounts[0];
    if (active) expandedEmails.add(active.email);
  }
  list.replaceChildren(...accounts.map(renderAccount));
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
      expandedEmails.add(email);
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

refreshButton.addEventListener("click", () => {
  if (currentTab === "projects") {
    void loadProjects(true);
  } else {
    void refreshQuotas();
  }
});

function formatTokens(num) {
  const n = Number(num || 0);
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

function renderProjectCard(project) {
  const isExpanded = expandedProjects.has(project.path);
  const card = document.createElement("article");
  card.className = `project-card${project.active ? " is-active" : ""}${isExpanded ? " expanded" : ""}`;

  const header = document.createElement("div");
  header.className = "project-header";

  const top = document.createElement("div");
  top.className = "project-top";
  const dot = document.createElement("span");
  dot.className = "project-dot";
  dot.title = project.active ? t("projects.running") : t("projects.idle");

  const name = document.createElement("div");
  name.className = "project-name";
  name.textContent = project.name;
  name.title = project.name;

  const modelPill = document.createElement("span");
  modelPill.className = "model-pill";
  modelPill.textContent = project.model || (project.active ? t("projects.running") : t("projects.idle"));
  modelPill.title = modelPill.textContent;

  const chevronBtn = document.createElement("button");
  chevronBtn.className = "chevron-btn";
  chevronBtn.type = "button";
  chevronBtn.setAttribute("aria-expanded", String(isExpanded));
  const chevronIcon = document.createElement("span");
  chevronIcon.className = "chevron-icon";
  chevronIcon.textContent = "›";
  chevronBtn.append(chevronIcon);

  top.append(dot, name, modelPill, chevronBtn);

  const pathEl = document.createElement("div");
  pathEl.className = "project-path";
  pathEl.textContent = project.path;
  pathEl.title = project.path;
  pathEl.addEventListener("click", async (e) => {
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(project.path);
      setStatusKey("projects.copied", { path: project.name });
    } catch {
      // Ignore clipboard failure in restricted context.
    }
  });

  header.append(top, pathEl);

  const detailsWrap = document.createElement("div");
  detailsWrap.className = "project-details-wrap";
  const detailsInner = document.createElement("div");
  detailsInner.className = "card-details-inner";

  const box = document.createElement("div");
  box.className = "context-box";

  const limit = Math.max(1, project.contextLimit || 1_000_000);
  const freshInput = Math.max(0, project.latestInput || 0);
  const cacheRead = Math.max(0, project.latestCacheRead || 0);
  const latestOutput = Math.max(0, project.latestOutput || 0);
  const contextUsed = freshInput + cacheRead;
  const freeTokens = Math.max(0, limit - contextUsed);
  const pctNum = Math.min(100, (contextUsed / limit) * 100);
  const formatPct = (val) => `${Math.min(100, (Math.max(0, val) / limit) * 100).toFixed(1)}%`;

  const ctxHead = document.createElement("div");
  ctxHead.className = "context-head";
  const hero = document.createElement("div");
  hero.className = "context-hero";
  const numEl = document.createElement("span");
  numEl.className = "context-num";
  numEl.textContent = formatTokens(contextUsed);
  const limitEl = document.createElement("span");
  limitEl.className = "context-limit";
  limitEl.textContent = `/ ${formatTokens(limit)}`;
  const titleEl = document.createElement("span");
  titleEl.className = "context-title";
  titleEl.textContent = t("projects.contextWindow");
  hero.append(numEl, limitEl, titleEl);

  const pctBadge = document.createElement("span");
  pctBadge.className = `context-pct${pctNum < 40 ? " ok" : pctNum >= 80 ? " high" : ""}`;
  pctBadge.textContent = `${pctNum.toFixed(1)}%`;
  ctxHead.append(hero, pctBadge);

  // 80 个点阵单元（20x4），非零项至少保留 1 格以便区分颜色
  const totalCells = 80;
  const freshCells = Math.min(totalCells, freshInput > 0 ? Math.max(1, Math.round((freshInput / limit) * totalCells)) : 0);
  const cacheCells = Math.min(totalCells - freshCells, cacheRead > 0 ? Math.max(1, Math.round((cacheRead / limit) * totalCells)) : 0);
  const outCells = Math.min(totalCells - freshCells - cacheCells, latestOutput > 0 ? Math.max(1, Math.round((latestOutput / limit) * totalCells)) : 0);

  const matrix = document.createElement("div");
  matrix.className = "dot-matrix";
  for (let i = 0; i < totalCells; i++) {
    const cell = document.createElement("span");
    let cls = "matrix-cell";
    if (i < freshCells) cls += " fresh";
    else if (i < freshCells + cacheCells) cls += " cache";
    else if (i < freshCells + cacheCells + outCells) cls += " output";
    cell.className = cls;
    matrix.append(cell);
  }

  const legend = document.createElement("div");
  legend.className = "ctx-legend";
  const legendItems = [
    { cls: "fresh", label: t("projects.legend.input"), val: `${formatTokens(freshInput)} (${formatPct(freshInput)})` },
    { cls: "cache", label: t("projects.legend.cache"), val: `${formatTokens(cacheRead)} (${formatPct(cacheRead)})` },
    { cls: "output", label: t("projects.legend.output"), val: `${formatTokens(latestOutput)} (${formatPct(latestOutput)})` },
    { cls: "free", label: t("projects.legend.free"), val: `${formatTokens(freeTokens)} (${formatPct(freeTokens)})` },
  ];
  for (const item of legendItems) {
    const r = document.createElement("div");
    r.className = "ctx-row";
    const left = document.createElement("span");
    left.className = "ctx-label";
    const sw = document.createElement("span");
    sw.className = `ctx-swatch ${item.cls}`;
    left.append(sw, document.createTextNode(item.label));
    const val = document.createElement("span");
    val.className = "ctx-val";
    val.textContent = item.val;
    r.append(left, val);
    legend.append(r);
  }

  const foot = document.createElement("div");
  foot.className = "context-foot";
  const totalTokens = (project.totalInput || 0) + (project.totalCacheRead || 0) + (project.totalOutput || 0);
  const footLeft = document.createElement("span");
  footLeft.textContent = t("projects.footer", { steps: project.stepCount || 0, total: formatTokens(totalTokens) });
  const footRight = document.createElement("span");
  footRight.className = "context-foot-right";
  const statusText = document.createElement("span");
  statusText.textContent = project.active ? t("projects.running") : t("projects.idle");
  const removeBtn = document.createElement("button");
  removeBtn.type = "button";
  removeBtn.className = "project-remove-btn";
  removeBtn.textContent = t("projects.remove");
  removeBtn.addEventListener("click", async (e) => {
    e.stopPropagation();
    removeBtn.disabled = true;
    try {
      await Accounts.RemoveProject(project.path);
      expandedProjects.delete(project.path);
      renderProjectList(latestProjects.filter((p) => p.path !== project.path));
    } catch (error) {
      removeBtn.disabled = false;
      setStatusRaw(error?.message ?? String(error), true);
    }
  });
  footRight.append(statusText, removeBtn);
  foot.append(footLeft, footRight);

  box.append(ctxHead, matrix, legend, foot);
  detailsInner.append(box);
  detailsWrap.append(detailsInner);

  const toggleProject = () => {
    const next = !card.classList.contains("expanded");
    card.classList.toggle("expanded", next);
    chevronBtn.setAttribute("aria-expanded", String(next));
    if (next) expandedProjects.add(project.path);
    else expandedProjects.delete(project.path);
  };
  header.addEventListener("click", toggleProject);
  chevronBtn.addEventListener("click", (e) => {
    e.stopPropagation();
    toggleProject();
  });

  card.append(header, detailsWrap);
  return card;
}

function renderProjectList(projects) {
  latestProjects = projects;
  if (!hasInitializedProjectsExpand && projects.length > 0) {
    hasInitializedProjectsExpand = true;
    const active = projects.find((p) => p.active) ?? projects[0];
    if (active) expandedProjects.add(active.path);
  }
  projectList.replaceChildren(...projects.map(renderProjectCard));
  projectEmpty.hidden = projects.length !== 0;
}

async function loadProjects(showStatus = false) {
  try {
    const projects = await Accounts.ListProjects();
    renderProjectList(projects);
    if (showStatus) setStatusKey("projects.refreshed");
  } catch (error) {
    renderProjectList([]);
    setStatusRaw(error?.message ?? String(error), true);
  }
}

async function pollProjects() {
  if (latestProjects.length === 0) return;
  try {
    const fresh = await Accounts.ListProjects();
    const byPath = new Map(fresh.map((p) => [p.path, p]));
    const updated = latestProjects.map((p) => byPath.get(p.path) ?? p);
    renderProjectList(updated);
  } catch {
    // 轮询失败时保留当前列表，避免界面闪烁
  }
}

function stopProjectPolling() {
  if (projectPollTimer) {
    clearInterval(projectPollTimer);
    projectPollTimer = null;
  }
}

function startProjectPolling() {
  stopProjectPolling();
  projectPollTimer = setInterval(() => {
    if (currentTab === "projects" && document.visibilityState === "visible") {
      void pollProjects();
    }
  }, 5000);
}

window.addEventListener("blur", stopProjectPolling);
window.addEventListener("focus", () => {
  if (currentTab === "projects") {
    void loadProjects(false);
    startProjectPolling();
  }
});

function switchTab(tab) {
  currentTab = tab;
  const isProjects = tab === "projects";
  tabAccounts.classList.toggle("active", !isProjects);
  tabProjects.classList.toggle("active", isProjects);
  tabAccounts.setAttribute("aria-selected", String(!isProjects));
  tabProjects.setAttribute("aria-selected", String(isProjects));
  viewAccounts.hidden = isProjects;
  viewProjects.hidden = !isProjects;
  if (isProjects) {
    void loadProjects(false);
    startProjectPolling();
  } else {
    stopProjectPolling();
  }
}

tabAccounts.addEventListener("click", () => switchTab("accounts"));
tabProjects.addEventListener("click", () => switchTab("projects"));
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
    if (view.email) expandedEmails.add(view.email);
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
