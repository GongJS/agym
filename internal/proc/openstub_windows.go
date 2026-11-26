//go:build windows

package proc

import (
	"os"
	"path/filepath"
)

// InstallBrowserOpenStub installs a Windows batch stub and returns BROWSER env
// overrides so background agy probes do not launch a real browser window.
func InstallBrowserOpenStub(directory, markerEnv string) (map[string]string, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	stub := filepath.Join(directory, "open.cmd")
	body := "@echo off\r\necho %* | findstr /I \"accounts.google.com\" >nul && type nul > \"%" + markerEnv + "%\"\r\nexit /b 0\r\n"
	if existing, err := os.ReadFile(stub); err != nil || string(existing) != body {
		if err := os.WriteFile(stub, []byte(body), 0o700); err != nil {
			return nil, err
		}
	}
	return map[string]string{"BROWSER": stub}, nil
}
