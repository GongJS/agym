package accounts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const slotLockTimeoutSeconds = 300

var ErrSlotLockTimeout = errors.New("timed out waiting for the agy account slot")

// LockedSlot exposes credential operations only while Roster.WithSlotLock holds
// the same cross-process lock used by the Python implementation.
type LockedSlot struct {
	keychain Keychain
	lockPath string
}

// WithSlotLock serializes slot mutations with all agym implementations.
// Background work steps aside for queued interactive work; callers should use
// LockedSlot.Contended as a cancellation check while probing.
func (r *Roster) WithSlotLock(ctx context.Context, background bool, fn func(*LockedSlot) error) error {
	if r.keychain == nil {
		return ErrUnsupported
	}
	lockPath := r.lockPath
	if lockPath == "" {
		var err error
		lockPath, err = defaultSlotLockPath()
		if err != nil {
			return err
		}
	}
	lock, err := acquireSlotLock(ctx, lockPath, background)
	if err != nil {
		return err
	}
	defer lock.Close()
	return fn(&LockedSlot{keychain: r.keychain, lockPath: lockPath})
}

func (s *LockedSlot) CurrentRaw(ctx context.Context) (string, error) {
	return s.keychain.Read(ctx, liveSlotService, liveSlotAccount)
}

func (s *LockedSlot) BackupRaw(ctx context.Context, email string) (string, error) {
	return s.keychain.Read(ctx, backupSlotService, email)
}

// WriteCurrent replaces the live login and verifies the exact value by reading
// it back. It must only be called from a WithSlotLock callback.
func (s *LockedSlot) WriteCurrent(ctx context.Context, raw string) error {
	if err := s.keychain.Write(ctx, liveSlotService, liveSlotAccount, raw); err != nil {
		return err
	}
	stored, err := s.keychain.Read(ctx, liveSlotService, liveSlotAccount)
	if err != nil {
		return err
	}
	if stored != raw {
		return errors.New("Keychain read-back verification failed")
	}
	return nil
}

// ClearCurrent removes the live login and verifies that the slot is empty.
// It must only be used while the caller holds WithSlotLock.
func (s *LockedSlot) ClearCurrent(ctx context.Context) error {
	err := s.keychain.Delete(ctx, liveSlotService, liveSlotAccount)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	_, err = s.CurrentRaw(ctx)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("Keychain live slot was not cleared")
}

// WriteBackup replaces an account backup and verifies the exact stored blob.
// It must only be used while the caller holds WithSlotLock.
func (s *LockedSlot) WriteBackup(ctx context.Context, email, raw string) error {
	email, err := validateEmail(email)
	if err != nil {
		return err
	}
	got, err := parseLoginEmail(raw)
	if err != nil || got != email {
		return fmt.Errorf("login backup does not match account %s", email)
	}
	if err := s.keychain.Write(ctx, backupSlotService, email, raw); err != nil {
		return err
	}
	stored, err := s.BackupRaw(ctx, email)
	if err != nil {
		return err
	}
	if stored != raw {
		return errors.New("Keychain backup read-back verification failed")
	}
	return nil
}

// Activate copies a saved login into agy's live Keychain slot. It verifies the
// backup belongs to the requested account before changing the active login.
func (s *LockedSlot) Activate(ctx context.Context, email string) error {
	raw, err := s.BackupRaw(ctx, email)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("%w: %s", ErrNoSavedLogin, email)
	}
	if err != nil {
		return fmt.Errorf("read saved login for %s: %w", email, err)
	}
	got, err := parseLoginEmail(raw)
	if err != nil {
		return fmt.Errorf("saved login for %s is invalid: %w", email, err)
	}
	if got != email {
		return fmt.Errorf("saved login does not match account %s", email)
	}
	return s.WriteCurrent(ctx, raw)
}

// Contended reports whether an interactive caller is queued for the slot.
func (s *LockedSlot) Contended() bool {
	return slotLockContended(s.lockPath)
}

func defaultSlotLockPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "agy-tools", "slot.lock"), nil
}

func (r *Roster) Switch(ctx context.Context, email string) error {
	email, err := validateEmail(email)
	if err != nil {
		return err
	}
	items, err := readEmails(r.paths.AccountsFile)
	if err != nil {
		return err
	}
	if !contains(items, email) {
		return fmt.Errorf("account is not in the roster: %s", email)
	}
	return r.WithSlotLock(ctx, false, func(slot *LockedSlot) error {
		return slot.Activate(ctx, email)
	})
}
