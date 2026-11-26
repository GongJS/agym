package usage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gongjs/agym/internal/accounts"
)

const usageReport = `{"command":{"data":{"groups":[{"name":"gemini-shared","description":"Models within this group: gemini-2.5-pro","buckets":[{"window":"weekly","remaining_fraction":0.735,"reset_time":"tomorrow"}]},{"name":"claude","buckets":[{"window":"5h","remaining_fraction":0.5}]}]}}}`

type fakeRunner struct{ result ProbeResult }

func (r fakeRunner) QueryUsage(context.Context, func() bool) ProbeResult { return r.result }

type fakeKeychain map[string]string

func (k fakeKeychain) Read(_ context.Context, service, account string) (string, error) {
	value, ok := k[service+"/"+account]
	if !ok {
		return "", accounts.ErrNotFound
	}
	return value, nil
}

func (k fakeKeychain) Write(_ context.Context, service, account, secret string) error {
	k[service+"/"+account] = secret
	return nil
}

func (k fakeKeychain) Delete(_ context.Context, service, account string) error {
	delete(k, service+"/"+account)
	return nil
}

func encodedLogin(email string) string {
	payload, _ := json.Marshal(map[string]any{"id_token_email": email, "token": map[string]string{"refresh_token": "private-token"}})
	return "go-keyring-base64:" + base64.StdEncoding.EncodeToString(payload)
}

func setupRefresh(t *testing.T, kc fakeKeychain) (*accounts.Roster, string) {
	t.Helper()
	dir := t.TempDir()
	paths := accounts.Paths{AccountsFile: filepath.Join(dir, "agy-accounts.txt"), ManualOnlyFile: filepath.Join(dir, "agy-manual-only.txt")}
	if err := os.WriteFile(paths.AccountsFile, []byte("a@example.com\nb@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	roster := accounts.NewRosterWithLockPath(paths, kc, filepath.Join(dir, "slot.lock"))
	return roster, filepath.Join(dir, "quota_cache.json")
}

func TestRefreshProbesOtherAccountAndRestoresOriginalSlot(t *testing.T) {
	a := encodedLogin("a@example.com")
	b := encodedLogin("b@example.com")
	kc := fakeKeychain{
		"gemini/antigravity":              a,
		"gemini-agy-backup/a@example.com": a,
		"gemini-agy-backup/b@example.com": b,
	}
	roster, cachePath := setupRefresh(t, kc)
	refresher := NewRefresher(roster, fakeRunner{ProbeResult{ExitCode: 0, Stdout: usageReport, Outcome: "exited"}}, cachePath)
	cache, err := refresher.Refresh(context.Background(), []string{"a@example.com", "b@example.com"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := kc["gemini/antigravity"]; got != a {
		t.Fatal("refresh did not restore the original live login")
	}
	if got := kc["gemini-agy-backup/b@example.com"]; got != b {
		t.Fatal("refresh changed the saved backup")
	}
	view, ok := cache.View("b@example.com")
	if !ok || view.Status != "ok" || view.Buckets["gw"].Percent != 74 || view.Buckets["c5"].Percent != 50 {
		t.Fatalf("refreshed quota = %#v, %v", view, ok)
	}
	raw, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if containsString(string(raw), "private-token") {
		t.Fatal("quota cache contains a raw login token")
	}
}

func TestRefreshYieldRestoresSlotWithoutRecordingQuota(t *testing.T) {
	a := encodedLogin("a@example.com")
	b := encodedLogin("b@example.com")
	kc := fakeKeychain{"gemini/antigravity": a, "gemini-agy-backup/b@example.com": b}
	roster, cachePath := setupRefresh(t, kc)
	refresher := NewRefresher(roster, fakeRunner{ProbeResult{ExitCode: -1, Stderr: "yield", Outcome: "yielded"}}, cachePath)
	cache, err := refresher.Refresh(context.Background(), []string{"b@example.com"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if kc["gemini/antigravity"] != a {
		t.Fatal("yielded probe did not restore original slot")
	}
	if _, ok := cache.View("b@example.com"); ok {
		t.Fatal("yielded probe wrote a quota result")
	}
}

func TestRefreshMissingBackupRecordsNoKeychainWithoutChangingSlot(t *testing.T) {
	a := encodedLogin("a@example.com")
	kc := fakeKeychain{"gemini/antigravity": a}
	roster, cachePath := setupRefresh(t, kc)
	refresher := NewRefresher(roster, fakeRunner{ProbeResult{ExitCode: 0, Stdout: usageReport, Outcome: "exited"}}, cachePath)
	cache, err := refresher.Refresh(context.Background(), []string{"b@example.com"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if kc["gemini/antigravity"] != a {
		t.Fatal("missing backup changed the live slot")
	}
	view, ok := cache.View("b@example.com")
	if !ok || view.Status != "no_keychain" {
		t.Fatalf("missing backup view = %#v, %v", view, ok)
	}
}

func TestRefreshRejectsEmptyCurrentSlot(t *testing.T) {
	roster, cachePath := setupRefresh(t, fakeKeychain{"gemini-agy-backup/b@example.com": encodedLogin("b@example.com")})
	refresher := NewRefresher(roster, fakeRunner{}, cachePath)
	_, err := refresher.Refresh(context.Background(), []string{"b@example.com"}, true)
	if err == nil || errors.Is(err, accounts.ErrNotFound) {
		t.Fatalf("Refresh error = %v, want safe empty-slot error", err)
	}
}

func TestQuotaFromResultDoesNotExposeStderr(t *testing.T) {
	for _, result := range []ProbeResult{
		{ExitCode: 1, Outcome: "exited", Stderr: "not eligible: https://accounts.google.com/verify?token=private-token"},
		{ExitCode: 1, Outcome: "exited", Stderr: "private-token caused failure"},
	} {
		quota := quotaFromResult(result)
		if containsString(quota.Message, "private-token") || containsString(quota.Message, "https://") {
			t.Fatalf("quota message leaked raw stderr: %#v", quota)
		}
	}
}

func TestRefreshSubsetRetainsOtherAccounts(t *testing.T) {
	kc := fakeKeychain{
		"gemini/antigravity":              encodedLogin("a@example.com"),
		"gemini-agy-backup/a@example.com": encodedLogin("a@example.com"),
		"gemini-agy-backup/b@example.com": encodedLogin("b@example.com"),
	}
	roster, cachePath := setupRefresh(t, kc)
	refresher := NewRefresher(roster, fakeRunner{ProbeResult{ExitCode: 0, Stdout: usageReport, Outcome: "exited"}}, cachePath)
	if _, err := refresher.Refresh(context.Background(), []string{"a@example.com", "b@example.com"}, true); err != nil {
		t.Fatalf("initial Refresh error = %v", err)
	}
	cache, err := refresher.RefreshSubset(context.Background(), []string{"a@example.com", "b@example.com"}, []string{"a@example.com"}, true)
	if err != nil {
		t.Fatalf("RefreshSubset error = %v", err)
	}
	if _, ok := cache.View("a@example.com"); !ok {
		t.Fatal("expected refreshed account a@example.com in cache")
	}
	if _, ok := cache.View("b@example.com"); !ok {
		t.Fatal("expected non-targeted account b@example.com to remain retained in cache")
	}
}
