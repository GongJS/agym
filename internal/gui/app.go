package gui

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/gongjs/agym/internal/accounts"
	"github.com/gongjs/agym/internal/appservice"
	"github.com/gongjs/agym/internal/projects"
	"github.com/gongjs/agym/internal/updater"
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
		mygo.GlobalShortcut.UnregisterAll()
		svc.Shutdown()
	})

	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{
				Label: "agym",
				Submenu: []*mygo.MenuItem{
					{Role: mygo.RoleQuit},
				},
			},
			{Role: mygo.RoleEditMenu},
		}))

		tray, trayErr := mygo.NewTray(mygo.TrayOptions{
			ID:             "agym-tray",
			Icon:           trayIconPNG,
			IconIsTemplate: true,
			ToolTip:        "Agy Manager",
		})
		if trayErr != nil {
			return
		}

		win = mygo.NewWindow(mygo.WindowOptions{
			Title:             "Agy Manager",
			Width:             630,
			Height:            500,
			MinWidth:          550,
			MinHeight:         400,
			MaxHeight:         820,
			Hidden:            false,
			TitleBarStyle:     mygo.TitleBarHiddenInset,
			DisableMinimize:   true,
			DisableMaximize:   true,
			DisableFullScreen: true,
			Content:           ui.View(state.view),
		})
		win.Center()
		state.setWindow(win, startPolling, stopPolling)
		state.applyShortcut(win)

		showWindow := func() {
			if win.IsMinimized() {
				win.Restore()
			}
			win.Show()
			win.Focus()
		}

		menu := mygo.NewMenu([]*mygo.MenuItem{
			{
				Label: "Open Agy Manager",
				Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
					showWindow()
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
			if win.IsVisible() && !win.IsMinimized() {
				win.Hide()
			} else {
				showWindow()
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
			state.clearTransientStatus()
			if state.isProjectsTab() {
				startPolling()
			} else {
				state.loadAccountsAsync(win, false)
			}
			// 每次打开托盘弹窗时，后台静默更新一次项目列表与账户额度
			state.loadProjectsAsync(win, false)
			state.refreshQuotasBackgroundAsync(win)
			state.checkForUpdatesAsync(win, false)
		})
		win.OnHide(func() {
			stopPolling()
		})

		state.loadAccountsAsync(win, false)
		state.checkForUpdatesAsync(win, false)
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
	// Standard 2-column window preserves user-chosen size rather than auto-shrinking.
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
	if s.loadAccountsBusy {
		s.loadAccountsAgain = true
		s.mu.Unlock()
		return
	}
	s.loadAccountsBusy = true
	gen := s.switchGen
	s.mu.Unlock()

	go func() {
		items, err := s.svc.ListAccounts()
		activeModel := readCurrentAgyModel()
		s.mu.Lock()
		s.loadAccountsBusy = false
		again := s.loadAccountsAgain
		s.loadAccountsAgain = false
		if s.applyAccountsLocked(gen, items) {
			s.currentAgyModel = activeModel
			if err != nil {
				s.statusMsg = err.Error()
				s.statusErr = true
			} else if showRefreshed {
				s.setStatusLocked(win, s.tr("status.refreshed"), false, true)
			}
		} else if err != nil && !s.switching && s.switchGen == gen {
			s.statusMsg = err.Error()
			s.statusErr = true
		}
		s.mu.Unlock()
		s.triggerUpdate(win)
		if again {
			s.loadAccountsAsync(win, false)
		}
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
		items, err := s.svc.RefreshQuotasIfDue()
		activeModel := readCurrentAgyModel()
		s.mu.Lock()
		s.refreshing = false
		updated := err == nil && s.applyAccountsLocked(gen, items)
		if updated {
			s.currentAgyModel = activeModel
		}
		s.mu.Unlock()
		if updated {
			s.triggerUpdate(win)
		}
	}()
}

func (s *uiState) refreshQuotasAsync(win *mygo.Window) {
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
	s.statusMsg = s.tr("status.refreshing")
	s.statusErr = false
	s.mu.Unlock()
	s.triggerUpdate(win)

	go func() {
		items, err := s.svc.RefreshQuotas()
		activeModel := readCurrentAgyModel()
		s.mu.Lock()
		s.refreshing = false
		if err != nil {
			s.statusMsg = err.Error()
			s.statusErr = true
		} else if s.applyAccountsLocked(gen, items) {
			s.currentAgyModel = activeModel
			s.setStatusLocked(win, s.tr("status.refreshed"), false, true)
		}
		s.mu.Unlock()
		s.triggerUpdate(win)
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
				s.setStatusLocked(win, s.tr("projects.refreshed"), false, true)
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
	fresh, err := s.svc.ListProjects()
	if err != nil {
		return
	}
	s.mu.Lock()
	if reflect.DeepEqual(s.projects, fresh) {
		s.mu.Unlock()
		return
	}
	s.applyProjectListLocked(fresh)
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

func (s *uiState) checkForUpdatesAsync(win *mygo.Window, manual bool) {
	s.mu.Lock()
	if s.updateChecking || s.updateApplying {
		s.mu.Unlock()
		return
	}
	s.updateChecking = true
	if manual {
		s.updateNotice = ""
	}
	s.mu.Unlock()
	if manual {
		s.triggerUpdate(win)
	}

	go func() {
		info, err := updater.CheckLatest(context.Background(), manual)
		s.mu.Lock()
		s.updateChecking = false
		if err != nil {
			if manual {
				s.statusMsg = err.Error()
				s.statusErr = true
			}
			s.mu.Unlock()
			if manual {
				s.triggerUpdate(win)
			}
			return
		}
		newer := updater.IsNewer(info.Version, updater.Version)
		s.updateAvailable = newer
		s.latestVersion = info.Version
		s.latestURL = info.URL
		if manual {
			if newer {
				s.updateNotice = ""
			} else if updater.Version == "dev" {
				s.updateNotice = s.tr("settings.devVersion", info.Version)
			} else {
				s.updateNotice = s.tr("settings.upToDate", updater.Version)
			}
		}
		s.mu.Unlock()
		if newer || manual {
			s.triggerUpdate(win)
		}
	}()
}

func (s *uiState) selfUpdateAsync(win *mygo.Window) {
	s.mu.Lock()
	if s.updateApplying {
		s.mu.Unlock()
		return
	}
	s.updateApplying = true
	s.statusMsg = s.tr("settings.updating")
	s.statusErr = false
	s.mu.Unlock()
	s.triggerUpdate(win)

	go func() {
		_, err := updater.SelfUpdate(context.Background())
		if err != nil {
			s.mu.Lock()
			s.updateApplying = false
			s.statusMsg = err.Error()
			s.statusErr = true
			s.mu.Unlock()
			s.triggerUpdate(win)
			return
		}
		exePath, exeErr := os.Executable()
		if exeErr == nil {
			if resolved, rErr := filepath.EvalSymlinks(exePath); rErr == nil {
				exePath = resolved
			}
			cmd := exec.Command(exePath, "ui")
			_ = cmd.Start()
		}
		mygo.App.Quit()
	}()
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
