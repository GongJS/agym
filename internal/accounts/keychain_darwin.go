//go:build darwin

package accounts

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

const (
	securityBinary = "/usr/bin/security"
	// secErrItemNotFound is the exit code returned by /usr/bin/security when a keychain item does not exist (errSecItemNotFound).
	secErrItemNotFound = 44
)

var safeKeychainText = regexp.MustCompile(`^[A-Za-z0-9@+/=:._-]+$`)

func isKeychainNotFound(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == secErrItemNotFound
}

// NewSystemKeychain returns the platform Keychain implementation.
func NewSystemKeychain() Keychain {
	return macKeychain{}
}

// macKeychain uses macOS's security CLI. Writes are sent over stdin (security
// -i), never as command-line arguments, and are read back for verification.
type macKeychain struct{}

func (macKeychain) Read(ctx context.Context, service, account string) (string, error) {
	cmd := exec.CommandContext(ctx, securityBinary, "find-generic-password", "-s", service, "-a", account, "-w")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	if isKeychainNotFound(err) {
		return "", ErrNotFound
	}
	return "", &keychainError{op: "read", err: err}
}

func (macKeychain) Write(ctx context.Context, service, account, secret string) error {
	if !safeKeychainText.MatchString(service) || !safeKeychainText.MatchString(account) || !safeKeychainText.MatchString(secret) {
		return errors.New("saved login contains characters that cannot be written safely")
	}
	command := fmt.Sprintf("add-generic-password -s %s -a %s -w '%s' -U\n", service, account, secret)
	cmd := exec.CommandContext(ctx, securityBinary, "-i")
	cmd.Stdin = strings.NewReader(command)
	if err := cmd.Run(); err != nil {
		return &keychainError{op: "write", err: errors.New("security command failed")}
	}
	stored, err := (macKeychain{}).Read(ctx, service, account)
	if err != nil {
		return err
	}
	if stored != secret {
		return errors.New("keychain read-back verification failed")
	}
	return nil
}

func (macKeychain) Delete(ctx context.Context, service, account string) error {
	cmd := exec.CommandContext(ctx, securityBinary, "delete-generic-password", "-s", service, "-a", account)
	if err := cmd.Run(); err == nil {
		return nil
	} else if isKeychainNotFound(err) {
		return nil
	} else {
		return &keychainError{op: "delete", err: err}
	}
}
