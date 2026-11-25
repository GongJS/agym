package appservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/gongjs/agym/internal/accounts"
	"github.com/gongjs/agym/internal/catalog"
	"github.com/gongjs/agym/internal/usage"
)

// AccountView contains safe roster metadata and an optional normalized quota.
// Credential blobs never leave the shared core or macOS Keychain.
type AccountView struct {
	Email           string             `json:"email"`
	Current         bool               `json:"current"`
	InRoster        bool               `json:"inRoster"`
	Rotation        bool               `json:"rotation"`
	HasSavedLogin   bool               `json:"hasSavedLogin"`
	Quota           *usage.AccountView `json:"quota,omitempty"`
	ModelsKnown     bool               `json:"modelsKnown"`
	AvailableModels []catalog.Model    `json:"availableModels,omitempty"`
	DefaultModel    string             `json:"defaultModel,omitempty"`
}

// OAuthStatusView contains only safe progress information for the tray.
type OAuthStatusView struct {
	AttemptID string `json:"attemptId,omitempty"`
	State     string `json:"state"`
	URL       string `json:"url,omitempty"`
	Email     string `json:"email,omitempty"`
	Message   string `json:"message,omitempty"`
	ExpiresIn int    `json:"expiresIn,omitempty"`
}

// Accounts exposes shared account, quota, catalog and onboarding operations to Wails.
type Accounts struct {
	roster    *accounts.Roster
	refresher *usage.Refresher
	cachePath string

	oauth     *accounts.OAuthAdder
	oauthMu   sync.Mutex
	oauthView OAuthStatusView
	oauthCmds chan accounts.OAuthCommand
	oauthStop context.CancelFunc
}

func NewAccounts(roster *accounts.Roster) *Accounts {
	cachePath, _ := usage.DefaultCachePath()
	return &Accounts{
		roster:    roster,
		cachePath: cachePath,
		refresher: usage.NewRefresher(roster, usage.AgyRunner{}, cachePath),
		oauth:     accounts.NewOAuthAdder(roster),
	}
}

func (s *Accounts) ListAccounts() ([]AccountView, error) {
	return s.accountViews(context.Background(), false)
}

// RefreshQuotas performs a force refresh for each configured account and then
// returns the same roster projection used by ListAccounts.
func (s *Accounts) RefreshQuotas() ([]AccountView, error) {
	return s.accountViews(context.Background(), true)
}

func (s *Accounts) SwitchAccount(email string) error {
	return s.roster.Switch(context.Background(), email)
}

// AddExistingAccount adds an account that is already present in the agy
// Keychain backup. It does not initiate sign-in or modify the active slot.
func (s *Accounts) AddExistingAccount(email string) (bool, error) {
	return s.roster.Add(context.Background(), email)
}

// AddCurrentAccount saves the currently active login as a roster backup after
// the core revalidates that the live Keychain identity still matches email.
func (s *Accounts) AddCurrentAccount(email string) (bool, error) {
	return s.roster.AddCurrentAccount(context.Background(), email)
}

// StartOAuth starts one asynchronous official agy OAuth sign-in attempt.
func (s *Accounts) StartOAuth() (OAuthStatusView, error) {
	if s == nil || s.roster == nil || s.oauth == nil {
		return OAuthStatusView{}, fmt.Errorf("OAuth account service is not initialized")
	}
	s.oauthMu.Lock()
	if oauthActive(s.oauthView.State) {
		current := s.oauthView
		s.oauthMu.Unlock()
		return current, fmt.Errorf("an account sign-in is already in progress")
	}
	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		s.oauthMu.Unlock()
		return OAuthStatusView{}, fmt.Errorf("create sign-in attempt: %w", err)
	}
	attemptID := hex.EncodeToString(idBytes[:])
	ctx, cancel := context.WithCancel(context.Background())
	commands := make(chan accounts.OAuthCommand, 4)
	s.oauthStop = cancel
	s.oauthCmds = commands
	s.oauthView = OAuthStatusView{AttemptID: attemptID, State: accounts.OAuthStateStarting, Message: "Preparing official agy sign-in"}
	initial := s.oauthView
	s.oauthMu.Unlock()

	go func() {
		result, runErr := s.oauth.Run(ctx, commands, func(event accounts.OAuthEvent) {
			s.updateOAuth(attemptID, OAuthStatusView{
				AttemptID: attemptID,
				State:     event.State,
				URL:       event.URL,
				Email:     event.Email,
				Message:   event.Message,
				ExpiresIn: event.ExpiresIn,
			})
		})
		if runErr != nil {
			state := accounts.OAuthStateError
			message := runErr.Error()
			if ctx.Err() != nil || strings.Contains(strings.ToLower(message), "cancelled") {
				state = accounts.OAuthStateCancelled
				message = "Sign-in cancelled"
			}
			s.updateOAuth(attemptID, OAuthStatusView{AttemptID: attemptID, State: state, Message: message})
		} else {
			s.updateOAuth(attemptID, OAuthStatusView{AttemptID: attemptID, State: accounts.OAuthStateAdded, Email: result.Email, Message: "Account safely saved and added"})
		}
		cancel()
	}()
	return initial, nil
}

