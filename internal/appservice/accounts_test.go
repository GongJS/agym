package appservice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongjs/agym/internal/accounts"
	"github.com/gongjs/agym/internal/catalog"
	"github.com/gongjs/agym/internal/usage"
)

type serviceKeychain map[string]string

func (k serviceKeychain) Read(_ context.Context, service, account string) (string, error) {
	value, ok := k[service+"/"+account]
	if !ok {
		return "", accounts.ErrNotFound
	}
	return value, nil
}

func (k serviceKeychain) Write(_ context.Context, service, account, secret string) error {
	k[service+"/"+account] = secret
	return nil
}

func (k serviceKeychain) Delete(_ context.Context, service, account string) error {
	delete(k, service+"/"+account)
	return nil
}

func TestListAccountsProjectsQuotaMetadataAndCatalogWithoutCredentials(t *testing.T) {
	const email = "active@example.com"
	const privateToken = "refresh-token-must-not-leave-core"
	dir := t.TempDir()
	paths := accounts.Paths{
		AccountsFile:   filepath.Join(dir, "agy-accounts.txt"),
		ManualOnlyFile: filepath.Join(dir, "agy-manual-only.txt"),
	}
	if err := os.WriteFile(paths.AccountsFile, []byte(email+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loginJSON := `{"id_token_email":"` + email + `","token":{"refresh_token":"` + privateToken + `"}}`
	rawLogin := "go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte(loginJSON))
	keychain := serviceKeychain{
		"gemini/antigravity":         rawLogin,
		"gemini-agy-backup/" + email: rawLogin,
	}
	roster := accounts.NewRosterWithLockPath(paths, keychain, filepath.Join(dir, "slot.lock"))
	service := NewAccounts(roster)
	service.cachePath = filepath.Join(dir, "quota.json")
	service.refresher.CatalogPath = filepath.Join(dir, "models.json")

	cache := usage.LoadCache(service.cachePath)
	cache.Record(email, usage.Quota{
		Status:     "ok",
		Buckets:    map[string]usage.Bucket{"gw": {Percent: 71, Reset: "soon"}},
		TierID:     "g1-pro-tier",
		TierName:   "Google AI Pro",
		RegionCode: "JP",
	}, time.Unix(1_800_000_000, 0))
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	models := catalog.New(service.refresher.CatalogPath)
	models.Record(email, []catalog.Model{
		{ID: "gemini-3-pro", Name: "Gemini 3 Pro", Tag: "New"},
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro"},
	}, 1_800_000_000, "gemini-3-pro")
	if err := models.Save(); err != nil {
		t.Fatal(err)
	}

	views, err := service.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("ListAccounts() returned %d views, want 1", len(views))
	}
	view := views[0]
	if view.Email != email || !view.Current || !view.InRoster || view.Quota == nil {
		t.Fatalf("account identity projection = %#v", view)
	}
	if view.Quota.TierName != "Google AI Pro" || view.Quota.RegionCode != "JP" {
		t.Fatalf("quota metadata = %#v", view.Quota)
	}
	if !view.ModelsKnown || view.DefaultModel != "gemini-3-pro" || len(view.AvailableModels) != 2 || view.AvailableModels[0].Tag != "New" {
		t.Fatalf("model catalog projection = %#v", view)
	}
	serialized, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), privateToken) || strings.Contains(string(serialized), rawLogin) {
		t.Fatalf("credential material escaped in AccountView JSON: %s", serialized)
	}
}

func TestOAuthStatusAndLifecycleGuards(t *testing.T) {
	var nilSvc *Accounts
	if got := nilSvc.OAuthStatus(); got.State != accounts.OAuthStateIdle {
		t.Fatalf("nil.OAuthStatus().State = %q, want %q", got.State, accounts.OAuthStateIdle)
	}
	nilSvc.Shutdown()

	dir := t.TempDir()
	roster := accounts.NewRosterWithLockPath(
		accounts.Paths{
			AccountsFile:   filepath.Join(dir, "agy-accounts.txt"),
			ManualOnlyFile: filepath.Join(dir, "agy-manual-only.txt"),
		},
		serviceKeychain{},
		filepath.Join(dir, "slot.lock"),
	)
	svc := NewAccounts(roster)
	if got := svc.OAuthStatus(); got.State != accounts.OAuthStateIdle {
		t.Fatalf("initial OAuthStatus().State = %q, want %q", got.State, accounts.OAuthStateIdle)
	}
	if err := svc.SubmitOAuthCode("missing", "123"); err == nil {
		t.Fatal("SubmitOAuthCode on inactive attempt succeeded, want error")
	}
	if err := svc.CancelOAuth("missing"); err == nil {
		t.Fatal("CancelOAuth on inactive attempt succeeded, want error")
	}
	svc.Shutdown()
}

func TestRosterMutationsViaAppService(t *testing.T) {
	const email = "user@example.com"
	dir := t.TempDir()
	paths := accounts.Paths{
		AccountsFile:   filepath.Join(dir, "agy-accounts.txt"),
		ManualOnlyFile: filepath.Join(dir, "agy-manual-only.txt"),
	}
	if err := os.WriteFile(paths.AccountsFile, []byte(email+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	roster := accounts.NewRosterWithLockPath(paths, serviceKeychain{"gemini-agy-backup/" + email: "token"}, filepath.Join(dir, "slot.lock"))
	svc := NewAccounts(roster)

	if err := svc.SetRotation(email, false); err != nil {
		t.Fatalf("SetRotation(false) = %v", err)
	}
	views, err := svc.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Rotation {
		t.Fatalf("after SetRotation(false), views = %#v", views)
	}

	if err := svc.RemoveAccount(email); err != nil {
		t.Fatalf("RemoveAccount() = %v", err)
	}
	views, err = svc.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 0 {
		t.Fatalf("after RemoveAccount(), len(views) = %d, want 0", len(views))
	}
}

func TestSyncAgySettingsPreservesExistingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	initial := "{\n  \"allowNonWorkspaceAccess\": true,\n  \"effort\": \"high\",\n  \"model\": \"Gemini 4 Hydrogen (Medium)\"\n}\n"
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := syncAgySettingsToPath(path, "Claude Sonnet 4.6 (Thinking)", "medium"); err != nil {
		t.Fatalf("syncAgySettingsToPath() error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, `"allowNonWorkspaceAccess": true`) ||
		!strings.Contains(got, `"model": "Claude Sonnet 4.6 (Thinking)"`) ||
		!strings.Contains(got, `"effort": "medium"`) {
		t.Fatalf("unexpected settings.json content:\n%s", got)
	}

	if err := syncAgySettingsToPath(path, "Gemini 4 Hydrogen (High)", "default"); err != nil {
		t.Fatalf("syncAgySettingsToPath(default) error = %v", err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"effort"`) {
		t.Fatalf("expected effort key to be removed for default, got:\n%s", string(raw))
	}
}
