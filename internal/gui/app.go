package gui

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	switch state.themeMode() {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	}

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
			Tray:         tray,
			Gap:          6,
			Width:        396,
			Height:       280,
			MinWidth:     396,
			MaxWidth:     396,
			MinHeight:    240,
			MaxHeight:    680,
			FitContent:   false,
			Focusable:    true,
			Hidden:       true,
			Shadow:       true,
			CornerRadius: 14,
			Popover:      false,
			Content:      ui.View(state.view),
		})
		state.setWindow(win, startPolling, stopPolling)

		positionFlyout := func() {
			tb := tray.Bounds()
			if tb.Width <= 0 && tb.Height <= 0 {
				return
			}
			w, h := win.Size()
			if w <= 0 {
				w = 396
			}
			if h <= 0 {
				h = 280
			}
			x := tb.X + (tb.Width-w)/2
			y := tb.Y + tb.Height + 6
			wa := mygo.Screen.DisplayNearestPoint(mygo.Point{X: tb.X, Y: tb.Y}).WorkArea
			if wa.Width > 0 {
				if x+w > wa.X+wa.Width-8 {
					x = wa.X + wa.Width - w - 8
				}
				if x < wa.X+8 {
					x = wa.X + 8
				}
				if y+h > wa.Y+wa.Height-8 {
					y = tb.Y - h - 6
				}
				if y < wa.Y+4 {
					y = wa.Y + 4
				}
			}
			win.SetPosition(x, y)
		}

		menu := mygo.NewMenu([]*mygo.MenuItem{
			{
				Label: "Open Agy Manager",
				Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
					positionFlyout()
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
				positionFlyout()
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
				startPolling()
			} else {
				state.loadAccountsAsync(win, false)
			}
			// 每次打开托盘弹窗时，后台静默更新一次项目列表与账户额度
			state.loadProjectsAsync(win, false)
			state.refreshQuotasBackgroundAsync(win)
		})
		win.OnHide(func() {
			stopPolling()
		})

		state.loadAccountsAsync(win, false)
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
	s.syncWindowHeight(win)
	win.Update(func() {})
}

func (s *uiState) syncWindowHeight(win *mygo.Window) {
	if win == nil {
		return
	}
	s.mu.Lock()
	hasContent := len(s.accounts) > 0 || len(s.projects) > 0 || s.showAddAccount || (s.oauth.State != "" && s.oauth.State != "idle")
	targetH := 280
	if hasContent {
		targetH = 540
	}
	changed := s.lastWindowHeight != targetH
	if changed {
		s.lastWindowHeight = targetH
	}
	s.mu.Unlock()
	if changed {
		win.SetSize(396, targetH)
	}
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
	s.mu.Lock()
	gen := s.switchGen
	s.mu.Unlock()
	go func() {
		items, err := s.svc.ListAccounts()
		activeModel := readCurrentAgyModel()
		s.mu.Lock()
		if s.switching || s.switchGen != gen {
			s.mu.Unlock()
			return
		}
		s.currentAgyModel = activeModel
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

func (s *uiState) refreshQuotasBackgroundAsync(win *mygo.Window) {
	if s.svc == nil {
		return
	}
	s.mu.Lock()
	if s.refreshing || s.switching {
		s.mu.Unlock()
		return
	}
	gen := s.switchGen
	s.refreshing = true
	s.mu.Unlock()

	go func() {
		items, err := s.svc.RefreshQuotas()
		activeModel := readCurrentAgyModel()
		s.mu.Lock()
		s.refreshing = false
		if err == nil && !s.switching && s.switchGen == gen {
			s.accounts = items
			s.currentAgyModel = activeModel
		}
		s.mu.Unlock()
		if err == nil {
			s.triggerUpdate(win)
		}
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

func (s *uiState) loadProjectsSyncIfNeeded() bool {
	if s.svc == nil {
		return true
	}
	s.mu.Lock()
	loaded := s.projectsLoaded
	s.mu.Unlock()
	if loaded {
		return true
	}
	items, err := s.svc.ListProjects()
	s.mu.Lock()
	s.projectsLoading = false
	s.projectsLoaded = true
	if err != nil {
		s.statusMsg = err.Error()
		s.statusErr = true
	} else {
		s.applyProjectListLocked(items)
	}
	s.mu.Unlock()
	return false
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

func readCurrentAgyModel() string {
	agyDir, err := projects.DefaultAgyDir()
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(agyDir, "settings.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return ""
	}
	return strings.TrimSpace(cfg.Model)
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
