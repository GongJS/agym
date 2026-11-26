//go:build !darwin && !linux

package accounts

import (
	"os"
	"path/filepath"
)

// DefaultAgyBinary returns the default executable path to the official agy CLI on non-Unix systems.
func DefaultAgyBinary() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin", "agy.exe"), nil
}

// DefaultQuotaDir returns the isolated profile directory used for agy quota and OAuth probes.
func DefaultQuotaDir() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "sparkle", "agy-quota"), nil
}
