package gui

import (
	"context"
	_ "embed"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/gongjs/agym/internal/accounts"
	"github.com/gongjs/agym/internal/appservice"
	"github.com/gongjs/agym/internal/projects"
)

//go:embed tray-template.png
var trayIconPNG []byte

// Run starts the pure-Go MyGo menu bar tray application.
func Run() error {
	hideConsoleWindow()
	roster, err := accounts.NewDefaultRoster()
	if err != nil {
		return err
	}
	if _, err := roster.RecoverOAuthSlot(context.Background(), accounts.NewOAuthAdder(roster).OwnerFile); err != nil {
		return err
	}
	svc := appservice.NewAccounts(roster)
	state := newUIState(svc)

	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.App.OnWindowAllClosed(func() {})

	var win *mygo.Window
	var quitting bool
	var pollMu sync.Mutex
	var pollStop chan struct{}

	stopPolling := func() {
		pollMu.Lock()
		if pollStop != nil {
			close(pollStop)
			pollStop = nil
		}
		pollMu.Unlock()
	}

	startPolling := func() {
		pollMu.Lock()
		if pollStop != nil {
			pollMu.Unlock()
			return
		}
		stopCh := make(chan struct{})
		pollStop = stopCh
		pollMu.Unlock()

		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stopCh:
					return
				case <-ticker.C:
					if state.isProjectsTab() && win != nil && win.IsVisible() {
						state.pollExistingProjects(win)
					}
				}
			}
		}()
	}

	svc.SetOnQuotasUpdated(func() {
		state.loadAccountsAsync(win, false)
	})

	mygo.App.OnBeforeQuit(func(_ *mygo.QuitEvent) {
		quitting = true
		stopPolling()
		svc.Shutdown()
	})

	mygo.App.WhenReady(func() {
		tray, trayErr := mygo.NewTray(mygo.TrayOptions{
			ID:             "agym-tray",
			Icon:           trayIconPNG,
			IconIsTemplate: true,
			ToolTip:        "Agy Manager",
		})
		if trayErr != nil {
			return
		}

		win = mygo.NewFlyout(mygo.FlyoutOptions{
			Tray:       tray,
			Width:      396,
			Height:     580,
			MinWidth:   380,
			MaxWidth:   420,
			MinHeight:  420,
			MaxHeight:  680,
			FitContent: false,
			Focusable:  true,
			Hidden:     true,
			Popover:    true,
			Content:    ui.View(state.view),
		})
		state.setWindow(win, startPolling, stopPolling)

		menu := mygo.NewMenu([]*mygo.MenuItem{
			{
				Label: "Open Agy Manager",
				Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
					win.Show()
				},
			},
			mygo.Separator(),
			{
				Label: "Quit Agy Manager",
				Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
					mygo.App.Quit()
				},
			},
		})
		tray.OnClick(func() {
			if win.IsVisible() {
				win.Hide()
			} else {
				win.Show()
			}
		})
		tray.OnRightClick(func() {
			tray.SetMenu(menu)
			tray.PopUpMenu()
			tray.SetMenu(nil)
		})

		win.OnClose(func(e *mygo.CloseEvent) {
			if !quitting {
				e.PreventDefault()
				win.Hide()
			}
		})
		win.OnShow(func() {
			if state.isProjectsTab() {
				state.loadProjectsAsync(win, false)
				startPolling()
			} else {
				state.loadAccountsAsync(win, false)
			}
		})
		win.OnHide(func() {
			stopPolling()
		})

		state.loadAccountsAsync(win, false)
		state.refreshQuotasAsync(win)
	})

	return mygo.App.Run()
}

func (s *uiState) setWindow(win *mygo.Window, startPoll, stopPoll func()) {
	s.mu.Lock()
	s.win = win
	s.startPoll = startPoll
	s.stopPoll = stopPoll
	s.mu.Unlock()
}

func (s *uiState) triggerUpdate(win *mygo.Window) {
	if win == nil {
		return
	}
	win.Update(func() {})
}

func (s *uiState) isProjectsTab() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tabIndex == 1
}

