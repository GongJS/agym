//go:build darwin || linux

package accounts

import (
	"os"
	"path/filepath"
)

func defaultBaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "sparkle"), nil
}

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
	base, err := defaultBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "agy-quota"), nil
}
