package gui

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/gongjs/agym/internal/accounts"
	"github.com/gongjs/agym/internal/appservice"
	"github.com/gongjs/agym/internal/projects"
)

type palette struct {
	bg          ui.Color
	card        ui.Color
	cardSub     ui.Color
	soft        ui.Color
	fg          ui.Color
	muted       ui.Color
	line        ui.Color
	accent      ui.Color
	accentSoft  ui.Color
	ok          ui.Color
	danger      ui.Color
	dangerSoft  ui.Color
	track       ui.Color
	fresh       ui.Color
	cache       ui.Color
	output      ui.Color
	pillBg      ui.Color
	pillText    ui.Color
	warnBg      ui.Color
	warnText    ui.Color
	highBg      ui.Color
}

func currentPalette(c *ui.Context) palette {
	dark := c.Theme().Background.R < 128
	if dark {
		return palette{
			bg:         ui.RGB(24, 26, 32),
			card:       ui.RGBA(40, 44, 54, 0.92),
			cardSub:    ui.RGBA(28, 31, 38, 0.90),
			soft:       ui.RGBA(255, 255, 255, 0.08),
			fg:         ui.RGB(240, 243, 249),
			muted:      ui.RGB(154, 163, 178),
			line:       ui.RGBA(255, 255, 255, 0.11),
			accent:     ui.RGB(59, 130, 246),
			accentSoft: ui.RGBA(59, 130, 246, 0.22),
			ok:         ui.RGB(52, 211, 153),
			danger:     ui.RGB(248, 113, 113),
			dangerSoft: ui.RGBA(248, 113, 113, 0.16),
			track:      ui.RGBA(255, 255, 255, 0.12),
			fresh:      ui.RGB(139, 180, 255),
			cache:      ui.RGB(197, 163, 255),
			output:     ui.RGB(110, 231, 183),
			pillBg:     ui.RGBA(255, 255, 255, 0.14),
			pillText:   ui.RGB(240, 243, 249),
			warnBg:     ui.RGBA(251, 191, 36, 0.18),
			warnText:   ui.RGB(251, 191, 36),
			highBg:     ui.RGBA(248, 113, 113, 0.20),
		}
	}
	return palette{
		bg:         ui.RGB(244, 246, 251),
		card:       ui.RGBA(255, 255, 255, 0.96),
		cardSub:    ui.RGB(248, 250, 253),
		soft:       ui.RGBA(15, 23, 42, 0.06),
		fg:         ui.RGB(18, 24, 38),
		muted:      ui.RGB(100, 116, 139),
		line:       ui.RGBA(15, 23, 42, 0.10),
		accent:     ui.RGB(37, 99, 235),
		accentSoft: ui.RGBA(37, 99, 235, 0.12),
		ok:         ui.RGB(16, 185, 129),
		danger:     ui.RGB(220, 38, 38),
		dangerSoft: ui.RGBA(220, 38, 38, 0.10),
		track:      ui.RGBA(15, 23, 42, 0.10),
		fresh:      ui.RGB(139, 180, 255),
		cache:      ui.RGB(197, 163, 255),
		output:     ui.RGB(110, 231, 183),
		pillBg:     ui.RGB(255, 255, 255),
		pillText:   ui.RGB(18, 24, 38),
		warnBg:     ui.RGBA(245, 158, 11, 0.16),
		warnText:   ui.RGB(180, 83, 9),
		highBg:     ui.RGBA(220, 38, 38, 0.14),
	}
}

var (
	sunSVG          = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/></svg>`))
	moonSVG         = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="currentColor"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>`))
	chevronRightSVG = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>`))
	chevronDownSVG  = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>`))
)

type uiState struct {
	mu                sync.Mutex
	svc               *appservice.Accounts
	win               *mygo.Window
	startPoll         func()
	stopPoll          func()
	lang              langCode
	theme             string
	tabIndex          int
	accounts          []appservice.AccountView
	projects          []projects.ProjectView
	expandedAccounts  map[string]bool
	expandedProjects  map[string]bool
	initAccountExpand bool
	initProjectExpand bool
	refreshing        bool
	refreshingEmails  map[string]bool
	currentQuotaBusy  bool
	switching         bool
	switchGen         uint64
	projectsLoaded    bool
	projectsLoading   bool
	adding            bool
	addEmail          string
	oauthCode         string
	oauth             appservice.OAuthStatusView
	oauthPolling      bool
	statusMsg         string
	statusErr         bool
	lastWindowHeight  int
}

