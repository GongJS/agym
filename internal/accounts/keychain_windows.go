//go:build windows

package accounts

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
)

var (
	advapi32    = windows.NewLazySystemDLL("advapi32.dll")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

type winCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// NewSystemKeychain returns the Windows Credential Manager implementation.
func NewSystemKeychain() Keychain {
	return winKeychain{}
}

type winKeychain struct{}

func credTarget(service, account string) string {
	return service + ":" + account
}

func (winKeychain) Read(ctx context.Context, service, account string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	targetPtr, err := windows.UTF16PtrFromString(credTarget(service, account))
	if err != nil {
		return "", &keychainError{op: "read", err: err}
	}
	var credPtr *winCredential
	ret, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		uintptr(credTypeGeneric),
		0,
		uintptr(unsafe.Pointer(&credPtr)),
	)
	if ret == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return "", ErrNotFound
		}
		return "", &keychainError{op: "read", err: callErr}
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(credPtr)))

	if credPtr.CredentialBlobSize == 0 || credPtr.CredentialBlob == nil {
		return "", nil
	}
	blob := unsafe.Slice(credPtr.CredentialBlob, credPtr.CredentialBlobSize)
	return string(blob), nil
}

func (w winKeychain) Write(ctx context.Context, service, account, secret string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	targetPtr, err := windows.UTF16PtrFromString(credTarget(service, account))
	if err != nil {
		return &keychainError{op: "write", err: err}
	}
	userPtr, err := windows.UTF16PtrFromString(account)
	if err != nil {
		return &keychainError{op: "write", err: err}
	}
	secretBytes := []byte(secret)
	var blobPtr *byte
	if len(secretBytes) > 0 {
		blobPtr = &secretBytes[0]
	}
	cred := winCredential{
		Type:               credTypeGeneric,
		TargetName:         targetPtr,
		CredentialBlobSize: uint32(len(secretBytes)),
		CredentialBlob:     blobPtr,
		Persist:            credPersistLocalMachine,
		UserName:           userPtr,
	}
	ret, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&cred)), 0)
	if ret == 0 {
		return &keychainError{op: "write", err: fmt.Errorf("CredWriteW failed: %w", callErr)}
	}
	stored, err := w.Read(ctx, service, account)
	if err != nil {
		return err
	}
	if stored != secret {
		return errors.New("windows credential read-back verification failed")
	}
	return nil
}

func (winKeychain) Delete(ctx context.Context, service, account string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	targetPtr, err := windows.UTF16PtrFromString(credTarget(service, account))
	if err != nil {
		return &keychainError{op: "delete", err: err}
	}
	ret, _, callErr := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		uintptr(credTypeGeneric),
		0,
	)
	if ret == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return nil
		}
		return &keychainError{op: "delete", err: callErr}
	}
	return nil
}
