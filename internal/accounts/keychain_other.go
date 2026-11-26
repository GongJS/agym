//go:build !darwin && !windows

package accounts

import "context"

// NewSystemKeychain returns an unsupported stub on non-macOS platforms.
func NewSystemKeychain() Keychain {
	return unsupportedKeychain{}
}

// unsupportedKeychain is a platform stub so pure core tests and CLI builds can
// run on Linux and Windows without pretending that credentials are available there.
type unsupportedKeychain struct{}

func (unsupportedKeychain) Read(context.Context, string, string) (string, error) {
	return "", ErrUnsupported
}

func (unsupportedKeychain) Write(context.Context, string, string, string) error {
	return ErrUnsupported
}

func (unsupportedKeychain) Delete(context.Context, string, string) error {
	return ErrUnsupported
}