func newUIState(svc *appservice.Accounts) *uiState {
	s := &uiState{
		svc:              svc,
		lang:             langZH,
		expandedAccounts: make(map[string]bool),
		expandedProjects: make(map[string]bool),
		refreshingEmails: make(map[string]bool),
	}
	s.loadPrefs()
	return s
}

func (s *uiState) view(c *ui.Context) {
	s.mu.Lock()
	themeOverride := s.theme
	tabIdx := s.tabIndex
	accountsCopy := append([]appservice.AccountView(nil), s.accounts...)
	projectsCopy := append([]projects.ProjectView(nil), s.projects...)
	statusMsg := s.statusMsg
	statusErr := s.statusErr
	s.mu.Unlock()

	if themeOverride == "dark" {
		c.SetTheme(ui.DarkTheme())
	} else if themeOverride == "light" {
		c.SetTheme(ui.LightTheme())
	}
	p := currentPalette(c)
	isDark := c.Theme().Background.R < 128

	root := ui.Column(c).Fill().Padding(12).Gap(10)
	if !c.Vibrancy() {
		root = root.Background(p.bg)
	}
	root.Children(func() {
		// Header
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Box(c).Width(24).Height(24).Radius(7).Background(p.accent).Center().Children(func() {
				ui.Text(c, "A").FontSize(12).Bold().TextColor(ui.RGB(255, 255, 255))
			})
			ui.Text(c, s.tr("app.title")).FontSize(13.5).Bold().TextColor(p.fg).Grow(1)

			// Sun / Moon theme toggle pill
			ui.Row(c).AlignItems(ui.Center).Gap(2).Padding(2).Radius(999).Background(p.soft).Children(func() {
				sunBtn := ui.ButtonBase(c).Width(24).Height(24).Radius(12).Center().Cursor(ui.CursorPointer)
				sunCol := p.muted
				if !isDark {
					sunBtn = sunBtn.Background(p.pillBg)
					sunCol = p.fg
				}
				if sunBtn.Children(func() {
					ui.Icon(c, sunSVG).Size(13, 13).TextColor(sunCol)
				}).Clicked() {
					s.setThemeMode("light")
				}

				moonBtn := ui.ButtonBase(c).Width(24).Height(24).Radius(12).Center().Cursor(ui.CursorPointer)
				moonCol := p.muted
				if isDark {
					moonBtn = moonBtn.Background(p.pillBg)
					moonCol = p.fg
				}
				if moonBtn.Children(func() {
					ui.Icon(c, moonSVG).Size(12, 12).TextColor(moonCol)
				}).Clicked() {
					s.setThemeMode("dark")
				}
			})

			if ui.ButtonBase(c).Padding(4, 8).Radius(7).Background(p.soft).Cursor(ui.CursorPointer).Children(func() {
				ui.Text(c, s.tr("lang.toggle")).FontSize(10.5).Bold().TextColor(p.fg)
			}).Clicked() {
				s.mu.Lock()
				if s.lang == langZH {
					s.lang = langEN
				} else {
					s.lang = langZH
				}
				s.mu.Unlock()
			}
		})

		// Tabs (50/50 segmented bar)
		ui.Row(c).FillWidth().Padding(3).Gap(3).Radius(10).Background(p.soft).Border(1, p.line).Children(func() {
			tabs := []string{s.tr("tab.accounts"), s.tr("tab.projects")}
			for i, label := range tabs {
				active := i == tabIdx
				btn := ui.ButtonBase(c).Grow(1).Padding(6, 0).Radius(8).Center().Cursor(ui.CursorPointer)
				if active {
					btn = btn.Background(p.pillBg)
				}
				if btn.Children(func() {
					txtCol := p.muted
					if active {
						txtCol = p.pillText
					}
					t := ui.Text(c, label).FontSize(11).TextColor(txtCol)
					if active {
						t.Bold()
					}
				}).Clicked() && !active {
					s.switchTab(i)
				}
			}
		})

		if tabIdx == 0 {
			ui.Column(c).FillWidth().Grow(1).Gap(8).Children(func() {
				s.renderAccountsTab(c, p, accountsCopy)
			})
		} else {
			ui.Column(c).FillWidth().Grow(1).Gap(8).Children(func() {
				s.renderProjectsTab(c, p, projectsCopy)
			})
		}

		// Footer
		ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Padding(2, 2, 0, 2).Children(func() {
			ui.Text(c, s.tr("footer.sharedCli")).FontSize(9.5).TextColor(p.muted).Ellipsis("…").SingleLine().Shrink(1)
			if statusMsg != "" {
				col := p.muted
				if statusErr {
					col = p.danger
				}
				ui.Text(c, statusMsg).FontSize(9.5).TextColor(col).Ellipsis("…").SingleLine()
			}
		})
	})
}

