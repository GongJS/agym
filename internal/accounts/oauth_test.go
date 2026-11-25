package accounts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseLoginMetadataKeepsRefreshTokenCoreOnly(t *testing.T) {
	data, err := json.Marshal(map[string]any{
		"id_token_email": "person@example.com",
		"auth_method":    "consumer",
		"token": map[string]any{
			"refresh_token": "refresh-secret",
			"expires_in":    3599,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseLoginMetadata(keyringPrefix + base64.RawStdEncoding.EncodeToString(data))
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "person@example.com" || got.AuthMethod != "consumer" || got.RefreshToken != "refresh-secret" {
		t.Fatalf("ParseLoginMetadata() = %#v", got)
	}
	if strings.Contains(ErrInvalidLogin.Error(), got.RefreshToken) {
		t.Fatal("private refresh token appeared in a user-facing parser error")
	}
}

func TestFindAuthorizationURLRequiresCompleteURLTerminator(t *testing.T) {
	partial := "sign in: \x1b[32mhttps://accounts.google.com/o/oauth2/auth?state=abc"
	if got := findAuthorizationURL(partial); got != "" {
		t.Fatalf("incomplete URL = %q, want no event before delimiter", got)
	}
	complete := partial + "\x1b[0m\r\ncontinue"
	if got, want := findAuthorizationURL(complete), "https://accounts.google.com/o/oauth2/auth?state=abc"; got != want {
		t.Fatalf("complete URL = %q, want %q", got, want)
	}
}

func TestRecoverOAuthSlotRestoresOnlyWhenLiveSlotIsEmpty(t *testing.T) {
	roster, _, keychain := testRoster(t)
	owner := filepath.Join(t.TempDir(), "slot-owner")
	if err := os.WriteFile(owner, []byte("active@example.com"), 0o600); err != nil {
		t.Fatal(err)
	}
	delete(keychain, key(liveSlotService, liveSlotAccount))
	restored, err := roster.RecoverOAuthSlot(context.Background(), owner)
	if err != nil || !restored {
		t.Fatalf("RecoverOAuthSlot() = (%v, %v), want restored", restored, err)
	}
	if got, err := parseLoginEmail(keychain[key(liveSlotService, liveSlotAccount)]); err != nil || got != "active@example.com" {
		t.Fatalf("restored current login = %q, %v", got, err)
	}
	if _, err := os.Stat(owner); !os.IsNotExist(err) {
		t.Fatalf("owner marker still exists: %v", err)
	}

	if err := os.WriteFile(owner, []byte("active@example.com"), 0o600); err != nil {
		t.Fatal(err)
	}
	liveBefore := encodedLogin(t, "other@example.com")
	keychain[key(liveSlotService, liveSlotAccount)] = liveBefore
	restored, err = roster.RecoverOAuthSlot(context.Background(), owner)
	if err != nil || restored {
		t.Fatalf("RecoverOAuthSlot with occupied slot = (%v, %v), want not restored", restored, err)
	}
	if keychain[key(liveSlotService, liveSlotAccount)] != liveBefore {
		t.Fatal("recovery overwrote a non-empty live slot")
	}
}

func TestWriteOwnerMarkerIsPrivateAndContainsOnlyOwnerEmail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "slot-owner")
	if err := writeOwnerMarker(path, "person@example.com"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("owner marker mode = %o, want 600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "person@example.com" {
		t.Fatalf("owner marker = %q, %v", data, err)
	}
}

func TestOAuthPTYTimeoutWithEmptyGeminiDirStopsReaderAndProcess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-agy")
	script := "#!/bin/sh\ni=0\nwhile [ $i -lt 500 ]; do printf 'working output %s\\n' \"$i\"; i=$((i+1)); done\nwhile :; do sleep 1; done\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(binary, 0o700); err != nil {
		t.Fatal(err)
	}
	roster, _, _ := testRoster(t)
	ctx := context.Background()
	started := time.Now()
	err := roster.WithSlotLock(ctx, false, func(slot *LockedSlot) error {
		if err := slot.ClearCurrent(ctx); err != nil {
			return err
		}
		adder := &OAuthAdder{Roster: roster, Binary: binary, GeminiDir: "", Timeout: 120 * time.Millisecond}
		_, runErr := adder.runPTY(ctx, slot, "", map[string]struct{}{}, nil, func(OAuthEvent) {})
		if runErr == nil || !strings.Contains(runErr.Error(), "timed out") {
			return errors.New("runPTY did not return its timeout error")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("OAuth PTY cleanup took %s", elapsed)
	}
	if _, err := os.Stat(filepath.Join(home, "Library", "Application Support", "sparkle", "agy-quota", "antigravity-cli")); err != nil {
		t.Fatalf("empty GeminiDir did not resolve to the isolated default: %v", err)
	}
}
