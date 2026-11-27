package accounts

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
)

// Account is safe to return to a UI: it contains no login tokens or Keychain data.
type Account struct {
	Email         string `json:"email"`
	Current       bool   `json:"current"`
	InRoster      bool   `json:"inRoster"`
	Rotation      bool   `json:"rotation"`
	HasSavedLogin bool   `json:"hasSavedLogin"`
}

// Roster manages the existing newline-delimited account and manual-only files.
type Roster struct {
	paths    Paths
	keychain Keychain
	lockPath string
}

func NewRoster(paths Paths, keychain Keychain) *Roster {
	lockPath, _ := defaultSlotLockPath()
	return NewRosterWithLockPath(paths, keychain, lockPath)
}

// NewDefaultRoster creates the shared account store using the established
// on-disk paths and the platform Keychain backend.
func NewDefaultRoster() (*Roster, error) {
	paths, err := DefaultPaths()
	if err != nil {
		return nil, err
	}
	return NewRoster(paths, NewSystemKeychain()), nil
}

// CurrentEmail returns the identity stored in agy's live Keychain slot. An
// empty result means no login is active in that slot.
func (r *Roster) CurrentEmail(ctx context.Context) (string, error) {
	if r.keychain == nil {
		return "", ErrUnsupported
	}
	raw, err := r.keychain.Read(ctx, liveSlotService, liveSlotAccount)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return parseLoginEmail(raw)
}

// NewRosterWithLockPath creates a roster with an explicit lock path. It is
// useful for isolated tests; production should use NewRoster.
func NewRosterWithLockPath(paths Paths, keychain Keychain, lockPath string) *Roster {
	return &Roster{paths: paths, keychain: keychain, lockPath: lockPath}
}

func (r *Roster) List(ctx context.Context) ([]Account, error) {
	emails, err := readEmails(r.paths.AccountsFile)
	if err != nil {
		return nil, err
	}
	manual, err := readEmails(r.paths.ManualOnlyFile)
	if err != nil {
		return nil, err
	}
	manualOnly := make(map[string]struct{}, len(manual))
	for _, email := range manual {
		manualOnly[email] = struct{}{}
	}

	current := ""
	if r.keychain != nil {
		raw, readErr := r.keychain.Read(ctx, liveSlotService, liveSlotAccount)
		switch {
		case errors.Is(readErr, ErrNotFound), errors.Is(readErr, ErrUnsupported):
		case readErr != nil:
			return nil, fmt.Errorf("read current agy account: %w", readErr)
		default:
			current, _ = parseLoginEmail(raw)
		}
	}

	accounts := make([]Account, 0, len(emails))
	for _, email := range emails {
		_, manualOnlyAccount := manualOnly[email]
		item := Account{Email: email, Current: email == current, InRoster: true, Rotation: !manualOnlyAccount}
		if r.keychain != nil {
			_, loginErr := r.keychain.Read(ctx, backupSlotService, email)
			switch {
			case loginErr == nil:
				item.HasSavedLogin = true
			case errors.Is(loginErr, ErrNotFound), errors.Is(loginErr, ErrUnsupported):
			default:
				return nil, fmt.Errorf("check saved login for %s: %w", email, loginErr)
			}
		}
		accounts = append(accounts, item)
	}
	if current != "" && !contains(emails, current) {
		item := Account{Email: current, Current: true}
		if r.keychain != nil {
			_, loginErr := r.keychain.Read(ctx, backupSlotService, current)
			switch {
			case loginErr == nil:
				item.HasSavedLogin = true
			case errors.Is(loginErr, ErrNotFound), errors.Is(loginErr, ErrUnsupported):
			default:
				return nil, fmt.Errorf("check saved login for %s: %w", current, loginErr)
			}
		}
		accounts = append(accounts, item)
	}
	return accounts, nil
}

// Add adds an already-authenticated account whose agy credential backup exists.
// OAuth sign-in is intentionally deferred; adding an email alone must not imply
// that a usable login is configured.
func (r *Roster) Add(ctx context.Context, email string) (bool, error) {
	email, err := validateEmail(email)
	if err != nil {
		return false, err
	}
	if r.keychain == nil {
		return false, ErrUnsupported
	}
	if _, err := r.keychain.Read(ctx, backupSlotService, email); err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, fmt.Errorf("%w: %s", ErrNoSavedLogin, email)
		}
		return false, fmt.Errorf("check saved login for %s: %w", email, err)
	}
	var added bool
	err = r.withRosterLock(ctx, func() error {
		accounts, err := readEmails(r.paths.AccountsFile)
		if err != nil {
			return err
		}
		for _, existing := range accounts {
			if existing == email {
				return nil
			}
		}
		if err := writeEmails(r.paths.AccountsFile, append(accounts, email)); err != nil {
			return err
		}
		added = true
		return nil
	})
	return added, err
}