func (s *uiState) switchTab(next int) {
	s.mu.Lock()
	s.tabIndex = next
	startPoll := s.startPoll
	stopPoll := s.stopPoll
	s.mu.Unlock()

	if next == 1 {
		s.loadProjectsSyncIfNeeded()
		if startPoll != nil {
			startPoll()
		}
	} else {
		if stopPoll != nil {
			stopPoll()
		}
	}
}

func (s *uiState) renderAccountsTab(c *ui.Context, p palette, items []appservice.AccountView) {
	summary := s.tr("summary.none")
	for _, a := range items {
		if a.Current {
			summary = s.tr("summary.current", a.Email)
			break
		}
	}
	if len(items) == 0 {
		s.mu.Lock()
		if s.refreshing {
			summary = s.tr("summary.refreshing")
		}
		s.mu.Unlock()
	}
	ui.Text(c, summary).FontSize(10.5).TextColor(p.muted).Ellipsis("…").SingleLine()

	if len(items) > 0 {
		ui.Scroll(c).FillWidth().Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				for _, acc := range items {
					s.renderAccountCard(c, p, acc)
				}
			})
		})
	}

	s.renderOnboardingCard(c, p)
}

func (s *uiState) renderAccountCard(c *ui.Context, p palette, acc appservice.AccountView) {
	s.mu.Lock()
	expanded := s.expandedAccounts[acc.Email]
	cardRefreshing := s.refreshingEmails[acc.Email]
	win := s.win
	s.mu.Unlock()

	borderCol := p.line
	borderW := float32(1)
	if acc.Current {
		borderCol = p.accent
		borderW = 1.5
	}

	ui.Column(c).Key(acc.Email).FillWidth().Padding(10, 11).Gap(8).Radius(12).Background(p.card).Border(borderW, borderCol).Children(func() {
		// Card Header
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			// Radio indicator
			radioCol := p.line
			if acc.Current {
				radioCol = p.accent
			}
			radioBtn := ui.ButtonBase(c).Width(16).Height(16).Radius(8).Border(1.5, radioCol).Center().Cursor(ui.CursorPointer).Children(func() {
				if acc.Current {
					ui.Box(c).Width(8).Height(8).Radius(4).Background(p.accent)
				}
			})
			if radioBtn.Clicked() && !acc.Current && (acc.HasSavedLogin || acc.Current) {
				s.switchAccountAsync(win, acc.Email)
			}

			toggleClicked := false
			titleRow := ui.Row(c).Grow(1).Shrink(1).AlignItems(ui.Center).Gap(6).Cursor(ui.CursorPointer).Children(func() {
				ui.Text(c, acc.Email).FontSize(11.5).Bold().TextColor(p.fg).Ellipsis("…").SingleLine().Grow(1).Shrink(1)

				if acc.Current {
					ui.Box(c).Padding(2, 6).Radius(6).Background(p.accentSoft).Children(func() {
						ui.Text(c, s.tr("account.current")).FontSize(9.5).Bold().TextColor(p.accent)
					})
				}
				tierLabel := ""
				if acc.Quota != nil {
					tierLabel = formatTierLabel(acc.Quota.TierID, acc.Quota.TierName)
				}
				if tierLabel != "" {
					ui.Box(c).Padding(2, 6).Radius(6).Background(p.soft).Children(func() {
						ui.Text(c, tierLabel).FontSize(9.5).TextColor(p.muted)
					})
				}
			})
			if titleRow.Clicked() {
				toggleClicked = true
			}

			cardRefreshBtn := ui.ButtonBase(c).Padding(2, 6).Radius(6).Background(p.soft).Cursor(ui.CursorPointer).Disabled(cardRefreshing).Children(func() {
				lbl := "↻"
				if cardRefreshing {
					lbl = "…"
				}
				ui.Text(c, lbl).FontSize(10.5).Bold().TextColor(p.muted)
			})
			if cardRefreshBtn.Clicked() {
				s.refreshSingleAccountAsync(win, acc.Email)
			}

			chevIcon := chevronRightSVG
			if expanded {
				chevIcon = chevronDownSVG
			}
			if ui.ButtonBase(c).Padding(4).Radius(5).Cursor(ui.CursorPointer).Children(func() {
				ui.Icon(c, chevIcon).Size(12, 12).TextColor(p.muted)
			}).Clicked() {
				toggleClicked = true
			}
			if toggleClicked {
				s.mu.Lock()
				s.expandedAccounts[acc.Email] = !expanded
				s.mu.Unlock()
			}
		})

		if expanded {
			ui.Column(c).FillWidth().Gap(7).Padding(8, 9).Radius(9).Background(p.cardSub).Border(1, p.line).Children(func() {
				if acc.Quota == nil || len(acc.Quota.Buckets) == 0 {
					ui.Text(c, s.tr("account.noQuota")).FontSize(10).TextColor(p.muted)
				} else {
					preferred := []string{"g5", "gw", "c5", "cw"}
					seen := make(map[string]bool, len(acc.Quota.Buckets))
					keys := make([]string, 0, len(acc.Quota.Buckets))
					for _, k := range preferred {
						if _, ok := acc.Quota.Buckets[k]; ok {
							keys = append(keys, k)
							seen[k] = true
						}
					}
					var extra []string
					for k := range acc.Quota.Buckets {
						if !seen[k] {
							extra = append(extra, k)
						}
					}
					sort.Strings(extra)
					keys = append(keys, extra...)

					for _, k := range keys {
						b := acc.Quota.Buckets[k]
						pct := b.Percent
						if pct < 0 {
							pct = 0
						}
						if pct > 100 {
							pct = 100
						}
						barColor := ui.Hex(quotaColorHex(pct))
						ui.Column(c).FillWidth().Gap(3).Children(func() {
							ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
								ui.Text(c, s.bucketLabel(k)).FontSize(10).TextColor(p.fg)
								rightText := formatPctInt(pct)
								if resetText := s.formatResetCountdown(pct, b.Reset); resetText != "" {
									rightText = rightText + " · " + resetText
								}
								ui.Text(c, rightText).FontSize(9.5).TextColor(p.muted)
							})
							ui.Box(c).FillWidth().Height(5).Radius(999).Background(p.track).Children(func() {
								if pct > 0 {
									ui.Box(c).WidthPercent(float32(pct)).Height(5).Radius(999).Background(barColor)
								}
							})
						})
					}
				}

				ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Padding(4, 0, 0, 0).Children(func() {
					rot := acc.Rotation
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Switch(c, &rot)
						ui.Text(c, s.tr("account.rotation")).FontSize(10).TextColor(p.muted)
					})
					if rot != acc.Rotation {
						s.setRotationAsync(win, acc.Email, rot)
					}

					if !acc.Current {
						if ui.ButtonBase(c).Padding(2, 7).Radius(5).Background(p.dangerSoft).Cursor(ui.CursorPointer).Children(func() {
							ui.Text(c, s.tr("account.remove")).FontSize(9.5).Bold().TextColor(p.danger)
						}).Clicked() {
							s.removeAccountAsync(win, acc.Email)
						}
					}
				})
			})
		}
	})
}

