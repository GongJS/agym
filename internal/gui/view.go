package gui

import (
	"encoding/json"
	"fmt"
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
	"github.com/gongjs/agym/internal/updater"
	"github.com/gongjs/agym/internal/usage"
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
	currentAgyModel   string
	projects          []projects.ProjectView
	expandedAccounts  map[string]bool
	expandedProjects  map[string]bool
	initAccountExpand bool
	initProjectExpand bool
	refreshing        bool
	refreshingEmails  map[string]bool
	rotationBusy      map[string]bool
	loadAccountsBusy  bool
	loadAccountsAgain bool
	switching         bool
	switchGen         uint64
	projectsLoaded     bool
	projectsLoading    bool
	adding             bool
	showAddAccount         bool
	sidebarAccountMenuOpen bool
	confirmRemoveEmail     string
	confirmRemoveProject   string
	addEmail           string
	oauthCode         string
	oauth             appservice.OAuthStatusView
	oauthPolling      bool
	statusMsg         string
	statusErr         bool
	shortcutEnabled   bool
	shortcutAccel     string
	shortcutInput     string
	shortcutRecording bool
	registeredAccel   string
	updateChecking    bool
	updateApplying    bool
	updateAvailable   bool
	latestVersion     string
	latestURL         string
	updateNotice      string
}

func newUIState(svc *appservice.Accounts) *uiState {
	s := &uiState{
		svc:              svc,
		lang:             langZH,
		expandedAccounts: make(map[string]bool),
		expandedProjects: make(map[string]bool),
		refreshingEmails: make(map[string]bool),
		rotationBusy:     make(map[string]bool),
		shortcutEnabled:  true,
		shortcutAccel:    "CommandOrControl+Shift+A",
		shortcutInput:    "CommandOrControl+Shift+A",
	}
	s.loadPrefs()
	return s
}

func (s *uiState) applyAccountsLocked(gen uint64, items []appservice.AccountView) bool {
	if s.switching || s.switchGen != gen {
		return false
	}
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
	return true
}

func (s *uiState) view(c *ui.Context) {
	s.mu.Lock()
	themeOverride := s.theme
	tabIdx := s.tabIndex
	accountsCopy := append([]appservice.AccountView(nil), s.accounts...)
	projectsCopy := append([]projects.ProjectView(nil), s.projects...)
	activeModel := s.currentAgyModel
	statusMsg := s.statusMsg
	statusErr := s.statusErr
	menuOpen := s.sidebarAccountMenuOpen
	updateAvail := s.updateAvailable
	win := s.win
	s.mu.Unlock()

	if themeOverride == "dark" {
		c.SetTheme(ui.DarkTheme())
	} else if themeOverride == "light" {
		c.SetTheme(ui.LightTheme())
	}
	p := currentPalette(c)

	activeEmail := ""
	for _, a := range accountsCopy {
		if a.Current {
			activeEmail = a.Email
			break
		}
	}
	if activeEmail == "" && len(accountsCopy) > 0 {
		activeEmail = accountsCopy[0].Email
	}

	bar := c.TitleBar()
	sidebarTopPad := float32(14)
	if bar.Bottom > 0 {
		sidebarTopPad = bar.Bottom + 10
	}
	headerH := float32(42)
	if bar.Height > headerH {
		headerH = bar.Height
	}

	ui.Row(c).Fill().Children(func() {
		// Left Sidebar
		ui.Column(c).Width(164).FillHeight().Background(p.cardSub).Padding(sidebarTopPad, 10, 10, 10).Gap(10).Children(func() {
			// Brand header (draggable)
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(7).Padding(2, 4).DragWindow().Children(func() {
				ui.Box(c).Width(22).Height(22).Radius(6).Background(p.accent).Center().Children(func() {
					ui.Text(c, "A").FontSize(11.5).Bold().TextColor(p.onAccent)
				})
				ui.Text(c, s.tr("app.title")).FontSize(13).Bold().TextColor(p.fg).Ellipsis("…").SingleLine().Grow(1).Shrink(1)
			})

			// Navigation
			type navItem struct {
				idx   int
				label string
				icon  *ui.SVG
			}
			navs := []navItem{
				{0, s.tr("tab.accounts"), accountsSVG},
				{1, s.tr("tab.projects"), projectsSVG},
				{2, s.tr("tab.settings"), settingsSVG},
			}
			ui.Column(c).FillWidth().Gap(3).Children(func() {
				for _, item := range navs {
					active := item.idx == tabIdx
					btn := ui.ButtonBase(c).FillWidth().Padding(8, 10).Radius(8).Cursor(ui.CursorPointer)
					if active {
						btn = btn.Background(p.accentSoft)
					}
					if btn.Children(func() {
						col := p.muted
						if active {
							col = p.accent
						}
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
							ui.Icon(c, item.icon).Size(14, 14).TextColor(col)
							txt := ui.Text(c, item.label).FontSize(11.5).TextColor(col).Grow(1).Shrink(1)
							if active {
								txt.Bold()
							}
							if item.idx == 2 && updateAvail {
								ui.Box(c).Width(6).Height(6).Radius(3).Background(p.danger)
							}
						})
					}).Clicked() && !active {
						s.switchTab(item.idx)
					}
				}
			})

			ui.Box(c).Grow(1)

			// Bottom-left active account card (hidden when no accounts are configured)
			if len(accountsCopy) > 0 {
				hasMultiple := len(accountsCopy) > 1
				canJumpTab := tabIdx != 0
				cardInteractive := hasMultiple || canJumpTab
				cardCursor := ui.CursorDefault
				if cardInteractive {
					cardCursor = ui.CursorPointer
				}
				cardBorder := p.line
				if menuOpen {
					cardBorder = p.accent
				}

				initial := "?"
				if runes := []rune(activeEmail); len(runes) > 0 {
					initial = strings.ToUpper(string(runes[:1]))
				}
				modelLabel := activeModel
				if modelLabel == "" {
					modelLabel = s.tr("sidebar.noModel")
				}

				accountCard := ui.ButtonBase(c).FillWidth().Padding(8, 9).Radius(10).Background(p.card).Border(1, cardBorder).Cursor(cardCursor).Disabled(!cardInteractive).Children(func() {
					ui.Column(c).FillWidth().Gap(5).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
							ui.Box(c).Width(20).Height(20).Radius(10).Background(p.accentSoft).Center().Children(func() {
								ui.Text(c, initial).FontSize(10).Bold().TextColor(p.accent)
							})
							if hasMultiple {
								ui.Icon(c, chevronsUpDownSVG).Size(12, 12).TextColor(p.muted)
							} else if tabIdx != 0 {
								ui.Icon(c, chevronRightSVG).Size(11, 11).TextColor(p.muted)
							}
						})
						ui.Text(c, activeEmail).FontSize(10.5).Bold().TextColor(p.fg).Ellipsis("…").SingleLine()
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(5).Children(func() {
							ui.Box(c).Width(5).Height(5).Radius(3).Background(p.ok)
							ui.Text(c, modelLabel).FontSize(9.5).TextColor(p.muted).Ellipsis("…").SingleLine().Grow(1).Shrink(1)
						})
					})
				})
				if accountCard.Clicked() {
					if hasMultiple {
						s.mu.Lock()
						s.sidebarAccountMenuOpen = !menuOpen
						s.mu.Unlock()
						menuOpen = !menuOpen
					} else if tabIdx != 0 {
						s.switchTab(0)
					}
				}

				if hasMultiple {
					popOpen := menuOpen
					ui.PopoverBase(c, accountCard, &popOpen, func(panel ui.Element) {
						panel.Attach(ui.AnchorTopLeft, ui.AnchorBottomLeft).Width(212).Padding(6).Radius(10).Background(p.card).Border(1, p.line).Children(func() {
							ui.Column(c).FillWidth().Gap(3).Children(func() {
								for _, a := range accountsCopy {
									canSwitch := !a.Current && a.HasSavedLogin
									rowCursor := ui.CursorDefault
									if canSwitch {
										rowCursor = ui.CursorPointer
									}
									rowBg := ui.Color{}
									if a.Current {
										rowBg = p.accentSoft
									}
									itemBtn := ui.ButtonBase(c).FillWidth().Padding(6, 8).Radius(7).Background(rowBg).Cursor(rowCursor).Disabled(!canSwitch && !a.Current).Children(func() {
										ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
											dot := p.muted
											if a.Current {
												dot = p.accent
											} else if a.HasSavedLogin {
												dot = p.ok
											}
											ui.Box(c).Width(6).Height(6).Radius(3).Background(dot)
											nameColor := p.fg
											if a.Current {
												nameColor = p.accent
											} else if !a.HasSavedLogin {
												nameColor = p.muted
											}
											txt := ui.Text(c, a.Email).FontSize(10.5).TextColor(nameColor).Ellipsis("…").SingleLine().Grow(1).Shrink(1)
											if a.Current {
												txt.Bold()
											}
											if a.Current {
												ui.Text(c, s.tr("account.current")).FontSize(9).Bold().TextColor(p.accent)
											} else if !a.HasSavedLogin {
												ui.Text(c, s.tr("account.needLogin")).FontSize(9).TextColor(p.warnText)
											}
										})
									})
									if itemBtn.Clicked() && canSwitch {
										popOpen = false
										s.mu.Lock()
										s.sidebarAccountMenuOpen = false
										s.mu.Unlock()
										s.switchAccountAsync(win, a.Email)
									}
								}
								if tabIdx != 0 {
									ui.Divider(c)
									if ui.ButtonBase(c).FillWidth().Padding(6, 8).Radius(7).Cursor(ui.CursorPointer).Children(func() {
										ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
											ui.Text(c, s.tr("sidebar.manageAccounts")).FontSize(10.5).TextColor(p.muted)
											ui.Icon(c, chevronRightSVG).Size(11, 11).TextColor(p.muted)
										})
									}).Clicked() {
										popOpen = false
										s.mu.Lock()
										s.sidebarAccountMenuOpen = false
										s.mu.Unlock()
										s.switchTab(0)
									}
								}
							})
						})
					})
					if popOpen != menuOpen {
						s.mu.Lock()
						s.sidebarAccountMenuOpen = popOpen
						s.mu.Unlock()
					}
				}
			}
		})

		// Vertical divider
		ui.Box(c).Width(1).FillHeight().Background(p.line)

		// Right Content Area
		content := ui.Column(c).Grow(1).Shrink(1).FillHeight()
		if !c.Vibrancy() {
			content = content.Background(p.bg)
		}
		content.Children(func() {
			// Section header (draggable, clears Windows/Linux top-right buttons if present)
			sectionTitle := s.tr("tab.accounts")
			if tabIdx == 1 {
				sectionTitle = s.tr("tab.projects")
			} else if tabIdx == 2 {
				sectionTitle = s.tr("tab.settings")
			}
			ui.Row(c).FillWidth().Height(headerH).Padding(0, maxFloat(16, bar.Right+8), 0, 16).AlignItems(ui.Center).Justify(ui.SpaceBetween).DragWindow().Children(func() {
				ui.Text(c, sectionTitle).FontSize(14).Bold().TextColor(p.fg)
				if statusMsg != "" && !statusErr {
					ui.Text(c, statusMsg).FontSize(10).TextColor(p.muted).Ellipsis("…").SingleLine()
				}
			})

			ui.Column(c).FillWidth().Grow(1).Padding(0, 16, 8, 16).Gap(8).Children(func() {
				switch tabIdx {
				case 0:
					s.renderAccountsTab(c, p, accountsCopy)
				case 1:
					s.renderProjectsTab(c, p, projectsCopy)
				default:
					s.renderSettingsTab(c, p)
				}
			})

			// Footer
			ui.Row(c).FillWidth().AlignItems(ui.Center).Padding(4, 16, 10, 16).Children(func() {
				ui.Text(c, s.tr("footer.sharedCli")).FontSize(9.5).TextColor(p.muted).Ellipsis("…").SingleLine()
			})
		})
	})
}

