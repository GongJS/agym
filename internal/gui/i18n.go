package gui

import (
	"fmt"
	"strings"
	"time"
)

type langCode string

const (
	langZH langCode = "zh"
	langEN langCode = "en"
)

var messagesZH = map[string]string{
	"app.title":               "Agy Manager",
	"tab.accounts":            "账户",
	"tab.projects":            "项目",
	"lang.toggle":             "EN",
	"refresh.label":           "↻",
	"summary.loading":         "正在读取账户状态…",
	"summary.refreshing":      "正在刷新当前账户额度…",
	"summary.current":         "当前使用：%s",
	"summary.model":           "当前模型：%s",
	"summary.none":            "尚未配置账户，请先登录或添加已有账户。",
	"status.refreshing":       "正在刷新额度…",
	"status.refreshed":        "额度已更新",
	"status.switching":        "正在切换账户…",
	"status.switched":         "已切换到 %s",
	"status.adding":           "正在添加账户…",
	"status.added":            "已添加账户",
	"status.exists":           "该账户已在列表中",
	"status.removed":          "已移除 %s",
	"status.rotationOn":       "已开启 %s 自动轮换",
	"status.rotationOff":      "已关闭 %s 自动轮换",
	"account.current":         "当前",
	"account.rotation":        "自动轮换",
	"account.remove":          "移除",
	"account.noQuota":         "暂无额度数据（点击右上角刷新）",
	"account.resetAt":         "重置：%s",
	"quota.bucket.g5":         "5 小时",
	"quota.bucket.gw":         "每周",
	"quota.bucket.c5":         "5 小时",
	"quota.bucket.cw":         "每周",
	"quota.group.gemini":      "Gemini",
	"quota.group.claude":      "Claude · GPT",
	"quota.available":         "额度充足",
	"quota.exhausted":         "额度已用尽",
	"quota.resetsSoon":        "即将重置",
	"quota.refreshesIn":       "%s 后重置",
	"account.addPlaceholder":  "输入已有账户邮箱…",
	"account.add":             "添加",
	"account.oauthStart":      "登录新账户 (OAuth)",
	"oauth.starting":          "正在启动官方登录…",
	"oauth.waiting":           "等待浏览器授权…",
	"oauth.authorize":         "请在浏览器完成授权，并粘贴授权码：",
	"oauth.openLink":          "打开授权页",
	"oauth.codePlaceholder":   "粘贴授权码…",
	"oauth.submit":            "提交",
	"oauth.cancel":            "取消",
	"oauth.submitted":         "正在验证授权码…",
	"oauth.added":             "已登录并添加账户",
	"oauth.cancelled":         "已取消登录",
	"projects.emptyTitle":     "暂无项目记录",
	"projects.emptyDesc":      "使用官方 agy 在项目目录运行对话后，这里会自动展示上下文与 Token 统计。",
	"projects.running":        "运行中",
	"projects.idle":           "空闲",
	"projects.contextWindow":  "上下文窗口",
	"projects.legend.input":   "新增输入",
	"projects.legend.cache":   "缓存命中",
	"projects.legend.output":  "模型输出",
	"projects.legend.free":    "剩余空间",
	"projects.footer":         "%d 步 · 累计 %s",
	"projects.remove":         "移除",
	"projects.copied":         "已复制路径：%s",
	"projects.refreshed":      "项目状态已刷新",
	"footer.sharedCli":        "账户与自动轮换设置会与现有 CLI 共用。",
}