func (s *uiState) renderOnboardingCard(c *ui.Context, p palette) {
	s.mu.Lock()
	email := s.addEmail
	adding := s.adding
	oauth := s.oauth
	code := s.oauthCode
	win := s.win
	s.mu.Unlock()

	ui.Column(c).FillWidth().Padding(10, 11).Gap(8).Radius(12).Background(p.card).Border(1, p.line).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			inp := ui.TextInput(c, &email).Placeholder(s.tr("account.addPlaceholder")).Grow(1).Shrink(1)
			if email != s.addEmail {
				s.mu.Lock()
				s.addEmail = email
				s.mu.Unlock()
			}
			addBtn := ui.ButtonBase(c).Padding(5, 10).Radius(7).Background(p.accent).Cursor(ui.CursorPointer).Disabled(adding || email == "").Children(func() {
				ui.Text(c, s.tr("account.add")).FontSize(10.5).Bold().TextColor(ui.RGB(255, 255, 255))
			})
			if addBtn.Clicked() || inp.Submitted() {
				s.addAccountAsync(win, email)
			}
		})

		oauthBusy := oauth.State == accounts.OAuthStateStarting || oauth.State == accounts.OAuthStateWaiting || oauth.State == accounts.OAuthStateAuthorize
		if ui.ButtonBase(c).FillWidth().Padding(6, 10).Radius(7).Background(p.soft).Cursor(ui.CursorPointer).Disabled(oauthBusy).Center().Children(func() {
			ui.Text(c, s.tr("account.oauthStart")).FontSize(10.5).Bold().TextColor(p.fg)
		}).Clicked() {
			s.startOAuthAsync(win)
		}

		if oauth.State != "" && oauth.State != "idle" {
			ui.Column(c).FillWidth().Gap(6).Padding(8, 9).Radius(9).Background(p.cardSub).Border(1, p.line).Children(func() {
				msg := oauth.Message
				if key := "oauth." + oauth.State; s.tr(key) != key {
					msg = s.tr(key)
				}
				if oauth.State == "error" && oauth.Message != "" {
					msg = oauth.Message
				}
				ui.Text(c, msg).FontSize(10).TextColor(p.fg)

				if oauth.State == accounts.OAuthStateAuthorize && oauth.URL != "" {
					if ui.ButtonBase(c).Padding(4, 8).Radius(6).Background(p.accentSoft).Cursor(ui.CursorPointer).Children(func() {
						ui.Text(c, s.tr("oauth.openLink")).FontSize(10).Bold().TextColor(p.accent)
					}).Clicked() {
						go func(u string) { _ = mygo.Shell.OpenExternal(u) }(oauth.URL)
					}

					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
						codeInp := ui.TextInput(c, &code).Placeholder(s.tr("oauth.codePlaceholder")).Grow(1).Shrink(1)
						if code != s.oauthCode {
							s.mu.Lock()
							s.oauthCode = code
							s.mu.Unlock()
						}
						subBtn := ui.ButtonBase(c).Padding(4, 9).Radius(6).Background(p.accent).Cursor(ui.CursorPointer).Disabled(code == "").Children(func() {
							ui.Text(c, s.tr("oauth.submit")).FontSize(10).Bold().TextColor(ui.RGB(255, 255, 255))
						})
						if subBtn.Clicked() || codeInp.Submitted() {
							s.submitOAuthCodeAsync(win, oauth.AttemptID, code)
						}
					})
				}

				if oauthBusy {
					if ui.ButtonBase(c).Padding(3, 8).Radius(5).Background(p.dangerSoft).Cursor(ui.CursorPointer).Children(func() {
						ui.Text(c, s.tr("oauth.cancel")).FontSize(9.5).Bold().TextColor(p.danger)
					}).Clicked() {
						s.cancelOAuthAsync(win, oauth.AttemptID)
					}
				}
			})
		}
	})
}