func (s *uiState) loadAccountsAsync(win *mygo.Window, showRefreshed bool) {
	if s.svc == nil {
		return
	}
	go func() {
		items, err := s.svc.ListAccounts()
		s.mu.Lock()
		if err != nil {
			s.statusMsg = err.Error()
			s.statusErr = true
		} else {
			s.accounts = items
			if !s.initAccountExpand && len(items) > 0 {
				s.initAccountExpand = true
				for _, a := range items {
					if a.Current {
						s.expandedAccounts[a.Email] = true
						break
					}
				}
				if len(s.expandedAccounts) == 0 {
					s.expandedAccounts[items[0].Email] = true
				}
			}
			if showRefreshed {
				s.statusMsg = s.tr("status.refreshed")
				s.statusErr = false
			}
		}
		s.mu.Unlock()
		s.triggerUpdate(win)
	}()
}

func (s *uiState) refreshQuotasAsync(win *mygo.Window) {
	if s.svc == nil {
		return
	}
	s.mu.Lock()
	if s.refreshing {
		s.mu.Unlock()
		return
	}
	s.refreshing = true
	s.statusMsg = s.tr("status.refreshing")
	s.statusErr = false
	s.mu.Unlock()
	s.triggerUpdate(win)

	go func() {
		items, err := s.svc.RefreshQuotas()
		s.mu.Lock()
		s.refreshing = false
		if err != nil {
			s.statusMsg = err.Error()
			s.statusErr = true
		} else {
			s.accounts = items
			s.statusMsg = s.tr("status.refreshed")
			s.statusErr = false
		}
		s.mu.Unlock()
		s.triggerUpdate(win)
	}()
}

func (s *uiState) refreshCurrentAccountQuotaAsync(win *mygo.Window) {
	if s.svc == nil {
		return
	}
	s.mu.Lock()
	if s.refreshing || s.currentQuotaBusy {
		s.mu.Unlock()
		return
	}
	s.currentQuotaBusy = true
	s.mu.Unlock()

	go func() {
		items, err := s.svc.RefreshCurrentQuota()
		s.mu.Lock()
		s.currentQuotaBusy = false
		if err == nil {
			s.accounts = items
		}
		s.mu.Unlock()
		if err == nil {
			s.triggerUpdate(win)
		}
	}()
}

func (s *uiState) loadProjectsAsync(win *mygo.Window, showRefreshed bool) {
	if s.svc == nil {
		return
	}
	s.mu.Lock()
	if !s.projectsLoaded {
		s.projectsLoading = true
	}
	s.mu.Unlock()
	go func() {
		items, err := s.svc.ListProjects()
		s.mu.Lock()
		s.projectsLoading = false
		s.projectsLoaded = true
		if err != nil {
			s.statusMsg = err.Error()
			s.statusErr = true
		} else {
			s.applyProjectListLocked(items)
			if showRefreshed {
				s.statusMsg = s.tr("projects.refreshed")
				s.statusErr = false
			}
		}
		s.mu.Unlock()
		s.triggerUpdate(win)
	}()
}

func (s *uiState) pollExistingProjects(win *mygo.Window) {
	if s.svc == nil {
		return
	}
	s.mu.Lock()
	existingCount := len(s.projects)
	s.mu.Unlock()
	if existingCount == 0 {
		return
	}
	fresh, err := s.svc.ListProjects()
	if err != nil {
		return
	}
	byPath := make(map[string]projects.ProjectView, len(fresh))
	for _, p := range fresh {
		byPath[p.Path] = p
	}
	s.mu.Lock()
	updated := make([]projects.ProjectView, len(s.projects))
	for i, p := range s.projects {
		if newer, ok := byPath[p.Path]; ok {
			updated[i] = newer
		} else {
			updated[i] = p
		}
	}
	s.projects = updated
	s.mu.Unlock()
	s.triggerUpdate(win)
}

func (s *uiState) applyProjectListLocked(items []projects.ProjectView) {
	s.projects = items
	if !s.initProjectExpand && len(items) > 0 {
		s.initProjectExpand = true
		chosen := items[0].Path
		for _, p := range items {
			if p.Active {
				chosen = p.Path
				break
			}
		}
		s.expandedProjects[chosen] = true
	}
}
