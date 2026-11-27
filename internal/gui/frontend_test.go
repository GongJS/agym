package gui

import (
	"testing"

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

	// Verify polished states: error banner, un-switchable account, confirm remove, expanded onboarding
	state.tabIndex = 0
	state.accounts = append(state.accounts, appservice.AccountView{
		Email:         "secondary@example.com",
		Current:       false,
		HasSavedLogin: false,
	})
	state.expandedAccounts["secondary@example.com"] = true
	state.confirmRemoveEmail = "secondary@example.com"
	state.showAddAccount = true
	state.statusMsg = "network timeout while refreshing quota"
	state.statusErr = true
	imgPolished := ui.Render(state.view, 396, 560, 1)
	if imgPolished == nil || imgPolished.Bounds().Empty() {
		t.Fatal("expected non-empty rendered image for polished accounts states")
	}
}