func (s *uiState) renderProjectsTab(c *ui.Context, p palette, items []projects.ProjectView) {
	s.mu.Lock()
	loading := s.projectsLoading && !s.projectsLoaded
	s.mu.Unlock()

	if loading {
		ui.Column(c).FillWidth().Padding(32, 16).Center().Children(func() {
			ui.Spinner(c)
		})
		return
	}

	if len(items) == 0 {
		ui.Column(c).FillWidth().Grow(1).Center().Children(func() {
			ui.Column(c).FillWidth().Padding(24, 16).Gap(6).Radius(12).Background(p.card).Border(1, p.line).Center().Children(func() {
				ui.Text(c, s.tr("projects.emptyTitle")).FontSize(12).Bold().TextColor(p.fg)
				ui.Text(c, s.tr("projects.emptyDesc")).FontSize(10).TextColor(p.muted)
			})
		})
		return
	}

	ui.Scroll(c).FillWidth().Grow(1).Children(func() {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			for _, proj := range items {
				s.renderProjectCard(c, p, proj)
			}
		})
	})
}

func (s *uiState) renderProjectCard(c *ui.Context, p palette, proj projects.ProjectView) {
	s.mu.Lock()
	expanded := s.expandedProjects[proj.Path]
	win := s.win
	s.mu.Unlock()

	borderCol := p.line
	borderW := float32(1)
	if proj.Active {
		borderCol = p.accent
		borderW = 1.5
	}

	ui.Column(c).Key(proj.Path).FillWidth().Padding(10, 11).Gap(6).Radius(12).Background(p.card).Border(borderW, borderCol).Children(func() {
		header := ui.Column(c).FillWidth().Gap(3).Cursor(ui.CursorPointer).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(7).Children(func() {
				dotCol := p.muted
				if proj.Active {
					dotCol = p.ok
				}
				ui.Box(c).Width(7).Height(7).Radius(4).Background(dotCol)
				ui.Text(c, proj.Name).FontSize(12).Bold().TextColor(p.fg).Ellipsis("…").SingleLine().Grow(1).Shrink(1)
				modelText := proj.Model
				if modelText == "" {
					if proj.Active {
						modelText = s.tr("projects.running")
					} else {
						modelText = s.tr("projects.idle")
					}
				}
				ui.Box(c).Padding(2, 6).Radius(999).Background(p.soft).Border(1, p.line).Children(func() {
					ui.Text(c, modelText).FontSize(9.5).TextColor(p.muted).Ellipsis("…").SingleLine()
				})
				chevIcon := chevronRightSVG
				if expanded {
					chevIcon = chevronDownSVG
				}
				ui.Icon(c, chevIcon).Size(12, 12).TextColor(p.muted)
			})

			pathBtn := ui.ButtonBase(c).FillWidth().Justify(ui.Start).Cursor(ui.CursorPointer).Children(func() {
				ui.Text(c, proj.Path).FontSize(9.5).TextColor(p.muted).Ellipsis("…").SingleLine()
			})
			if pathBtn.Clicked() {
				go func(path, name string) {
					mygo.Clipboard.WriteText(path)
					s.mu.Lock()
					s.statusMsg = s.tr("projects.copied", name)
					s.statusErr = false
					s.mu.Unlock()
					s.triggerUpdate(win)
				}(proj.Path, proj.Name)
			}
		})
		if header.Clicked() {
			s.mu.Lock()
			s.expandedProjects[proj.Path] = !expanded
			s.mu.Unlock()
		}

		if expanded {
			limit := proj.ContextLimit
			if limit <= 0 {
				limit = 1_000_000
			}
			fresh := max64(0, proj.LatestInput)
			cacheRead := max64(0, proj.LatestCacheRead)
			output := max64(0, proj.LatestOutput)
			used := fresh + cacheRead
			free := max64(0, limit-used)
			pctVal := math.Min(100, (float64(used)/float64(limit))*100)

			ui.Column(c).FillWidth().Gap(8).Padding(9, 10).Radius(10).Background(p.cardSub).Border(1, p.line).Children(func() {
				// Context Hero
				ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
					ui.Row(c).AlignItems(ui.Baseline).Gap(5).Children(func() {
						ui.Text(c, formatTokens(used)).FontSize(17).Bold().TextColor(p.fg)
						ui.Text(c, "/ "+formatTokens(limit)).FontSize(10.5).TextColor(p.muted)
						ui.Text(c, s.tr("projects.contextWindow")).FontSize(10).TextColor(p.muted)
					})
					badgeBg, badgeFg := p.warnBg, p.warnText
					if pctVal < 40 {
						badgeBg, badgeFg = p.accentSoft, p.accent
					} else if pctVal >= 80 {
						badgeBg, badgeFg = p.highBg, p.danger
					}
					ui.Box(c).Padding(2, 7).Radius(6).Background(badgeBg).Children(func() {
						ui.Text(c, formatPct(used, limit)).FontSize(10).Bold().TextColor(badgeFg)
					})
				})

				// 20x4 Dot Matrix (80 cells)
				const totalCells = 80
				freshCells := minInt(totalCells, calcMatrixCells(fresh, limit, totalCells))
				cacheCells := minInt(totalCells-freshCells, calcMatrixCells(cacheRead, limit, totalCells))
				outCells := minInt(totalCells-freshCells-cacheCells, calcMatrixCells(output, limit, totalCells))

				ui.Column(c).FillWidth().Gap(3).Children(func() {
					for row := 0; row < 4; row++ {
						ui.Row(c).FillWidth().Gap(3).Children(func() {
							for col := 0; col < 20; col++ {
								idx := row*20 + col
								cellCol := p.track
								if idx < freshCells {
									cellCol = p.fresh
								} else if idx < freshCells+cacheCells {
									cellCol = p.cache
								} else if idx < freshCells+cacheCells+outCells {
									cellCol = p.output
								}
								ui.Box(c).Height(11).Grow(1).Radius(2.5).Background(cellCol)
							}
						})
					}
				})

				// Legend
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					renderLegendRow(c, p, p.fresh, s.tr("projects.legend.input"), formatTokens(fresh)+" ("+formatPct(fresh, limit)+")")
					renderLegendRow(c, p, p.cache, s.tr("projects.legend.cache"), formatTokens(cacheRead)+" ("+formatPct(cacheRead, limit)+")")
					renderLegendRow(c, p, p.output, s.tr("projects.legend.output"), formatTokens(output)+" ("+formatPct(output, limit)+")")
					renderLegendRow(c, p, p.track, s.tr("projects.legend.free"), formatTokens(free)+" ("+formatPct(free, limit)+")")
				})

				// Footer
				totalTokens := proj.TotalInput + proj.TotalCacheRead + proj.TotalOutput
				statusLabel := s.tr("projects.idle")
				if proj.Active {
					statusLabel = s.tr("projects.running")
				}
				ui.Divider(c)
				ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Padding(2, 0, 0, 0).Children(func() {
					ui.Text(c, s.tr("projects.footer", proj.StepCount, formatTokens(totalTokens))).FontSize(9.5).TextColor(p.muted)
					ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
						ui.Text(c, statusLabel).FontSize(9.5).TextColor(p.muted)
						if ui.ButtonBase(c).Padding(2, 6).Radius(5).Background(p.dangerSoft).Cursor(ui.CursorPointer).Children(func() {
							ui.Text(c, s.tr("projects.remove")).FontSize(9.5).Bold().TextColor(p.danger)
						}).Clicked() {
							s.removeProjectAsync(win, proj.Path)
						}
					})
				})
			})
		}
	})
}