// OAuthStatus returns the latest safe progress snapshot.
func (s *Accounts) OAuthStatus() OAuthStatusView {
	if s == nil {
		return OAuthStatusView{State: accounts.OAuthStateIdle}
	}
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	if s.oauthView.State == "" {
		return OAuthStatusView{State: accounts.OAuthStateIdle}
	}
	return s.oauthView
}

// SubmitOAuthCode sends one authorization code to the active attempt.
func (s *Accounts) SubmitOAuthCode(attemptID, code string) error {
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	if s.oauthView.AttemptID != attemptID || !oauthActive(s.oauthView.State) || s.oauthCmds == nil {
		return fmt.Errorf("this sign-in attempt is no longer active")
	}
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 2048 || strings.ContainsAny(code, "\r\n") {
		return fmt.Errorf("authorization code is invalid")
	}
	select {
	case s.oauthCmds <- accounts.OAuthCommand{Code: code}:
		return nil
	default:
		return fmt.Errorf("the sign-in is busy; try submitting the code again")
	}
}

// CancelOAuth stops the active attempt and cleans up its official agy process.
func (s *Accounts) CancelOAuth(attemptID string) error {
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	if s.oauthView.AttemptID != attemptID || !oauthActive(s.oauthView.State) || s.oauthStop == nil {
		return fmt.Errorf("this sign-in attempt is no longer active")
	}
	if s.oauthCmds != nil {
		select {
		case s.oauthCmds <- accounts.OAuthCommand{Cancel: true}:
		default:
		}
	}
	s.oauthStop()
	return nil
}

// Shutdown cancels an active OAuth attempt before the tray process exits.
func (s *Accounts) Shutdown() {
	if s == nil {
		return
	}
	s.oauthMu.Lock()
	if s.oauthStop != nil {
		s.oauthStop()
	}
	s.oauthMu.Unlock()
}

func (s *Accounts) updateOAuth(attemptID string, update OAuthStatusView) {
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	if s.oauthView.AttemptID != attemptID {
		return
	}
	s.oauthView = update
}

func oauthActive(state string) bool {
	switch state {
	case accounts.OAuthStateStarting, accounts.OAuthStateWaiting, accounts.OAuthStateAuthorize:
		return true
	default:
		return false
	}
}

func (s *Accounts) RemoveAccount(email string) error {
	return s.roster.Remove(email)
}

func (s *Accounts) SetRotation(email string, enabled bool) error {
	return s.roster.SetRotation(email, enabled)
}

func (s *Accounts) accountViews(ctx context.Context, refresh bool) ([]AccountView, error) {
	if s.roster == nil || s.refresher == nil {
		return nil, fmt.Errorf("account service is not initialized")
	}
	items, err := s.roster.List(ctx)
	if err != nil {
		return nil, err
	}
	emails := make([]string, len(items))
	for i, item := range items {
		emails[i] = item.Email
	}
	cache := usage.LoadCache(s.cachePath)
	if refresh {
		cache, err = s.refresher.Refresh(ctx, emails, true)
		if err != nil {
			return nil, err
		}
	}
	modelCatalog := catalog.Load(s.refresher.CatalogPath)
	views := make([]AccountView, 0, len(items))
	for _, item := range items {
		view := AccountView{
			Email:         item.Email,
			Current:       item.Current,
			InRoster:      item.InRoster,
			Rotation:      item.Rotation,
			HasSavedLogin: item.HasSavedLogin,
		}
		if quota, ok := cache.View(item.Email); ok {
			view.Quota = &quota
		}
		if models, known := modelCatalog.Models(item.Email); known {
			view.ModelsKnown = true
			view.AvailableModels = models
			view.DefaultModel = modelCatalog.DefaultModel(item.Email)
		}
		views = append(views, view)
	}
	return views, nil
}
