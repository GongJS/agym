//go:build darwin || linux

package proc

import (
	"os"
	"path/filepath"
)

// InstallBrowserOpenStub installs an `open` shell stub in directory so background
// agy probes cannot pop up an interactive browser window.
func InstallBrowserOpenStub(directory, markerEnv string) (map[string]string, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	stub := filepath.Join(directory, "open")
	body := "#!/bin/sh\ncase \"$*\" in\n  *accounts.google.com*) : > \"${" + markerEnv + "}\" ;;\nesac\nexit 0\n"
	if existing, err := os.ReadFile(stub); err == nil && string(existing) == body {
		if info, statErr := os.Stat(stub); statErr == nil && info.Mode()&0o111 != 0 {
			return nil, nil
		}
	}
	tmpFile, err := os.CreateTemp(directory, ".open-*")
	if err != nil {
		return nil, err
	}
	tmp := tmpFile.Name()
	if err := tmpFile.Chmod(0o700); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return nil, err
	}
	if _, err := tmpFile.WriteString(body); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return nil, err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, stub); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	return nil, nil
}
