//go:build darwin || linux

package accounts

import (
	"os"
	"path/filepath"
)

// DefaultAgyBinary returns the default path to the official agy CLI on Unix systems.
func DefaultAgyBinary() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin", "agy"), nil
}

// DefaultQuotaDir returns the isolated profile directory used for agy quota and OAuth probes.
func DefaultQuotaDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "sparkle", "agy-quota"), nil
}
