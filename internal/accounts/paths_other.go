//go:build !darwin && !linux

package accounts

import (
	"os"
	"os/exec"
	"path/filepath"
)

func defaultBaseDir() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "sparkle"), nil
}

// DefaultAgyBinary resolves the official agy executable on non-Unix systems.
func DefaultAgyBinary() (string, error) {
	if resolved, err := exec.LookPath("agy"); err == nil {
		return resolved, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin", "agy.exe"), nil
}

// DefaultQuotaDir returns the isolated profile directory used for agy quota and OAuth probes.
func DefaultQuotaDir() (string, error) {
	base, err := defaultBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "agy-quota"), nil
}