func (s *uiState) switchTab(next int) {
	s.mu.Lock()
	s.tabIndex = next
	s.shortcutRecording = false
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
	s.mu.Lock()
	refreshing := s.refreshing
	showAdd := s.showAddAccount
	oauthState := s.oauth.State
	statusMsg := s.statusMsg
	statusErr := s.statusErr
	win := s.win
	s.mu.Unlock()

	rotationCount := 0
	for _, a := range items {
		if a.Rotation {
			rotationCount++
		}
	}
	summary := s.tr("summary.none")
	if len(items) > 0 {
		summary = s.tr("summary.stats", len(items), rotationCount)
	} else if refreshing {
		summary = s.tr("summary.refreshing")
	}

	rightPad := float32(0)
	if len(items) > 0 {
		rightPad = 16
	}

	ui.Row(c).FillWidth().Padding(0, rightPad, 0, 0).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Children(func() {
		ui.Text(c, summary).FontSize(10.5).TextColor(p.muted).Ellipsis("…").SingleLine().Grow(1).Shrink(1)
		if len(items) > 0 {
			allBtn := ui.ButtonBase(c).Padding(4, 9).Radius(7).Background(p.soft).Cursor(ui.CursorPointer).Disabled(refreshing).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
					ui.Icon(c, refreshSVG).Size(10.5, 10.5).TextColor(p.fg)
					label := s.tr("summary.refreshAll")
					if refreshing {
						label = "…"
					}
					ui.Text(c, label).FontSize(9.8).Bold().TextColor(p.fg)
				})
			})
			if allBtn.Clicked() {
				s.refreshQuotasAsync(win)
			}
		}
	})

	if statusMsg != "" && statusErr {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(7, 10).Radius(9).Background(p.dangerSoft).Border(1, p.danger).Children(func() {
			ui.Text(c, statusMsg).FontSize(10).TextColor(p.danger).Grow(1).Shrink(1)
			if ui.ButtonBase(c).Padding(2).Radius(4).Cursor(ui.CursorPointer).Children(func() {
				ui.Icon(c, closeSVG).Size(11, 11).TextColor(p.danger)
			}).Clicked() {
				s.mu.Lock()
				s.statusMsg = ""
				s.statusErr = false
				s.mu.Unlock()
			}
		})
	}

	if len(items) > 0 {
		ui.Scroll(c).FillWidth().Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(0, 16, 0, 0).Gap(8).Children(func() {
				for _, acc := range items {
					s.renderAccountCard(c, p, acc)
				}
			})
		})
	}

	ui.Box(c).FillWidth().Padding(0, rightPad, 0, 0).Children(func() {
		oauthActive := oauthState != "" && oauthState != "idle"
		if len(items) == 0 || showAdd || oauthActive {
			s.renderOnboardingCard(c, p, len(items) > 0)
		} else {
			if ui.ButtonBase(c).FillWidth().Padding(8, 10).Radius(10).Background(p.card).Border(1, p.line).Center().Cursor(ui.CursorPointer).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
					ui.Icon(c, plusSVG).Size(12, 12).TextColor(p.accent)
					ui.Text(c, s.tr("account.addToggle")).FontSize(10.5).Bold().TextColor(p.fg)
				})
			}).Clicked() {
				s.mu.Lock()
				s.showAddAccount = true
				s.mu.Unlock()
				s.syncWindowHeight(win)
			}
		}
	})
}