func renderLegendRow(c *ui.Context, p palette, swatch ui.Color, label, val string) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Box(c).Width(7).Height(7).Radius(2).Background(swatch)
			ui.Text(c, label).FontSize(9.8).TextColor(p.muted)
		})
		ui.Text(c, val).FontSize(9.8).Bold().TextColor(p.fg)
	})
}

func (s *uiState) switchAccountAsync(win *mygo.Window, email string) {
	s.mu.Lock()
	s.switching = true
	s.switchGen++
	myGen := s.switchGen
	for i := range s.accounts {
		s.accounts[i].Current = s.accounts[i].Email == email
	}
	s.expandedAccounts[email] = true
	s.statusMsg = s.tr("status.switching")
	s.statusErr = false
	s.mu.Unlock()
	s.triggerUpdate(win)

	go func() {
		err := s.svc.SwitchAccount(email)
		s.mu.Lock()
		if s.switchGen == myGen {
			s.switching = false
		}
		if err != nil {
			s.statusMsg = err.Error()
			s.statusErr = true
		} else {
			s.statusMsg = s.tr("status.switched", email)
			s.statusErr = false
		}
		s.mu.Unlock()
		s.loadAccountsAsync(win, false)
	}()
}

func (s *uiState) refreshSingleAccountAsync(win *mygo.Window, email string) {
	if s.svc == nil {
		return
	}
	s.mu.Lock()
	if s.refreshingEmails[email] {
		s.mu.Unlock()
		return
	}
	s.refreshingEmails[email] = true
	s.statusMsg = s.tr("status.refreshing")
	s.statusErr = false
	s.mu.Unlock()
	s.triggerUpdate(win)

	go func() {
		items, err := s.svc.RefreshAccountQuota(email)
		s.mu.Lock()
		delete(s.refreshingEmails, email)
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

func (s *uiState) setRotationAsync(win *mygo.Window, email string, enabled bool) {
	go func() {
		err := s.svc.SetRotation(email, enabled)
		s.mu.Lock()
		if err != nil {
			s.statusMsg = err.Error()
			s.statusErr = true
		} else if enabled {
			s.statusMsg = s.tr("status.rotationOn", email)
			s.statusErr = false
		} else {
			s.statusMsg = s.tr("status.rotationOff", email)
			s.statusErr = false
		}
		s.mu.Unlock()
		s.loadAccountsAsync(win, false)
	}()
}

func (s *uiState) removeAccountAsync(win *mygo.Window, email string) {
	go func() {
		err := s.svc.RemoveAccount(email)
		s.mu.Lock()
		if err != nil {
			s.statusMsg = err.Error()
			s.statusErr = true
		} else {
			delete(s.expandedAccounts, email)
			s.statusMsg = s.tr("status.removed", email)
			s.statusErr = false
		}
		s.mu.Unlock()
		s.loadAccountsAsync(win, false)
	}()
}

func (s *uiState) addAccountAsync(win *mygo.Window, email string) {
	s.mu.Lock()
	if s.adding || email == "" {
		s.mu.Unlock()
		return
	}
	s.adding = true
	s.statusMsg = s.tr("status.adding")
	s.statusErr = false
	s.mu.Unlock()
	s.triggerUpdate(win)

	go func() {
		added, err := s.svc.AddExistingAccount(email)
		s.mu.Lock()
		s.adding = false
		if err != nil {
			s.statusMsg = err.Error()
			s.statusErr = true
		} else if added {
			s.addEmail = ""
			s.expandedAccounts[email] = true
			s.statusMsg = s.tr("status.added")
			s.statusErr = false
		} else {
			s.addEmail = ""
			s.statusMsg = s.tr("status.exists")
			s.statusErr = false
		}
		s.mu.Unlock()
		s.loadAccountsAsync(win, false)
		if err == nil && added {
			s.refreshQuotasAsync(win)
		} else {
			s.triggerUpdate(win)
		}
	}()
}

func (s *uiState) startOAuthAsync(win *mygo.Window) {
	go func() {
		view, err := s.svc.StartOAuth()
		s.mu.Lock()
		if err != nil {
			s.statusMsg = err.Error()
			s.statusErr = true
			s.oauth = s.svc.OAuthStatus()
		} else {
			s.oauth = view
		}
		shouldPoll := !s.oauthPolling
		if shouldPoll {
			s.oauthPolling = true
		}
		s.mu.Unlock()
		s.triggerUpdate(win)
		if shouldPoll {
			s.pollOAuthLoop(win)
		}
	}()
}

func (s *uiState) pollOAuthLoop(win *mygo.Window) {
	ticker := time.NewTicker(650 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		view := s.svc.OAuthStatus()
		s.mu.Lock()
		s.oauth = view
		done := view.State == "" || view.State == "idle" || view.State == "added" || view.State == "error" || view.State == "cancelled"
		if done {
			s.oauthPolling = false
			if view.State == "added" && view.Email != "" {
				s.expandedAccounts[view.Email] = true
			}
		}
		s.mu.Unlock()
		s.triggerUpdate(win)
		if view.State == "added" {
			s.loadAccountsAsync(win, false)
			s.refreshQuotasAsync(win)
		}
		if done {
			return
		}
	}
}

func (s *uiState) submitOAuthCodeAsync(win *mygo.Window, attemptID, code string) {
	go func() {
		err := s.svc.SubmitOAuthCode(attemptID, code)
		s.mu.Lock()
		if err != nil {
			s.statusMsg = err.Error()
			s.statusErr = true
		} else {
			s.oauthCode = ""
			s.statusMsg = s.tr("oauth.submitted")
			s.statusErr = false
		}
		s.mu.Unlock()
		s.triggerUpdate(win)
	}()
}

func (s *uiState) cancelOAuthAsync(win *mygo.Window, attemptID string) {
	go func() {
		_ = s.svc.CancelOAuth(attemptID)
		s.mu.Lock()
		s.oauth = s.svc.OAuthStatus()
		s.mu.Unlock()
		s.triggerUpdate(win)
	}()
}

func (s *uiState) removeProjectAsync(win *mygo.Window, path string) {
	go func() {
		err := s.svc.RemoveProject(path)
		s.mu.Lock()
		if err != nil {
			s.statusMsg = err.Error()
			s.statusErr = true
		} else {
			delete(s.expandedProjects, path)
			filtered := make([]projects.ProjectView, 0, len(s.projects))
			for _, p := range s.projects {
				if p.Path != path {
					filtered = append(filtered, p)
				}
			}
			s.projects = filtered
		}
		s.mu.Unlock()
		s.triggerUpdate(win)
	}()
}

func uiPrefsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "agy-tools", "ui_prefs.json")
}

