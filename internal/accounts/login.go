package accounts

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

const keyringPrefix = "go-keyring-base64:"

var ErrInvalidLogin = errors.New("saved agy login is not valid base64 JSON")

type loginToken struct {
	RefreshToken string `json:"refresh_token"`
}

type loginPayload struct {
	Email      string     `json:"id_token_email"`
	AuthMethod string     `json:"auth_method"`
	Token      loginToken `json:"token"`
}

// LoginMetadata stays inside the Go core. RefreshToken is only used to reject
// a login written back by an already-running agy session; it must never be
// serialized, logged, or returned to a Wails binding.
type LoginMetadata struct {
	Email        string
	AuthMethod   string
	RefreshToken string
}

// parseLoginEmail extracts only the account identity from agy's saved login
// blob. It never returns or logs access/refresh tokens.
func parseLoginEmail(raw string) (string, error) {
	body := strings.TrimSpace(strings.TrimPrefix(raw, keyringPrefix))
	decoded, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(body)
	}
	if err != nil {
		return "", ErrInvalidLogin
	}
	var payload loginPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return "", ErrInvalidLogin
	}
	if strings.TrimSpace(payload.Email) == "" {
		return "", errors.New("saved agy login has no account email")
	}
	return payload.Email, nil
}

// ParseLoginMetadata returns private identity fields needed by the onboarding
// flow. Callers must keep RefreshToken in memory and out of user-facing data.
func ParseLoginMetadata(raw string) (LoginMetadata, error) {
	body := strings.TrimSpace(strings.TrimPrefix(raw, keyringPrefix))
	decoded, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(body)
	}
	if err != nil {
		return LoginMetadata{}, ErrInvalidLogin
	}
	var payload loginPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return LoginMetadata{}, ErrInvalidLogin
	}
	if strings.TrimSpace(payload.Email) == "" {
		return LoginMetadata{}, errors.New("saved agy login has no account email")
	}
	return LoginMetadata{
		Email:        payload.Email,
		AuthMethod:   payload.AuthMethod,
		RefreshToken: payload.Token.RefreshToken,
	}, nil
}

// LoginEmail returns only the account identity from a saved credential blob.
// The raw blob and token fields are never returned by this helper.
func LoginEmail(raw string) (string, error) {
	return parseLoginEmail(raw)
}
