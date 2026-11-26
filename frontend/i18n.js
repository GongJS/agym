const STORAGE_KEY = "agym.lang";

export const MESSAGES = {
  "zh-CN": {
    "app.title": "账户管理",
    "app.subtitle": "管理 agy 账户与配额",
    "lang.toggle": "EN",
    "lang.toggleTitle": "Switch to English",
    "refresh.label": "刷新额度",
    "summary.loading": "正在读取账户…",
    "summary.refreshing": "正在逐个查询官方 agy 额度…",
    "summary.current": "当前账户：{email}",
    "summary.none": "没有检测到当前 agy 账户",
    "summary.error": "无法读取账户信息",
    "section.accounts": "账户",
    "empty.title": "还没有配置账户",
    "empty.desc": "可在这里启动官方 agy 登录，或把已保存的账户加入名单。",
    "add.label": "添加已保存的账户",
    "add.placeholder": "name@example.com",
    "add.button": "添加",
    "add.hint": "只添加钥匙串已有备份；当前账号也可在上方安全保存。凭据不会传入前端。",
    "oauth.start": "使用官方 agy 登录新账户",
    "oauth.openLink": "在浏览器中打开 Google 登录",
    "oauth.codeLabel": "粘贴 Google 授权码",
    "oauth.codePlaceholder": "授权码",
    "oauth.submit": "提交",
    "oauth.cancel": "取消登录",
    "footer.note": "账户与自动轮换设置会与现有 CLI 共用。",
    "badge.current": "当前使用",
    "badge.savedLogin": "登录已保存",
    "badge.noLogin": "无登录备份",
    "badge.stale": "旧数据",
    "badge.staleTitle": "最近一次查询未成功",
    "badge.noModels": "暂无模型目录",
    "badge.defaultModel": "默认",
    "badge.defaultModelTitle": "{id}（默认模型）",
    "badge.tierTitle": "订阅等级",
    "badge.regionTitle": "当前出口地区",
    "model.selectAria": "为 {email} 选择模型",
    "model.defaultTag": "默认",
    "model.empty": "暂无可用模型",
    "thinking.selectAria": "为 {email} 选择思考等级",
    "thinking.default": "思考 默认",
    "thinking.low": "思考 低",
    "thinking.medium": "思考 中",
    "thinking.high": "思考 高",
    "status.modelSaved": "已记录 {email} 的模型偏好",
    "status.thinkingSaved": "已记录 {email} 的思考等级偏好",
    "quota.unfetched": "尚未刷新额度",
    "quota.status.ok": "额度已更新",
    "quota.status.stale": "显示上次额度",
    "quota.status.no_keychain": "没有登录备份",
    "quota.status.ineligible": "账号需验证",
    "quota.status.unsupported_region": "地区暂不支持",
    "quota.status.auth_failed": "登录已失效",
    "quota.status.timeout": "查询超时",
    "quota.status.parse_err": "额度报告无法解析",
    "quota.status.error": "查询失败",
    "quota.status.unknown": "额度状态未知",
    "quota.bucket.gw": "Gem 周",
    "quota.bucket.g5": "Gem 5h",
    "quota.bucket.cw": "Claude 周",
    "quota.bucket.c5": "Claude 5h",
    "quota.group.gemini": "GEMINI 模型",
    "quota.group.claude": "CLAUDE & GPT 模型",
    "quota.window.weekly": "每周剩余额度",
    "quota.window.5h": "5 小时剩余额度",
    "quota.available": "额度充足",
    "quota.exhausted": "额度已用尽",
    "quota.refreshesIn": "{time} 后重置",
    "quota.resetsSoon": "即将重置",
    "quota.reset": "重置：{reset}",
    "quota.verify": "验证",
    "account.rotation": "轮换",
    "account.rotationTitle": "允许自动轮换使用此账户",
    "account.rotationAria": "允许 {email} 自动轮换",
    "account.switchCurrent": "当前",
    "account.switch": "切换",
    "account.switchCurrentAria": "{email} 当前正在使用",
    "account.switchAria": "切换到 {email}",
    "account.removeTitle": "从管理名单移除（保留钥匙串登录）",
    "account.removeAria": "移除 {email}",
    "account.removeConfirm": "从账户名单移除 {email}？\n钥匙串中的登录备份会保留。",
    "account.saveAndAdd": "保存并添加",
    "account.saveAndAddAria": "保存并添加当前账户 {email}",
    "status.rotationSaved": "轮换设置已保存",
    "status.switching": "正在安全切换账户…",
    "status.switched": "已切换到 {email}；已运行的 agy 进程不会改变",
    "status.removed": "已从名单移除；登录备份保留",
    "status.savingCurrent": "正在验证当前登录并保存账户…",
    "status.savedCurrent": "当前账户已安全保存并添加，正在自动获取额度…",
    "status.alreadyInRoster": "当前账户已在名单中",
    "status.adding": "正在添加…",
    "status.added": "账户已添加，正在自动获取额度…",
    "status.exists": "这个账户已经在名单中",
    "status.refreshing": "查询期间会按账户暂时切换钥匙串登录，完成后恢复原账户",
    "status.refreshed": "配额刷新完成",
    "oauth.msg.starting": "正在启动官方 agy 登录…",
    "oauth.msg.waiting": "正在等待官方 agy 登录…",
    "oauth.msg.authorize": "请在浏览器中完成 Google 授权并粘贴授权码",
    "oauth.msg.added": "账号已安全保存并添加",
    "oauth.msg.cancelled": "登录已取消",
    "oauth.msg.submitted": "授权码已提交，正在验证登录…",
    "oauth.msg.cancelling": "正在取消并恢复原账户…"
  },
  en: {
    "app.title": "Agy Manager",
    "app.subtitle": "Manage agy accounts & quotas",
    "lang.toggle": "中文",
    "lang.toggleTitle": "切换到简体中文",
    "refresh.label": "Refresh quotas",
    "summary.loading": "Loading accounts…",
    "summary.refreshing": "Querying official agy quotas…",
    "summary.current": "Active: {email}",
    "summary.none": "No active agy account detected",
    "summary.error": "Failed to load accounts",
    "section.accounts": "Accounts",
    "empty.title": "No accounts configured",
    "empty.desc": "Sign in with official agy below, or add an already-saved account.",
    "add.label": "Add saved account",
    "add.placeholder": "name@example.com",
    "add.button": "Add",
    "add.hint": "Only adds accounts backed up in Keychain; active login can be saved above. Credentials never leave the backend.",
    "oauth.start": "Sign in with official agy",
    "oauth.openLink": "Open Google sign-in in browser",
    "oauth.codeLabel": "Paste Google authorization code",
    "oauth.codePlaceholder": "Authorization code",
    "oauth.submit": "Submit",
    "oauth.cancel": "Cancel sign-in",
    "footer.note": "Accounts and rotation settings are shared with the CLI.",
    "badge.current": "Active",
    "badge.savedLogin": "Login saved",
    "badge.noLogin": "No backup",
    "badge.stale": "Stale",
    "badge.staleTitle": "Last refresh did not succeed",
    "badge.noModels": "No model catalog",
    "badge.defaultModel": "Default",
    "badge.defaultModelTitle": "{id} (default model)",
    "badge.tierTitle": "Subscription tier",
    "badge.regionTitle": "Exit region",
    "model.selectAria": "Select model for {email}",
    "model.defaultTag": "default",
    "model.empty": "No models available",
    "thinking.selectAria": "Select thinking level for {email}",
    "thinking.default": "thinking default",
    "thinking.low": "thinking low",
    "thinking.medium": "thinking medium",
    "thinking.high": "thinking high",
    "status.modelSaved": "Saved model preference for {email}",
    "status.thinkingSaved": "Saved thinking preference for {email}",
    "quota.unfetched": "Quota not refreshed yet",
    "quota.status.ok": "Quota updated",
    "quota.status.stale": "Showing cached quota",
    "quota.status.no_keychain": "No login backup",
    "quota.status.ineligible": "Verification required",
    "quota.status.unsupported_region": "Unsupported region",
    "quota.status.auth_failed": "Auth expired",
    "quota.status.timeout": "Timed out",
    "quota.status.parse_err": "Unparseable quota",
    "quota.status.error": "Query failed",
    "quota.status.unknown": "Unknown quota status",
    "quota.bucket.gw": "Gem Wk",
    "quota.bucket.g5": "Gem 5h",
    "quota.bucket.cw": "Claude Wk",
    "quota.bucket.c5": "Claude 5h",
    "quota.group.gemini": "GEMINI MODELS",
    "quota.group.claude": "CLAUDE AND GPT MODELS",
    "quota.window.weekly": "Weekly Limit Remaining",
    "quota.window.5h": "Five Hour Limit Remaining",
    "quota.available": "Quota available",
    "quota.exhausted": "Quota exhausted",
    "quota.refreshesIn": "Refreshes in {time}",
    "quota.resetsSoon": "Refreshes soon",
    "quota.reset": "Resets: {reset}",
    "quota.verify": "Verify",
    "account.rotation": "Rotate",
    "account.rotationTitle": "Allow automatic rotation for this account",
    "account.rotationAria": "Allow automatic rotation for {email}",
    "account.switchCurrent": "Current",
    "account.switch": "Switch",
    "account.switchCurrentAria": "{email} is currently active",
    "account.switchAria": "Switch to {email}",
    "account.removeTitle": "Remove from roster (keeps Keychain backup)",
    "account.removeAria": "Remove {email}",
    "account.removeConfirm": "Remove {email} from the roster?\nIts Keychain login backup will be kept.",
    "account.saveAndAdd": "Save & Add",
    "account.saveAndAddAria": "Save and add active account {email}",
    "status.rotationSaved": "Rotation setting saved",
    "status.switching": "Safely switching account…",
    "status.switched": "Switched to {email}; running agy processes are unchanged",
    "status.removed": "Removed from roster; login backup kept",
    "status.savingCurrent": "Verifying active login and saving account…",
    "status.savedCurrent": "Active account saved & added, fetching quota…",
    "status.alreadyInRoster": "Active account is already in the roster",
    "status.adding": "Adding account…",
    "status.added": "Account added, fetching quota…",
    "status.exists": "Account is already in the roster",
    "status.refreshing": "Temporarily switching Keychain slots per account; original slot will be restored",
    "status.refreshed": "Quotas refreshed",
    "oauth.msg.starting": "Starting official agy sign-in…",
    "oauth.msg.waiting": "Waiting for official agy sign-in…",
    "oauth.msg.authorize": "Complete Google sign-in in browser and paste the code",
    "oauth.msg.added": "Account safely saved and added",
    "oauth.msg.cancelled": "Sign-in cancelled",
    "oauth.msg.submitted": "Authorization code submitted, verifying…",
    "oauth.msg.cancelling": "Cancelling and restoring original account…"
  }
};

let currentLang = detectInitialLang();

function detectInitialLang() {
  try {
    const saved = window.localStorage.getItem(STORAGE_KEY);
    if (saved && MESSAGES[saved]) return saved;
  } catch {
    // Ignore storage access errors in restricted webviews.
  }
  const nav = (navigator.language || "").toLowerCase();
  return nav.startsWith("zh") ? "zh-CN" : "en";
}

export function getLang() {
  return currentLang;
}

export function setLang(lang) {
  if (!MESSAGES[lang]) return currentLang;
  currentLang = lang;
  try {
    window.localStorage.setItem(STORAGE_KEY, lang);
  } catch {
    // Ignore storage access errors.
  }
  return currentLang;
}

export function toggleLang() {
  return setLang(currentLang === "zh-CN" ? "en" : "zh-CN");
}

export function t(key, params = {}) {
  const dict = MESSAGES[currentLang] ?? MESSAGES["zh-CN"];
  let text = dict[key] ?? MESSAGES["zh-CN"][key] ?? key;
  for (const [k, v] of Object.entries(params)) {
    text = text.replaceAll(`{${k}}`, String(v));
  }
  return text;
}
