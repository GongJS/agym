package gui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/gongjs/agym/internal/appservice"
	"github.com/gongjs/agym/internal/projects"
	"github.com/gongjs/agym/internal/usage"
)

func TestNativeUIRendersAccountsAndProjects(t *testing.T) {
	state := newUIState(nil)
	state.accounts = []appservice.AccountView{
		{
			Email:         "user@example.com",
			Current:       true,
			HasSavedLogin: true,
			Quota: &usage.AccountView{
				TierName: "Free",
				Buckets: map[string]usage.Bucket{
					"Gemini": {Percent: 80, Reset: "2h"},
				},
			},
		},
	}
	state.expandedAccounts["user@example.com"] = true
	state.projects = []projects.ProjectView{
		{
			Name:            "agym",
			Path:            "/tmp/agym",
			Active:          true,
			Model:           "gemini-4-hydrogen-medium",
			StepCount:       12,
			ContextLimit:    1_000_000,
			LatestInput:     4200,
			LatestCacheRead: 29900,
			LatestOutput:    563,
		},
	}
	state.expandedProjects["/tmp/agym"] = true

	imgAccounts := ui.Render(state.view, 396, 560, 1)
	if imgAccounts == nil || imgAccounts.Bounds().Empty() {
		t.Fatal("expected non-empty rendered image for accounts tab")
	}

	state.tabIndex = 1
	imgProjects := ui.Render(state.view, 396, 560, 1)
	if imgProjects == nil || imgProjects.Bounds().Empty() {
		t.Fatal("expected non-empty rendered image for projects tab")
	}

	// Verify polished states: error banner, un-switchable account, confirm remove, expanded onboarding, stale & failed quota
	state.tabIndex = 0
	state.accounts = append(state.accounts,
		appservice.AccountView{
			Email:         "stale@example.com",
			Current:       false,
			HasSavedLogin: true,
			Quota: &usage.AccountView{
				Status:    "stale",
				CheckedAt: time.Now().Add(-15 * time.Minute).Unix(),
				Error:     "agy usage query timed out",
				Buckets: map[string]usage.Bucket{
					"g5": {Percent: 45, Reset: "2h"},
				},
			},
		},
		appservice.AccountView{
			Email:         "secondary@example.com",
			Current:       false,
			HasSavedLogin: false,
			Quota: &usage.AccountView{
				Status: "timeout",
				Error:  "agy usage query timed out",
			},
		},
	)
	state.expandedAccounts["stale@example.com"] = true
	state.expandedAccounts["secondary@example.com"] = true
	state.confirmRemoveEmail = "secondary@example.com"
	state.showAddAccount = true
	state.statusMsg = "network timeout while refreshing quota"
	state.statusErr = true
	imgPolished := ui.Render(state.view, 630, 500, 1)
	if imgPolished == nil || imgPolished.Bounds().Empty() {
		t.Fatal("expected non-empty rendered image for polished accounts states")
	}

	state.tabIndex = 2
	imgSettings := ui.Render(state.view, 630, 500, 1)
	if imgSettings == nil || imgSettings.Bounds().Empty() {
		t.Fatal("expected non-empty rendered image for settings tab")
	}
}