// AddCurrentAccount safely saves the active login as a backup and adds it to
// the roster. It refuses if the live slot no longer belongs to the requested
// account; existing backups are never overwritten.
func (r *Roster) AddCurrentAccount(ctx context.Context, email string) (bool, error) {
	email, err := validateEmail(email)
	if err != nil {
		return false, err
	}
	added := false
	err = r.WithSlotLock(ctx, false, func(slot *LockedSlot) error {
		raw, readErr := slot.CurrentRaw(ctx)
		if readErr != nil {
			return fmt.Errorf("read current agy login: %w", readErr)
		}
		current, parseErr := parseLoginEmail(raw)
		if parseErr != nil || current != email {
			return fmt.Errorf("current agy login does not match %s", email)
		}
		backup, backupErr := slot.BackupRaw(ctx, email)
		switch {
		case backupErr == nil:
			backupEmail, parseBackupErr := parseLoginEmail(backup)
			if parseBackupErr != nil || backupEmail != email {
				return fmt.Errorf("saved login backup does not match account %s", email)
			}
		case errors.Is(backupErr, ErrNotFound):
			if err := r.keychain.Write(ctx, backupSlotService, email, raw); err != nil {
				return fmt.Errorf("save current login for %s: %w", email, err)
			}
			stored, verifyErr := slot.BackupRaw(ctx, email)
			if verifyErr != nil || stored != raw {
				return errors.New("saved login backup could not be verified")
			}
		default:
			return fmt.Errorf("check saved login for %s: %w", email, backupErr)
		}

		items, err := readEmails(r.paths.AccountsFile)
		if err != nil {
			return err
		}
		if contains(items, email) {
			return nil
		}
		if err := writeEmails(r.paths.AccountsFile, append(items, email)); err != nil {
			return err
		}
		added = true
		return nil
	})
	return added, err
}

// Remove removes the account from both rosters but deliberately leaves its
// Keychain backup untouched so this operation is reversible and non-destructive.
func (r *Roster) Remove(email string) error {
	email, err := validateEmail(email)
	if err != nil {
		return err
	}
	return r.withRosterLock(context.Background(), func() error {
		accounts, err := readEmails(r.paths.AccountsFile)
		if err != nil {
			return err
		}
		manual, err := readEmails(r.paths.ManualOnlyFile)
		if err != nil {
			return err
		}
		if err := writeEmails(r.paths.AccountsFile, without(accounts, email)); err != nil {
			return err
		}
		return writeEmails(r.paths.ManualOnlyFile, without(manual, email))
	})
}

// SetRotation controls whether the account may be used in automatic rotation.
func (r *Roster) SetRotation(email string, enabled bool) error {
	email, err := validateEmail(email)
	if err != nil {
		return err
	}
	return r.withRosterLock(context.Background(), func() error {
		accounts, err := readEmails(r.paths.AccountsFile)
		if err != nil {
			return err
		}
		registered := false
		for _, existing := range accounts {
			if existing == email {
				registered = true
				break
			}
		}
		if !registered {
			return fmt.Errorf("account is not in the roster: %s", email)
		}
		manual, err := readEmails(r.paths.ManualOnlyFile)
		if err != nil {
			return err
		}
		if enabled {
			manual = without(manual, email)
		} else if !contains(manual, email) {
			manual = append(manual, email)
		}
		return writeEmails(r.paths.ManualOnlyFile, manual)
	})
}

func (r *Roster) withRosterLock(ctx context.Context, fn func() error) error {
	lockPath := filepath.Join(filepath.Dir(r.paths.AccountsFile), "roster.lock")
	lock, err := acquireSlotLock(ctx, lockPath, false)
	if err != nil {
		return err
	}
	defer lock.Close()
	return fn()
}

func readEmails(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var emails []string
	seen := make(map[string]struct{})
	for _, line := range strings.Split(string(data), "\n") {
		email := strings.TrimSpace(line)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}
	return emails, nil
}

func writeEmails(path string, emails []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agy-roster-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	for _, email := range emails {
		if _, err := fmt.Fprintln(tmp, email); err != nil {
			return err
		}
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func validateEmail(value string) (string, error) {
	email := strings.TrimSpace(value)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || strings.ContainsAny(email, "\r\n") {
		return "", fmt.Errorf("invalid account email: %q", value)
	}
	return email, nil
}

func without(items []string, target string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != target {
			out = append(out, item)
		}
	}
	return out
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
