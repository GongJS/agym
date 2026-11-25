package accounts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fakeKeychain map[string]string

func key(service, account string) string { return service + "/" + account }

func (f fakeKeychain) Read(_ context.Context, service, account string) (string, error) {
	value, ok := f[key(service, account)]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}
func (f fakeKeychain) Write(_ context.Context, service, account, secret string) error {
	f[key(service, account)] = secret
	return nil
}
func (f fakeKeychain) Delete(_ context.Context, service, account string) error {
	delete(f, key(service, account))
	return nil
}

func testRoster(t *testing.T) (*Roster, Paths, fakeKeychain) {
	t.Helper()
	dir := t.TempDir()
	paths := Paths{AccountsFile: filepath.Join(dir, "agy-accounts.txt"), ManualOnlyFile: filepath.Join(dir, "agy-manual-only.txt")}
	kc := fakeKeychain{
		key(liveSlotService, liveSlotAccount):        encodedLogin(t, "active@example.com"),
		key(backupSlotService, "active@example.com"): encodedLogin(t, "active@example.com"),
		key(backupSlotService, "other@example.com"):  encodedLogin(t, "other@example.com"),
	}
	return NewRoster(paths, kc), paths, kc
}

func encodedLogin(t *testing.T, email string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"id_token_email": email, "token": map[string]string{"refresh_token": "secret-never-returned"}})
	if err != nil {
		t.Fatal(err)
	}
	return keyringPrefix + base64.StdEncoding.EncodeToString(data)
}

func TestListReadsCompatibleRostersWithoutReturningSecrets(t *testing.T) {
	roster, paths, _ := testRoster(t)
	if err := os.WriteFile(paths.AccountsFile, []byte("active@example.com\n\nother@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ManualOnlyFile, []byte("other@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := roster.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Account{
		{Email: "active@example.com", Current: true, InRoster: true, Rotation: true, HasSavedLogin: true},
		{Email: "other@example.com", Current: false, InRoster: true, Rotation: false, HasSavedLogin: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %#v, want %#v", got, want)
	}
}

func TestAddRequiresSavedLoginAndIsIdempotent(t *testing.T) {
	roster, _, _ := testRoster(t)
	if _, err := roster.Add(context.Background(), "missing@example.com"); !errors.Is(err, ErrNoSavedLogin) {
		t.Fatalf("Add without backup error = %v, want ErrNoSavedLogin", err)
	}
	added, err := roster.Add(context.Background(), "other@example.com")
	if err != nil || !added {
		t.Fatalf("first Add() = (%v, %v), want (true, nil)", added, err)
	}
	added, err = roster.Add(context.Background(), "other@example.com")
	if err != nil || added {
		t.Fatalf("duplicate Add() = (%v, %v), want (false, nil)", added, err)
	}
}

func TestListIncludesCurrentAccountNotInRoster(t *testing.T) {
	roster, _, kc := testRoster(t)
	delete(kc, key(backupSlotService, "active@example.com"))

	got, err := roster.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Account{{Email: "active@example.com", Current: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %#v, want current-only account %#v", got, want)
	}
}

func TestAddCurrentAccountSavesMatchingLiveLogin(t *testing.T) {
	roster, paths, kc := testRoster(t)
	delete(kc, key(backupSlotService, "active@example.com"))

	added, err := roster.AddCurrentAccount(context.Background(), "active@example.com")
	if err != nil || !added {
		t.Fatalf("AddCurrentAccount() = (%v, %v), want (true, nil)", added, err)
	}
	if kc[key(backupSlotService, "active@example.com")] != kc[key(liveSlotService, liveSlotAccount)] {
		t.Fatal("active login was not copied into a verified backup")
	}
	accounts, err := readEmails(paths.AccountsFile)
	if err != nil || !reflect.DeepEqual(accounts, []string{"active@example.com"}) {
		t.Fatalf("roster = %v, %v; want active account", accounts, err)
	}
	added, err = roster.AddCurrentAccount(context.Background(), "active@example.com")
	if err != nil || added {
		t.Fatalf("duplicate AddCurrentAccount() = (%v, %v), want (false, nil)", added, err)
	}
}

func TestAddCurrentAccountRejectsChangedLiveIdentity(t *testing.T) {
	roster, _, kc := testRoster(t)
	kc[key(liveSlotService, liveSlotAccount)] = encodedLogin(t, "other@example.com")
	delete(kc, key(backupSlotService, "active@example.com"))

	if _, err := roster.AddCurrentAccount(context.Background(), "active@example.com"); err == nil {
		t.Fatal("AddCurrentAccount succeeded after the live account changed")
	}
	if _, exists := kc[key(backupSlotService, "active@example.com")]; exists {
		t.Fatal("mismatched live login was saved as the requested account")
	}
}

func TestRotationAndRemoveKeepCredentialBackup(t *testing.T) {
	roster, paths, kc := testRoster(t)
	if err := writeEmails(paths.AccountsFile, []string{"active@example.com", "other@example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := writeEmails(paths.ManualOnlyFile, []string{"other@example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := roster.SetRotation("other@example.com", true); err != nil {
		t.Fatal(err)
	}
	manual, err := readEmails(paths.ManualOnlyFile)
	if err != nil || len(manual) != 0 {
		t.Fatalf("manual-only after enable = %v, %v", manual, err)
	}
	if err := roster.SetRotation("other@example.com", false); err != nil {
		t.Fatal(err)
	}
	if err := roster.Remove("other@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Read(context.Background(), backupSlotService, "other@example.com"); err != nil {
		t.Fatalf("Remove deleted the saved Keychain backup: %v", err)
	}
	accounts, err := readEmails(paths.AccountsFile)
	if err != nil || !reflect.DeepEqual(accounts, []string{"active@example.com"}) {
		t.Fatalf("roster after remove = %v, %v", accounts, err)
	}
}

func TestParseLoginEmailSupportsLegacyPrefix(t *testing.T) {
	got, err := parseLoginEmail(encodedLogin(t, "person@example.com"))
	if err != nil || got != "person@example.com" {
		t.Fatalf("parseLoginEmail() = (%q, %v)", got, err)
	}
	if _, err := parseLoginEmail("not-base64"); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("invalid login error = %v", err)
	}
}