var messagesEN = map[string]string{
	"app.title":               "Agy Manager",
	"tab.accounts":            "Accounts",
	"tab.projects":            "Projects",
	"lang.toggle":             "中文",
	"refresh.label":           "↻",
	"summary.loading":         "Loading accounts…",
	"summary.refreshing":      "Refreshing current account quota…",
	"summary.current":         "Active: %s",
	"summary.model":           "Model: %s",
	"summary.none":            "No accounts configured yet.",
	"status.refreshing":       "Refreshing quotas…",
	"status.refreshed":        "Quotas updated",
	"status.switching":        "Switching account…",
	"status.switched":         "Switched to %s",
	"status.adding":           "Adding account…",
	"status.added":            "Account added",
	"status.exists":           "Account already exists",
	"status.removed":          "Removed %s",
	"status.rotationOn":       "Rotation enabled for %s",
	"status.rotationOff":      "Rotation disabled for %s",
	"account.current":         "Current",
	"account.rotation":        "Auto-rotate",
	"account.remove":          "Remove",
	"account.noQuota":         "No quota data yet (click refresh)",
	"account.resetAt":         "Resets: %s",
	"quota.bucket.g5":         "5h",
	"quota.bucket.gw":         "Weekly",
	"quota.bucket.c5":         "5h",
	"quota.bucket.cw":         "Weekly",
	"quota.group.gemini":      "Gemini",
	"quota.group.claude":      "Claude · GPT",
	"quota.available":         "Quota available",
	"quota.exhausted":         "Quota exhausted",
	"quota.resetsSoon":        "Resets soon",
	"quota.refreshesIn":       "Refreshes in %s",
	"account.addPlaceholder":  "Existing account email…",
	"account.add":             "Add",
	"account.oauthStart":      "Sign in new account (OAuth)",
	"oauth.starting":          "Starting official sign-in…",
	"oauth.waiting":           "Waiting for browser authorization…",
	"oauth.authorize":         "Complete sign-in in browser and paste code:",
	"oauth.openLink":          "Open Sign-in",
	"oauth.codePlaceholder":   "Paste authorization code…",
	"oauth.submit":            "Submit",
	"oauth.cancel":            "Cancel",
	"oauth.submitted":         "Verifying authorization code…",
	"oauth.added":             "Signed in and added account",
	"oauth.cancelled":         "Sign-in cancelled",
	"projects.emptyTitle":     "No project history",
	"projects.emptyDesc":      "Run official agy in a workspace to see context window and token stats here.",
	"projects.running":        "Running",
	"projects.idle":           "Idle",
	"projects.contextWindow":  "Context window",
	"projects.legend.input":   "Fresh input",
	"projects.legend.cache":   "Cache read",
	"projects.legend.output":  "Output",
	"projects.legend.free":    "Free space",
	"projects.footer":         "%d steps · Total %s",
	"projects.remove":         "Remove",
	"projects.copied":         "Copied path: %s",
	"projects.refreshed":      "Projects refreshed",
	"footer.sharedCli":        "Accounts and auto-rotation are shared with the CLI.",
}

func (s *uiState) tr(key string, args ...any) string {
	table := messagesZH
	if s.lang == langEN {
		table = messagesEN
	}
	msg, ok := table[key]
	if !ok {
		return key
	}
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

func formatTokens(num int64) string {
	if num < 0 {
		num = 0
	}
	if num >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(num)/1_000_000)
	}
	if num >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(num)/1_000)
	}
	return fmt.Sprintf("%d", num)
}

func formatPct(val, limit int64) string {
	if limit <= 0 || val <= 0 {
		return "0.0%"
	}
	pct := (float64(val) / float64(limit)) * 100
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf("%.1f%%", pct)
}

func quotaColorHex(pct int) string {
	if pct >= 50 {
		return "#34d399"
	}
	if pct >= 20 {
		return "#fbbf24"
	}
	return "#f87171"
}

func (s *uiState) bucketLabel(raw string) string {
	k := strings.ToLower(strings.TrimSpace(raw))
	if k == "" {
		return "Quota"
	}
	if translated := s.tr("quota.bucket." + k); translated != "quota.bucket."+k {
		return translated
	}
	return strings.TrimSpace(raw)
}

func (s *uiState) formatResetCountdown(pct int, resetStr string) string {
	raw := strings.TrimSpace(resetStr)
	if raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			t, err = time.Parse(time.RFC3339, raw)
		}
		if err == nil {
			if t.Year() <= 1970 && pct >= 100 {
				return s.tr("quota.available")
			}
			diff := time.Until(t)
			if diff <= 0 {
				return s.tr("quota.resetsSoon")
			}
			totalMins := int(diff.Minutes())
			hours := totalMins / 60
			mins := totalMins % 60
			if pct >= 100 && hours >= 24 {
				return s.tr("quota.available")
			}
			return s.tr("quota.refreshesIn", fmt.Sprintf("%dh %dm", hours, mins))
		}
		return raw
	}
	if pct <= 0 {
		return s.tr("quota.exhausted")
	}
	return s.tr("quota.available")
}
