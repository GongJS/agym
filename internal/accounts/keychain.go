package accounts

import (
	"context"
	"errors"
	"fmt"
)

var (
	// ErrNotFound means the requested keychain item does not exist.
	ErrNotFound = errors.New("keychain item not found")
	// ErrUnsupported is returned on platforms without a Keychain backend.
	ErrUnsupported = errors.New("macOS Keychain is only available on macOS")
	// ErrNoSavedLogin means the roster entry has no saved agy login backup.
	ErrNoSavedLogin = errors.New("no saved agy login was found for this account")
)

// Keychain abstracts the secure credential store. Raw login values must never
// be returned to the frontend or included in logs.
type Keychain interface {
	Read(ctx context.Context, service, account string) (string, error)
	Write(ctx context.Context, service, account, secret string) error
	Delete(ctx context.Context, service, account string) error
}

type keychainError struct {
	op  string
	err error
}

func (e *keychainError) Error() string { return fmt.Sprintf("keychain %s failed", e.op) }
func (e *keychainError) Unwrap() error { return e.err }