func (s *uiState) renderAccountCard(c *ui.Context, p palette, acc appservice.AccountView) {
	s.mu.Lock()
	expanded := s.expandedAccounts[acc.Email]
	cardRefreshing := s.refreshingEmails[acc.Email]
	activeModel := s.currentAgyModel
	confirmingRemove := s.confirmRemoveEmail == acc.Email
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
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(7).Children(func() {
			toggleClicked := false
			tierLabel := ""
			if acc.Quota != nil {
				tierLabel = formatTierLabel(acc.Quota.TierID, acc.Quota.TierName)
			}
			renderBadges := func() {
				if acc.Current {
					ui.Box(c).Padding(2, 6).Radius(999).Background(p.accentSoft).Children(func() {
						ui.Text(c, s.tr("account.current")).FontSize(9.5).Bold().TextColor(p.accent)
					})
				} else if !acc.HasSavedLogin {
					ui.Box(c).Padding(2, 6).Radius(999).Background(p.warnBg).Children(func() {
						ui.Text(c, s.tr("account.needLogin")).FontSize(9.5).Bold().TextColor(p.warnText)
					})
				}
				if acc.Quota != nil && acc.Quota.Status == "stale" {
					ui.Box(c).Padding(2, 6).Radius(999).Background(p.warnBg).Children(func() {
						ui.Text(c, s.tr("account.staleBadge")).FontSize(9.5).Bold().TextColor(p.warnText)
					})
				}
				if tierLabel != "" {
					ui.Box(c).Padding(2, 6).Radius(999).Background(p.soft).Children(func() {
						ui.Text(c, tierLabel).FontSize(9.5).TextColor(p.muted)
					})
				}
				if !expanded && acc.Rotation {
					ui.Box(c).Padding(2, 6).Radius(999).Background(p.soft).Children(func() {
						ui.Text(c, s.tr("account.inPool")).FontSize(9.5).TextColor(p.muted)
					})
				}
			}

			if expanded {
				titleRow := ui.Row(c).Grow(1).Shrink(1).AlignItems(ui.Center).Gap(5).Cursor(ui.CursorPointer).Children(func() {
					ui.Text(c, acc.Email).FontSize(11.5).Bold().TextColor(p.fg).Ellipsis("…").SingleLine().Grow(1).Shrink(1)
					renderBadges()
				})
				if titleRow.Clicked() {
					toggleClicked = true
				}
			} else {
				titleCol := ui.Column(c).Grow(1).Shrink(1).Gap(4).Cursor(ui.CursorPointer).Children(func() {
					ui.Text(c, acc.Email).FontSize(11.5).Bold().TextColor(p.fg).Ellipsis("…").SingleLine()
					ui.Row(c).Wrap().AlignItems(ui.Center).Gap(4).Children(renderBadges)
				})
				if titleCol.Clicked() {
					toggleClicked = true
				}

				if acc.Quota != nil && len(acc.Quota.Buckets) > 0 {
					ui.Column(c).Gap(3).Children(func() {
						renderMiniQuotaRow(c, p, "G", acc.Quota.Buckets, "g5", "gw")
						renderMiniQuotaRow(c, p, "C", acc.Quota.Buckets, "c5", "cw")
					})
				}
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
				if s.confirmRemoveEmail == acc.Email {
					s.confirmRemoveEmail = ""
				}
				s.mu.Unlock()
			}
		})

		if expanded {
			ui.Divider(c)
			ui.Column(c).FillWidth().Padding(2, 0, 0, 0).Gap(9).Children(func() {
				if acc.Quota == nil || len(acc.Quota.Buckets) == 0 {
					if acc.Quota != nil && acc.Quota.Status != "" && acc.Quota.Status != "ok" {
						ui.Text(c, s.quotaFailureMessage(acc.Quota.Status, acc.Quota.Error)).FontSize(10).TextColor(p.warnText)
					} else {
						ui.Text(c, s.tr("account.noQuota")).FontSize(10).TextColor(p.muted)
					}
				} else {
					if acc.Quota.Status == "stale" {
						notice := s.tr("account.staleNoticeNoTime")
						if age := s.formatRelativeAge(acc.Quota.CheckedAt); age != "" {
							notice = s.tr("account.staleNotice", age)
						}
						ui.Box(c).FillWidth().Padding(5, 8).Radius(7).Background(p.warnBg).Children(func() {
							ui.Text(c, notice).FontSize(9.5).TextColor(p.warnText)
						})
					}
					type quotaGroupSpec struct {
						title   string
						members []string
						keys    []string
					}
					groups := []quotaGroupSpec{
						{
							title:   s.tr("quota.group.gemini"),
							members: acc.Quota.Members["gemini"],
							keys:    []string{"g5", "gw"},
						},
						{
							title:   s.tr("quota.group.claude"),
							members: acc.Quota.Members["claude"],
							keys:    []string{"c5", "cw"},
						},
					}
					seen := make(map[string]bool, len(acc.Quota.Buckets))
					renderedGroups := 0
					for _, grp := range groups {
						var activeKeys []string
						for _, k := range grp.keys {
							if _, ok := acc.Quota.Buckets[k]; ok {
								activeKeys = append(activeKeys, k)
								seen[k] = true
							}
						}
						if len(activeKeys) == 0 && len(grp.members) == 0 {
							continue
						}
						if renderedGroups > 0 {
							ui.Divider(c)
						}
						renderedGroups++
						ui.Column(c).FillWidth().Gap(5).Children(func() {
							ui.Text(c, grp.title).FontSize(10.5).Bold().TextColor(p.fg)
							if len(grp.members) > 0 {
								renderModelTags(c, p, grp.members, acc.Current, activeModel)
							}
							for _, k := range activeKeys {
								s.renderQuotaBar(c, p, k, acc.Quota.Buckets[k])
							}
						})
					}
					var extra []string
					for k := range acc.Quota.Buckets {
						if !seen[k] {
							extra = append(extra, k)
						}
					}
					sort.Strings(extra)
					for _, k := range extra {
						s.renderQuotaBar(c, p, k, acc.Quota.Buckets[k])
					}
				}

				ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Padding(4, 0, 2, 0).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
						ui.Icon(c, clockSVG).Size(11, 11).TextColor(p.muted)
						checkedText := s.tr("account.notChecked")
						if acc.Quota != nil && acc.Quota.CheckedAt > 0 {
							clockStr := time.Unix(acc.Quota.CheckedAt, 0).Local().Format("15:04")
							if age := s.formatRelativeAge(acc.Quota.CheckedAt); age != "" {
								checkedText = s.tr("account.checkedAt", clockStr, age)
							}
						}
						ui.Text(c, checkedText).FontSize(9.5).TextColor(p.muted)
					})
					footerRefreshBtn := ui.ButtonBase(c).Padding(3, 8).Radius(6).Background(p.soft).Cursor(ui.CursorPointer).Disabled(cardRefreshing).Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
							ui.Icon(c, refreshSVG).Size(10.5, 10.5).TextColor(p.fg)
							label := s.tr("account.refresh")
							if cardRefreshing {
								label = "…"
							}
							ui.Text(c, label).FontSize(9.5).Bold().TextColor(p.fg)
						})
					})
					if footerRefreshBtn.Clicked() {
						s.refreshSingleAccountAsync(win, acc.Email)
					}
				})

				ui.Divider(c)

				ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Padding(2, 0, 0, 0).Children(func() {
					rot := acc.Rotation
					ui.Row(c).AlignItems(ui.Center).Gap(6).Cursor(ui.CursorPointer).Children(func() {
						ui.Switch(c, &rot).Cursor(ui.CursorPointer)
						ui.Text(c, s.tr("account.rotation")).FontSize(10).TextColor(p.muted)
					})
					if rot != acc.Rotation {
						s.setRotationAsync(win, acc.Email, rot)
					}

					if !acc.Current {
						if confirmingRemove {
							ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
								if ui.ButtonBase(c).Padding(3, 7).Radius(5).Background(p.soft).Cursor(ui.CursorPointer).Children(func() {
									ui.Text(c, s.tr("account.cancelRemove")).FontSize(9.5).TextColor(p.muted)
								}).Clicked() {
									s.mu.Lock()
									s.confirmRemoveEmail = ""
									s.mu.Unlock()
								}
								if ui.ButtonBase(c).Padding(3, 8).Radius(5).Background(p.danger).Cursor(ui.CursorPointer).Children(func() {
									ui.Text(c, s.tr("account.confirmRemove")).FontSize(9.5).Bold().TextColor(p.onAccent)
								}).Clicked() {
									s.mu.Lock()
									s.confirmRemoveEmail = ""
									s.mu.Unlock()
									s.removeAccountAsync(win, acc.Email)
								}
							})
						} else {
							ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
								if acc.HasSavedLogin {
									if ui.ButtonBase(c).Padding(3, 8).Radius(5).Background(p.accentSoft).Cursor(ui.CursorPointer).Children(func() {
										ui.Text(c, s.tr("account.switch")).FontSize(9.5).Bold().TextColor(p.accent)
									}).Clicked() {
										s.switchAccountAsync(win, acc.Email)
									}
								}
								if ui.ButtonBase(c).Padding(3, 8).Radius(5).Background(p.dangerSoft).Cursor(ui.CursorPointer).Children(func() {
									ui.Text(c, s.tr("account.remove")).FontSize(9.5).Bold().TextColor(p.danger)
								}).Clicked() {
									s.mu.Lock()
									s.confirmRemoveEmail = acc.Email
									s.mu.Unlock()
								}
							})
						}
					}
				})
			})
		}
	})
}

