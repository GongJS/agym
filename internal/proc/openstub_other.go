//go:build !darwin && !linux && !windows

package proc

// InstallBrowserOpenStub is a no-op stub on unsupported platforms.
func InstallBrowserOpenStub(_, _ string) (map[string]string, error) {
	return nil, nil
}