func (s *uiState) themeMode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.theme
}

func (s *uiState) setThemeMode(mode string) {
	s.mu.Lock()
	s.theme = mode
	win := s.win
	s.mu.Unlock()

	if win != nil {
		if mode == "dark" {
			mygo.Theme.SetSource(mygo.ThemeDark)
		} else if mode == "light" {
			mygo.Theme.SetSource(mygo.ThemeLight)
		}
		s.triggerUpdate(win)
	}
	go s.savePrefs()
}

func (s *uiState) loadPrefs() {
	path := uiPrefsPath()
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var payload struct {
		Theme string `json:"theme,omitempty"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return
	}
	s.mu.Lock()
	s.theme = payload.Theme
	s.mu.Unlock()
}

func (s *uiState) savePrefs() {
	path := uiPrefsPath()
	if path == "" {
		return
	}
	s.mu.Lock()
	payload := struct {
		Theme string `json:"theme,omitempty"`
	}{
		Theme: s.theme,
	}
	s.mu.Unlock()
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, data, 0o600)
}

func formatTierLabel(tierID, tierName string) string {
	id := strings.ToLower(strings.TrimSpace(tierID))
	name := strings.TrimSpace(tierName)
	lowerName := strings.ToLower(name)
	if id == "free-tier" || strings.Contains(lowerName, "starter") || strings.Contains(lowerName, "free") {
		return "Free"
	}
	if name != "" {
		return name
	}
	return strings.TrimSpace(tierID)
}

func formatPctInt(pct int) string {
	return formatTokens(int64(pct)) + "%"
}

func calcMatrixCells(val, limit int64, total int) int {
	if val <= 0 || limit <= 0 {
		return 0
	}
	n := int(math.Round((float64(val) / float64(limit)) * float64(total)))
	if n < 1 {
		return 1
	}
	return n
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