func (s *uiState) renderOnboardingCard(c *ui.Context, p palette, collapsible bool) {
	s.mu.Lock()
	email := s.addEmail
	adding := s.adding
	oauth := s.oauth
	code := s.oauthCode
	win := s.win
	s.mu.Unlock()

	oauthBusy := oauth.State == accounts.OAuthStateStarting || oauth.State == accounts.OAuthStateWaiting || oauth.State == accounts.OAuthStateAuthorize

	ui.Column(c).FillWidth().Padding(10, 11).Gap(8).Radius(12).Background(p.card).Border(1, p.line).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Text(c, s.tr("account.addToggle")).FontSize(11).Bold().TextColor(p.fg)
			if collapsible && !oauthBusy {
				if ui.ButtonBase(c).Padding(4).Radius(5).Cursor(ui.CursorPointer).Children(func() {
					ui.Icon(c, chevronDownSVG).Size(12, 12).TextColor(p.muted)
				}).Clicked() {
					s.mu.Lock()
					s.showAddAccount = false
					s.mu.Unlock()
					s.syncWindowHeight(win)
				}
			}
		})

		oauthBg := p.accent
		oauthFg := p.onAccent
		oauthLabel := s.tr("account.oauthStart")
		if oauthBusy {
			oauthBg = p.soft
			oauthFg = p.muted
			oauthLabel = s.tr("account.oauthBusy")
		}
		if ui.ButtonBase(c).FillWidth().Padding(7, 10).Radius(8).Background(oauthBg).Cursor(ui.CursorPointer).Disabled(oauthBusy).Center().Children(func() {
			ui.Text(c, oauthLabel).FontSize(10.5).Bold().TextColor(oauthFg)
		}).Clicked() {
			s.startOAuthAsync(win)
		}

		if oauth.State != "" && oauth.State != "idle" {
			msg := oauth.Message
			if key := "oauth." + oauth.State; s.tr(key) != key {
				msg = s.tr(key)
			}
			if oauth.State == "error" && oauth.Message != "" {
				msg = oauth.Message
			}
			if oauth.State == "error" {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Padding(7, 9).Radius(8).Background(p.dangerSoft).Children(func() {
					ui.Text(c, msg).FontSize(10).TextColor(p.danger).Grow(1).Shrink(1)
					if ui.ButtonBase(c).Padding(2).Radius(4).Cursor(ui.CursorPointer).Children(func() {
						ui.Icon(c, closeSVG).Size(10.5, 10.5).TextColor(p.danger)
					}).Clicked() {
						s.mu.Lock()
						s.oauth = appservice.OAuthStatusView{State: "idle"}
						s.mu.Unlock()
						s.syncWindowHeight(win)
					}
				})
			} else {
				ui.Column(c).FillWidth().Gap(6).Padding(4, 0, 2, 0).Children(func() {
					ui.Text(c, msg).FontSize(10).TextColor(p.fg)

					if oauth.State == accounts.OAuthStateAuthorize && oauth.URL != "" {
						if ui.ButtonBase(c).Padding(4, 8).Radius(6).Background(p.accentSoft).Cursor(ui.CursorPointer).Children(func() {
							ui.Text(c, s.tr("oauth.openLink")).FontSize(10).Bold().TextColor(p.accent)
						}).Clicked() {
							go func(u string) { _ = mygo.Shell.OpenExternal(u) }(oauth.URL)
						}

						trimmedCode := strings.TrimSpace(code)
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
							codeInp := ui.TextInput(c, &code).Placeholder(s.tr("oauth.codePlaceholder")).Grow(1).Shrink(1)
							if code != s.oauthCode {
								s.mu.Lock()
								s.oauthCode = code
								s.mu.Unlock()
							}
							subBg, subFg := p.soft, p.muted
							if trimmedCode != "" {
								subBg, subFg = p.accent, p.onAccent
							}
							subBtn := ui.ButtonBase(c).Padding(4, 9).Radius(6).Background(subBg).Cursor(ui.CursorPointer).Disabled(trimmedCode == "").Children(func() {
								ui.Text(c, s.tr("oauth.submit")).FontSize(10).Bold().TextColor(subFg)
							})
							if (subBtn.Clicked() || codeInp.Submitted()) && trimmedCode != "" {
								s.submitOAuthCodeAsync(win, oauth.AttemptID, trimmedCode)
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
		}

		ui.Divider(c)

		ui.Text(c, s.tr("account.addHint")).FontSize(9.5).TextColor(p.muted)

		trimmedEmail := strings.TrimSpace(email)
		canImport := !adding && trimmedEmail != ""
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			inp := ui.TextInput(c, &email).Placeholder(s.tr("account.addPlaceholder")).Grow(1).Shrink(1)
			if email != s.addEmail {
				s.mu.Lock()
				s.addEmail = email
				s.mu.Unlock()
			}
			importBg, importFg := p.soft, p.muted
			if canImport {
				importBg, importFg = p.accentSoft, p.accent
			}
			addBtn := ui.ButtonBase(c).Padding(5, 10).Radius(7).Background(importBg).Cursor(ui.CursorPointer).Disabled(!canImport).Children(func() {
				ui.Text(c, s.tr("account.add")).FontSize(10.5).Bold().TextColor(importFg)
			})
			if (addBtn.Clicked() || inp.Submitted()) && canImport {
				s.addAccountAsync(win, trimmedEmail)
			}
		})
	})
}

