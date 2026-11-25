package accounts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSwitchActivatesOnlyRegisteredSavedLogin(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{AccountsFile: filepath.Join(dir, "agy-accounts.txt"), ManualOnlyFile: filepath.Join(dir, "agy-manual-only.txt")}
	if err := writeEmails(paths.AccountsFile, []string{"active@example.com", "next@example.com"}); err != nil {
		t.Fatal(err)
	}
	kc := fakeKeychain{
		key(liveSlotService, liveSlotAccount):        encodedLogin(t, "active@example.com"),
		key(backupSlotService, "active@example.com"): encodedLogin(t, "active@example.com"),
		key(backupSlotService, "next@example.com"):   encodedLogin(t, "next@example.com"),
	}
	roster := NewRosterWithLockPath(paths, kc, filepath.Join(dir, "slot.lock"))
	if err := roster.Switch(context.Background(), "next@example.com"); err != nil {
		t.Fatal(err)
	}
	got, err := roster.CurrentEmail(context.Background())
	if err != nil || got != "next@example.com" {
		t.Fatalf("CurrentEmail() = (%q, %v), want next@example.com", got, err)
	}
	if err := roster.Switch(context.Background(), "missing@example.com"); err == nil {
		t.Fatal("Switch accepted an account not in the roster")
	}
	if got, _ := roster.CurrentEmail(context.Background()); got != "next@example.com" {
		t.Fatalf("failed switch changed active account to %q", got)
	}
}

func TestActivateRejectsMismatchedBackupBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{AccountsFile: filepath.Join(dir, "agy-accounts.txt"), ManualOnlyFile: filepath.Join(dir, "agy-manual-only.txt")}
	if err := writeEmails(paths.AccountsFile, []string{"next@example.com"}); err != nil {
		t.Fatal(err)
	}
	kc := fakeKeychain{
		key(liveSlotService, liveSlotAccount):      encodedLogin(t, "active@example.com"),
		key(backupSlotService, "next@example.com"): encodedLogin(t, "someone-else@example.com"),
	}
	roster := NewRosterWithLockPath(paths, kc, filepath.Join(dir, "slot.lock"))
	if err := roster.Switch(context.Background(), "next@example.com"); err == nil {
		t.Fatal("Switch accepted a backup for another account")
	}
	if got, _ := roster.CurrentEmail(context.Background()); got != "active@example.com" {
		t.Fatalf("rejected backup changed active account to %q", got)
	}
}

func TestBackgroundSlotReportsInteractiveWaiter(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{AccountsFile: filepath.Join(dir, "agy-accounts.txt"), ManualOnlyFile: filepath.Join(dir, "agy-manual-only.txt")}
	roster := NewRosterWithLockPath(paths, fakeKeychain{}, filepath.Join(dir, "slot.lock"))
	acquired := make(chan *LockedSlot, 1)
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- roster.WithSlotLock(context.Background(), true, func(slot *LockedSlot) error {
			acquired <- slot
			<-release
			return nil
		})
	}()
	var slot *LockedSlot
	select {
	case slot = <-acquired:
	case <-time.After(time.Second):
		t.Fatal("background lock was not acquired")
	}
	interactiveDone := make(chan struct{})
	go func() {
		_ = roster.WithSlotLock(context.Background(), false, func(*LockedSlot) error { close(interactiveDone); return nil })
	}()
	deadline := time.Now().Add(time.Second)
	for !slot.Contended() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !slot.Contended() {
		t.Fatal("background holder did not see interactive waiter")
	}
	close(release)
	select {
	case <-interactiveDone:
	case <-time.After(time.Second):
		t.Fatal("interactive waiter did not acquire the slot after release")
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestLockedSlotWriteCurrentVerifiesReadBack(t *testing.T) {
	dir := t.TempDir()
	kc := corruptKeychain{fakeKeychain: fakeKeychain{
		key(liveSlotService, liveSlotAccount): encodedLogin(t, "active@example.com"),
	}}
	roster := NewRosterWithLockPath(Paths{}, kc, filepath.Join(dir, "slot.lock"))
	err := roster.WithSlotLock(context.Background(), false, func(slot *LockedSlot) error {
		return slot.WriteCurrent(context.Background(), encodedLogin(t, "next@example.com"))
	})
	if err == nil {
		t.Fatal("WriteCurrent should reject a mismatched read-back")
	}
}

type corruptKeychain struct{ fakeKeychain }

func (c corruptKeychain) Write(ctx context.Context, service, account, secret string) error {
	if service == liveSlotService && account == liveSlotAccount {
		return c.fakeKeychain.Write(ctx, service, account, encodedLoginFor("wrong@example.com"))
	}
	return c.fakeKeychain.Write(ctx, service, account, secret)
}

func encodedLoginFor(email string) string {
	payload, _ := json.Marshal(map[string]string{"id_token_email": email})
	return "go-keyring-base64:" + base64.StdEncoding.EncodeToString(payload)
}

func TestSwitchReportsMissingBackup(t *testing.T) {
	dir := t.TempDir()
	paths := Paths{AccountsFile: filepath.Join(dir, "agy-accounts.txt"), ManualOnlyFile: filepath.Join(dir, "agy-manual-only.txt")}
	if err := os.WriteFile(paths.AccountsFile, []byte("next@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	roster := NewRosterWithLockPath(paths, fakeKeychain{}, filepath.Join(dir, "slot.lock"))
	if err := roster.Switch(context.Background(), "next@example.com"); !errors.Is(err, ErrNoSavedLogin) {
		t.Fatalf("Switch error = %v, want ErrNoSavedLogin", err)
	}
}