func (s *uiState) renderProjectsTab(c *ui.Context, p palette, items []projects.ProjectView) {
	s.mu.Lock()
	loading := s.projectsLoading && !s.projectsLoaded
	refreshing := s.projectsLoading
	win := s.win
	s.mu.Unlock()

	if loading {
		ui.Column(c).FillWidth().Padding(32, 16).Center().Children(func() {
			ui.Spinner(c)
		})
		return
	}

	if len(items) == 0 {
		ui.Column(c).FillWidth().Padding(16, 16).Gap(6).Radius(12).Background(p.card).Border(1, p.line).Children(func() {
			ui.Text(c, s.tr("projects.emptyTitle")).FontSize(12).Bold().TextColor(p.fg)
			ui.Text(c, s.tr("projects.emptyDesc")).FontSize(10).TextColor(p.muted)
		})
		return
	}

	runningCount := 0
	for _, proj := range items {
		if proj.Active {
			runningCount++
		}
	}
	summary := s.tr("projects.stats", len(items), runningCount)
	ui.Row(c).FillWidth().Padding(0, 16, 0, 0).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Children(func() {
		ui.Text(c, summary).FontSize(10.5).TextColor(p.muted).Ellipsis("…").SingleLine().Grow(1).Shrink(1)
		refBtn := ui.ButtonBase(c).Padding(4, 9).Radius(7).Background(p.soft).Cursor(ui.CursorPointer).Disabled(refreshing).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
				ui.Icon(c, refreshSVG).Size(10.5, 10.5).TextColor(p.fg)
				label := s.tr("projects.refresh")
				if refreshing {
					label = "…"
				}
				ui.Text(c, label).FontSize(9.8).Bold().TextColor(p.fg)
			})
		})
		if refBtn.Clicked() {
			s.loadProjectsAsync(win, true)
		}
	})

	ui.Scroll(c).FillWidth().Grow(1).Children(func() {
		ui.Column(c).FillWidth().Padding(0, 16, 0, 0).Gap(8).Children(func() {
			for _, proj := range items {
				s.renderProjectCard(c, p, proj)
			}
		})
	})
}

func (s *uiState) renderProjectCard(c *ui.Context, p palette, proj projects.ProjectView) {
	s.mu.Lock()
	expanded := s.expandedProjects[proj.Path]
	confirmingRemove := s.confirmRemoveProject == proj.Path
	win := s.win
	s.mu.Unlock()

	borderCol := p.line
	borderW := float32(1)
	if proj.Active {
		borderCol = p.accent
		borderW = 1.5
	}

	limit := proj.ContextLimit
	if limit <= 0 {
		limit = 1_000_000
	}
	fresh := max64(0, proj.LatestInput)
	cacheRead := max64(0, proj.LatestCacheRead)
	output := max64(0, proj.LatestOutput)
	used := fresh + cacheRead + output
	free := max64(0, limit-used)
	pctVal := math.Min(100, (float64(used)/float64(limit))*100)
	pctInt := int(math.Round(pctVal))
	if pctInt < 0 {
		pctInt = 0
	}
	if pctInt > 100 {
		pctInt = 100
	}

	ui.Column(c).Key(proj.Path).FillWidth().Padding(10, 11).Gap(8).Radius(12).Background(p.card).Border(borderW, borderCol).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(9).Children(func() {
			if proj.Active {
				ui.Box(c).Width(14).Height(14).Radius(7).Background(p.okSoft).Center().Children(func() {
					ui.Box(c).Width(6).Height(6).Radius(3).Background(p.ok)
				})
			} else {
				ui.Box(c).Width(14).Height(14).Center().Children(func() {
					ui.Box(c).Width(6).Height(6).Radius(3).Background(p.muted)
				})
			}

			toggleClicked := false
			copyClicked := false
			infoCol := ui.Column(c).Grow(1).Shrink(1).Gap(2).Cursor(ui.CursorPointer).Children(func() {
				ui.Text(c, proj.Name).FontSize(12).Bold().TextColor(p.fg).Ellipsis("…").SingleLine()

				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(4).Children(func() {
					ui.Text(c, shortenHomePath(proj.Path)).FontSize(9.5).TextColor(p.muted).Ellipsis("…").SingleLine().Shrink(1)
					copyBtn := ui.ButtonBase(c).Padding(2).Radius(4).Cursor(ui.CursorPointer).Children(func() {
						ui.Icon(c, copySVG).Size(10.5, 10.5).TextColor(p.muted)
					})
					if copyBtn.Clicked() {
						copyClicked = true
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

				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(4).Children(func() {
					if proj.Model != "" {
						ui.Text(c, proj.Model).FontSize(9.5).TextColor(p.muted).Ellipsis("…").SingleLine().Shrink(1)
						ui.Text(c, "·").FontSize(9.5).TextColor(p.muted)
					}
					if proj.Active {
						ui.Text(c, s.tr("projects.running")).FontSize(9.5).TextColor(p.ok)
					} else {
						idleText := s.tr("projects.idle")
						if proj.LastActiveAt > 0 {
							sec := proj.LastActiveAt
							if sec > 1_000_000_000_000 {
								sec /= 1000
							}
							if age := s.formatRelativeAge(sec); age != "" {
								idleText = idleText + " · " + age
							}
						}
						ui.Text(c, idleText).FontSize(9.5).TextColor(p.muted).Ellipsis("…").SingleLine()
					}
				})
			})
			if infoCol.Clicked() && !copyClicked {
				toggleClicked = true
			}

			if !expanded {
				renderContextRing(c, p, pctInt)
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
				s.expandedProjects[proj.Path] = !expanded
				if s.confirmRemoveProject == proj.Path {
					s.confirmRemoveProject = ""
				}
				s.mu.Unlock()
			}
		})

		if expanded {
			ui.Divider(c)
			ui.Column(c).FillWidth().Padding(2, 0, 0, 0).Gap(8).Children(func() {
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
				ui.Divider(c)
				ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Padding(2, 0, 0, 0).Children(func() {
					ui.Text(c, s.tr("projects.footer", proj.StepCount, formatTokens(totalTokens))).FontSize(9.5).TextColor(p.muted)
					if confirmingRemove {
						ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
							if ui.ButtonBase(c).Padding(3, 7).Radius(5).Background(p.soft).Cursor(ui.CursorPointer).Children(func() {
								ui.Text(c, s.tr("account.cancelRemove")).FontSize(9.5).TextColor(p.muted)
							}).Clicked() {
								s.mu.Lock()
								s.confirmRemoveProject = ""
								s.mu.Unlock()
							}
							if ui.ButtonBase(c).Padding(3, 8).Radius(5).Background(p.danger).Cursor(ui.CursorPointer).Children(func() {
								ui.Text(c, s.tr("account.confirmRemove")).FontSize(9.5).Bold().TextColor(p.onAccent)
							}).Clicked() {
								s.mu.Lock()
								s.confirmRemoveProject = ""
								s.mu.Unlock()
								s.removeProjectAsync(win, proj.Path)
							}
						})
					} else {
						if ui.ButtonBase(c).Padding(3, 8).Radius(5).Background(p.dangerSoft).Cursor(ui.CursorPointer).Children(func() {
							ui.Text(c, s.tr("account.remove")).FontSize(9.5).Bold().TextColor(p.danger)
						}).Clicked() {
							s.mu.Lock()
							s.confirmRemoveProject = proj.Path
							s.mu.Unlock()
						}
					}
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
	if s.refreshingEmails[email] || s.switching {
		s.mu.Unlock()
		return
	}
	gen := s.switchGen
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
		} else if s.applyAccountsLocked(gen, items) {
			s.statusMsg = s.tr("status.refreshed")
			s.statusErr = false
		}
		s.mu.Unlock()
		s.triggerUpdate(win)
	}()
}

func (s *uiState) setRotationAsync(win *mygo.Window, email string, enabled bool) {
	if s.svc == nil {
		return
	}
	s.mu.Lock()
	if s.rotationBusy[email] {
		s.mu.Unlock()
		return
	}
	s.rotationBusy[email] = true
	for i := range s.accounts {
		if s.accounts[i].Email == email {
			s.accounts[i].Rotation = enabled
			break
		}
	}
	s.mu.Unlock()

	go func() {
		err := s.svc.SetRotation(email, enabled)
		s.mu.Lock()
		delete(s.rotationBusy, email)
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
			s.showAddAccount = false
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
				s.showAddAccount = false
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
		switch mode {
		case "dark":
			mygo.Theme.SetSource(mygo.ThemeDark)
		case "light":
			mygo.Theme.SetSource(mygo.ThemeLight)
		default:
			mygo.Theme.SetSource(mygo.ThemeSystem)
		}
		s.triggerUpdate(win)
	}
	go s.savePrefs()
}

func (s *uiState) renderSettingsTab(c *ui.Context, p palette) {
	s.mu.Lock()
	curTheme := s.theme
	curLang := s.lang
	scEnabled := s.shortcutEnabled
	scAccel := s.shortcutAccel
	scRecording := s.shortcutRecording
	updChecking := s.updateChecking
	updApplying := s.updateApplying
	updAvail := s.updateAvailable
	latestVer := s.latestVersion
	latestURL := s.latestURL
	updNotice := s.updateNotice
	statusMsg := s.statusMsg
	statusErr := s.statusErr
	win := s.win
	s.mu.Unlock()

	if statusMsg != "" && statusErr {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(7, 10).Radius(9).Background(p.dangerSoft).Border(1, p.danger).Children(func() {
			ui.Text(c, statusMsg).FontSize(10).TextColor(p.danger).Grow(1).Shrink(1)
			if ui.ButtonBase(c).Padding(2).Radius(4).Cursor(ui.CursorPointer).Children(func() {
				ui.Icon(c, closeSVG).Size(11, 11).TextColor(p.danger)
			}).Clicked() {
				s.mu.Lock()
				s.statusMsg = ""
				s.statusErr = false
				s.mu.Unlock()
			}
		})
	}

	ui.Scroll(c).FillWidth().Grow(1).Children(func() {
		ui.Column(c).FillWidth().Padding(0, 16, 0, 0).Gap(10).Children(func() {
			// Appearance
			ui.Column(c).FillWidth().Padding(12).Gap(8).Radius(12).Background(p.card).Border(1, p.line).Children(func() {
				ui.Text(c, s.tr("settings.appearance")).FontSize(11.5).Bold().TextColor(p.fg)
				type themeOpt struct {
					val   string
					label string
				}
				opts := []themeOpt{
					{"", s.tr("settings.theme.system")},
					{"light", s.tr("settings.theme.light")},
					{"dark", s.tr("settings.theme.dark")},
				}
				ui.Row(c).FillWidth().Padding(3).Gap(3).Radius(9).Background(p.soft).Children(func() {
					for _, opt := range opts {
						active := curTheme == opt.val
						btn := ui.ButtonBase(c).Grow(1).Padding(6, 0).Radius(7).Center().Cursor(ui.CursorPointer)
						if active {
							btn = btn.Background(p.pillBg)
						}
						if btn.Children(func() {
							col := p.muted
							if active {
								col = p.pillText
							}
							t := ui.Text(c, opt.label).FontSize(10.5).TextColor(col)
							if active {
								t.Bold()
							}
						}).Clicked() && !active {
							s.setThemeMode(opt.val)
						}
					}
				})
			})

			// Language
			ui.Column(c).FillWidth().Padding(12).Gap(8).Radius(12).Background(p.card).Border(1, p.line).Children(func() {
				ui.Text(c, s.tr("settings.language")).FontSize(11.5).Bold().TextColor(p.fg)
				type langOpt struct {
					val   langCode
					label string
				}
				opts := []langOpt{
					{langZH, s.tr("settings.lang.zh")},
					{langEN, s.tr("settings.lang.en")},
				}
				ui.Row(c).FillWidth().Padding(3).Gap(3).Radius(9).Background(p.soft).Children(func() {
					for _, opt := range opts {
						active := curLang == opt.val
						btn := ui.ButtonBase(c).Grow(1).Padding(6, 0).Radius(7).Center().Cursor(ui.CursorPointer)
						if active {
							btn = btn.Background(p.pillBg)
						}
						if btn.Children(func() {
							col := p.muted
							if active {
								col = p.pillText
							}
							t := ui.Text(c, opt.label).FontSize(10.5).TextColor(col)
							if active {
								t.Bold()
							}
						}).Clicked() && !active {
							s.mu.Lock()
							s.lang = opt.val
							s.mu.Unlock()
							go s.savePrefs()
						}
					}
				})
			})

			// Global Shortcut
			if scRecording {
				mods := c.Modifiers()
				keys := c.KeysDown()
				for _, k := range keys {
					if k == ui.KeyEscape {
						s.mu.Lock()
						s.shortcutRecording = false
						s.mu.Unlock()
						scRecording = false
						break
					}
					if accel, ok := buildShortcutAccel(mods, k); ok {
						s.mu.Lock()
						s.shortcutRecording = false
						s.shortcutEnabled = true
						s.shortcutAccel = accel
						s.shortcutInput = accel
						s.mu.Unlock()
						scRecording = false
						scEnabled = true
						scAccel = accel
						s.applyShortcut(win)
						go s.savePrefs()
						break
					}
				}
			}

			ui.Row(c).FillWidth().Padding(10, 12).Radius(12).Background(p.card).Border(1, p.line).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
				ui.Text(c, s.tr("settings.shortcut")).FontSize(11.5).Bold().TextColor(p.fg)

				ui.Row(c).AlignItems(ui.Center).Gap(7).Children(func() {
					pillBg := p.soft
					pillBorder := p.line
					pillFg := p.fg
					pillLabel := s.tr("settings.shortcutRecord")
					if scRecording {
						pillBg = p.accentSoft
						pillBorder = p.accent
						pillFg = p.accent
						if modPreview := formatModsSymbols(c.Modifiers()); modPreview != "" {
							pillLabel = modPreview + "…"
						} else {
							pillLabel = s.tr("settings.shortcutListening")
						}
					} else if scEnabled && scAccel != "" {
						pillLabel = formatShortcutSymbols(scAccel)
					} else {
						pillFg = p.muted
					}

					recBtn := ui.ButtonBase(c).Width(116).Height(26).Radius(7).Background(pillBg).Border(1, pillBorder).Center().Cursor(ui.CursorPointer).Children(func() {
						ui.Text(c, pillLabel).FontSize(10.5).Bold().TextColor(pillFg).Ellipsis("…").SingleLine()
					})
					if recBtn.Clicked() {
						s.mu.Lock()
						s.shortcutRecording = !scRecording
						s.mu.Unlock()
					}

					if scEnabled && scAccel != "" {
						clearBtn := ui.ButtonBase(c).Width(18).Height(18).Radius(9).Background(p.muted).Center().Cursor(ui.CursorPointer).Children(func() {
							ui.Icon(c, closeSVG).Size(10, 10).TextColor(p.card)
						})
						if clearBtn.Clicked() {
							s.mu.Lock()
							s.shortcutRecording = false
							s.shortcutEnabled = false
							s.shortcutAccel = ""
							s.shortcutInput = ""
							s.statusMsg = s.tr("settings.shortcutOff")
							s.statusErr = false
							s.mu.Unlock()
							s.applyShortcut(win)
							go s.savePrefs()
						}
					}
				})
			})

			// About & Updates
			ui.Column(c).FillWidth().Padding(12).Gap(8).Radius(12).Background(p.card).Border(1, p.line).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Text(c, s.tr("settings.about")).FontSize(11.5).Bold().TextColor(p.fg)
						ui.Box(c).Padding(2, 6).Radius(999).Background(p.soft).Children(func() {
							ui.Text(c, updater.Version).FontSize(9.5).TextColor(p.muted)
						})
					})
					checkLabel := s.tr("settings.checkUpdate")
					if updChecking {
						checkLabel = s.tr("settings.checkingUpdate")
					}
					if ui.ButtonBase(c).Padding(4, 9).Radius(7).Background(p.soft).Cursor(ui.CursorPointer).Disabled(updChecking || updApplying).Children(func() {
						ui.Text(c, checkLabel).FontSize(9.8).Bold().TextColor(p.fg)
					}).Clicked() {
						s.checkForUpdatesAsync(win, true)
					}
				})

				if updAvail && latestVer != "" {
					ui.Column(c).FillWidth().Gap(7).Padding(8, 9).Radius(8).Background(p.accentSoft).Children(func() {
						ui.Text(c, s.tr("settings.updateAvail", latestVer)).FontSize(10.5).Bold().TextColor(p.accent)
						ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
							updLabel := s.tr("settings.updateNow")
							if updApplying {
								updLabel = s.tr("settings.updating")
							}
							if ui.ButtonBase(c).Padding(5, 10).Radius(7).Background(p.accent).Cursor(ui.CursorPointer).Disabled(updApplying).Children(func() {
								ui.Text(c, updLabel).FontSize(10).Bold().TextColor(p.onAccent)
							}).Clicked() {
								s.selfUpdateAsync(win)
							}
							if latestURL != "" {
								if ui.ButtonBase(c).Padding(5, 10).Radius(7).Background(p.card).Cursor(ui.CursorPointer).Children(func() {
									ui.Text(c, s.tr("settings.releaseNotes")).FontSize(10).TextColor(p.fg)
								}).Clicked() {
									go func(u string) { _ = mygo.Shell.OpenExternal(u) }(latestURL)
								}
							}
						})
					})
				} else if updNotice != "" {
					ui.Text(c, updNotice).FontSize(9.8).TextColor(p.muted)
				}
			})
		})
	})
}

func (s *uiState) applyShortcut(win *mygo.Window) {
	if win == nil {
		return
	}
	s.mu.Lock()
	prev := s.registeredAccel
	enabled := s.shortcutEnabled
	accel := strings.TrimSpace(s.shortcutAccel)
	s.mu.Unlock()

	if enabled && accel != "" && prev == accel {
		return
	}
	if prev != "" {
		mygo.GlobalShortcut.Unregister(prev)
	}
	if !enabled || accel == "" {
		s.mu.Lock()
		s.registeredAccel = ""
		s.mu.Unlock()
		return
	}
	toggleHandler := func() {
		if win.IsVisible() {
			win.Hide()
			return
		}
		win.Show()
		win.Focus()
	}
	err := mygo.GlobalShortcut.Register(accel, toggleHandler)
	if err != nil && prev != "" {
		if rollbackErr := mygo.GlobalShortcut.Register(prev, toggleHandler); rollbackErr == nil {
			s.mu.Lock()
			s.registeredAccel = prev
			s.shortcutAccel = prev
			s.shortcutInput = prev
			s.shortcutEnabled = true
			s.statusMsg = err.Error()
			s.statusErr = true
			s.mu.Unlock()
			s.triggerUpdate(win)
			return
		}
	}
	s.mu.Lock()
	if err != nil {
		s.registeredAccel = ""
		s.statusMsg = err.Error()
		s.statusErr = true
	} else {
		s.registeredAccel = accel
		if prev != "" && prev != accel {
			s.statusMsg = s.tr("settings.shortcutSaved", formatShortcutSymbols(accel))
			s.statusErr = false
		}
	}
	s.mu.Unlock()
	s.triggerUpdate(win)
}

func maxFloat(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
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
		Theme           string   `json:"theme,omitempty"`
		Lang            langCode `json:"lang,omitempty"`
		ShortcutEnabled *bool    `json:"shortcutEnabled,omitempty"`
		ShortcutAccel   string   `json:"shortcutAccel,omitempty"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return
	}
	s.mu.Lock()
	s.theme = payload.Theme
	if payload.Lang == langZH || payload.Lang == langEN {
		s.lang = payload.Lang
	}
	if payload.ShortcutEnabled != nil {
		s.shortcutEnabled = *payload.ShortcutEnabled
	}
	if strings.TrimSpace(payload.ShortcutAccel) != "" {
		s.shortcutAccel = strings.TrimSpace(payload.ShortcutAccel)
		s.shortcutInput = s.shortcutAccel
	}
	s.mu.Unlock()
}

func (s *uiState) savePrefs() {
	path := uiPrefsPath()
	if path == "" {
		return
	}
	s.mu.Lock()
	enabled := s.shortcutEnabled
	payload := struct {
		Theme           string   `json:"theme,omitempty"`
		Lang            langCode `json:"lang,omitempty"`
		ShortcutEnabled *bool    `json:"shortcutEnabled,omitempty"`
		ShortcutAccel   string   `json:"shortcutAccel,omitempty"`
	}{
		Theme:           s.theme,
		Lang:            s.lang,
		ShortcutEnabled: &enabled,
		ShortcutAccel:   s.shortcutAccel,
	}
	s.mu.Unlock()
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, data, 0o600)
}

var (
	trackRingSVG = ui.MustParseSVG([]byte(`<svg viewBox="0 0 36 36"><circle cx="18" cy="18" r="14" fill="none" stroke="currentColor" stroke-width="3.2"/></svg>`))
	ringSVGMu    sync.Mutex
	ringSVGCache = make(map[int]*ui.SVG)
)

func getContextArcSVG(pct int) *ui.SVG {
	if pct <= 0 {
		return nil
	}
	if pct >= 100 {
		return trackRingSVG
	}
	ringSVGMu.Lock()
	if cached, ok := ringSVGCache[pct]; ok {
		ringSVGMu.Unlock()
		return cached
	}
	ringSVGMu.Unlock()

	angle := 2 * math.Pi * float64(pct) / 100.0
	x := 18.0 + 14.0*math.Sin(angle)
	y := 18.0 - 14.0*math.Cos(angle)
	largeArc := 0
	if pct > 50 {
		largeArc = 1
	}
	raw := fmt.Sprintf(
		`<svg viewBox="0 0 36 36"><path d="M 18 4 A 14 14 0 %d 1 %.3f %.3f" fill="none" stroke="currentColor" stroke-width="3.2" stroke-linecap="round"/></svg>`,
		largeArc, x, y,
	)
	svg := ui.MustParseSVG([]byte(raw))
	ringSVGMu.Lock()
	ringSVGCache[pct] = svg
	ringSVGMu.Unlock()
	return svg
}

func formatModsSymbols(mods ui.Modifiers) string {
	var b strings.Builder
	if mods&ui.Ctrl != 0 {
		b.WriteString("⌃")
	}
	if mods&ui.Alt != 0 {
		b.WriteString("⌥")
	}
	if mods&ui.Shift != 0 {
		b.WriteString("⇧")
	}
	if mods&ui.Super != 0 {
		b.WriteString("⌘")
	}
	return b.String()
}

func formatShortcutSymbols(accel string) string {
	parts := strings.Split(accel, "+")
	var hasCtrl, hasAlt, hasShift, hasCmd bool
	var keyPart string
	for _, raw := range parts {
		tok := strings.TrimSpace(raw)
		switch strings.ToLower(tok) {
		case "control", "ctrl":
			hasCtrl = true
		case "alt", "option":
			hasAlt = true
		case "shift":
			hasShift = true
		case "command", "cmd", "super", "commandorcontrol", "cmdorctrl":
			hasCmd = true
		default:
			if tok != "" {
				keyPart = strings.ToUpper(tok)
			}
		}
	}
	var b strings.Builder
	if hasCtrl {
		b.WriteString("⌃")
	}
	if hasAlt {
		b.WriteString("⌥")
	}
	if hasShift {
		b.WriteString("⇧")
	}
	if hasCmd {
		b.WriteString("⌘")
	}
	b.WriteString(keyPart)
	return b.String()
}

func keyNameForAccel(k ui.Key) (string, bool) {
	if k >= ui.KeyA && k <= ui.KeyZ {
		return string(rune('A' + (k - ui.KeyA))), true
	}
	if k >= ui.Key0 && k <= ui.Key9 {
		return string(rune('0' + (k - ui.Key0))), true
	}
	if k >= ui.KeyF1 && k <= ui.KeyF12 {
		return fmt.Sprintf("F%d", 1+(k-ui.KeyF1)), true
	}
	switch k {
	case ui.KeySpace:
		return "Space", true
	case ui.KeyEnter:
		return "Enter", true
	case ui.KeyTab:
		return "Tab", true
	case ui.KeyBackspace:
		return "Backspace", true
	case ui.KeyDelete:
		return "Delete", true
	case ui.KeyUp:
		return "Up", true
	case ui.KeyDown:
		return "Down", true
	case ui.KeyLeft:
		return "Left", true
	case ui.KeyRight:
		return "Right", true
	case ui.KeyMinus:
		return "-", true
	case ui.KeyEqual:
		return "=", true
	case ui.KeyComma:
		return ",", true
	case ui.KeyPeriod:
		return ".", true
	case ui.KeySlash:
		return "/", true
	case ui.KeySemicolon:
		return ";", true
	case ui.KeyBracketLeft:
		return "[", true
	case ui.KeyBracketRight:
		return "]", true
	case ui.KeyBackslash:
		return "\\", true
	case ui.KeyBackquote:
		return "`", true
	default:
		return "", false
	}
}

func buildShortcutAccel(mods ui.Modifiers, k ui.Key) (string, bool) {
	keyName, ok := keyNameForAccel(k)
	if !ok {
		return "", false
	}
	isFnKey := k >= ui.KeyF1 && k <= ui.KeyF12
	if mods == 0 && !isFnKey {
		return "", false
	}
	var parts []string
	if mods&ui.Ctrl != 0 {
		parts = append(parts, "Control")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	if mods&ui.Super != 0 {
		parts = append(parts, "Command")
	}
	parts = append(parts, keyName)
	return strings.Join(parts, "+"), true
}

func renderContextRing(c *ui.Context, p palette, pct int) {
	ringCol := p.accent
	if pct >= 80 {
		ringCol = p.danger
	} else if pct >= 55 {
		ringCol = p.warnText
	}
	arcSVG := getContextArcSVG(pct)
	ui.Box(c).Width(34).Height(34).Children(func() {
		ui.Icon(c, trackRingSVG).Size(34, 34).TextColor(p.track)
		if arcSVG != nil {
			ui.Box(c).Absolute().Left(0).Top(0).Width(34).Height(34).Children(func() {
				ui.Icon(c, arcSVG).Size(34, 34).TextColor(ringCol)
			})
		}
		ui.Box(c).Absolute().Left(0).Top(0).Width(34).Height(34).Center().Children(func() {
			ui.Text(c, formatPctInt(pct)).FontSize(8.5).Bold().TextColor(p.fg)
		})
	})
}

func shortenHomePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}

func renderMiniQuotaRow(c *ui.Context, p palette, label string, buckets map[string]usage.Bucket, primary, fallback string) {
	b, ok := buckets[primary]
	if !ok {
		b, ok = buckets[fallback]
	}
	if !ok {
		return
	}
	pct := b.Percent
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	barColor := p.quotaColor(pct)
	ui.Row(c).AlignItems(ui.Center).Gap(5).Children(func() {
		ui.Text(c, label).FontSize(9.2).TextColor(p.muted).Width(10)
		ui.Box(c).Width(28).Height(4).Radius(999).Background(p.track).Children(func() {
			if pct > 0 {
				ui.Box(c).WidthPercent(float32(pct)).Height(4).Radius(999).Background(barColor)
			}
		})
		ui.Text(c, formatPctInt(pct)).FontSize(9.5).TextColor(p.fg).Width(28)
	})
}

func (s *uiState) renderQuotaBar(c *ui.Context, p palette, key string, b usage.Bucket) {
	pct := b.Percent
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	barColor := p.quotaColor(pct)
	ui.Column(c).FillWidth().Gap(3).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, s.bucketLabel(key)).FontSize(10).TextColor(p.fg)
				ui.Text(c, formatPctInt(pct)).FontSize(10).Bold().TextColor(p.fg)
			})
			if resetText := s.formatResetCountdown(pct, b.Reset); resetText != "" {
				ui.Text(c, resetText).FontSize(9.5).TextColor(p.muted)
			}
		})
		ui.Box(c).FillWidth().Height(5).Radius(999).Background(p.track).Children(func() {
			if pct > 0 {
				ui.Box(c).WidthPercent(float32(pct)).Height(5).Radius(999).Background(barColor)
			}
		})
	})
}

func renderModelTags(c *ui.Context, p palette, members []string, isCurrent bool, activeModel string) {
	activeLower := strings.ToLower(strings.TrimSpace(activeModel))
	ui.Row(c).FillWidth().Wrap().Gap(4).Children(func() {
		for _, m := range members {
			name := strings.TrimSpace(m)
			if name == "" {
				continue
			}
			isSelected := isCurrent && activeLower != "" && strings.Contains(activeLower, strings.ToLower(name))
			bg := p.soft
			fg := p.muted
			label := name
			if isSelected {
				bg = p.accentSoft
				fg = p.accent
				label = "★ " + name
			}
			ui.Box(c).Padding(2, 6).Radius(5).Background(bg).Children(func() {
				txt := ui.Text(c, label).FontSize(9.2).TextColor(fg)
				if isSelected {
					txt.Bold()
				}
			})
		}
	})
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
