package accounts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const (
	oauthMarkerEnv = "AGYQ_OAUTH_MARKER"
	oauthAttempt   = 75 * time.Second
	oauthPoll      = 100 * time.Millisecond
	oauthStopGrace = 3 * time.Second
)

var (
	authURLPattern = regexp.MustCompile(`https://accounts\.google\.com/\S+`)
	ansiPattern    = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
)

// OAuth state values emitted to CLI and Wails consumers.
const (
	OAuthStateIdle      = "idle"
	OAuthStateStarting  = "starting"
	OAuthStateWaiting   = "waiting"
	OAuthStateAuthorize = "authorize"
	OAuthStateAdded     = "added"
	OAuthStateError     = "error"
	OAuthStateCancelled = "cancelled"
)

// OAuthEvent is a safe state update for CLI or Wails consumers. It never
// contains raw Keychain data, tokens, authorization codes, or PTY output.
type OAuthEvent struct {
	State     string `json:"state"`
	URL       string `json:"url,omitempty"`
	Email     string `json:"email,omitempty"`
	Message   string `json:"message,omitempty"`
	ExpiresIn int    `json:"expiresIn,omitempty"`
}

// OAuthCommand is sent by a UI/CLI while Run is waiting for a Google sign-in.
type OAuthCommand struct {
	Code   string
	Cancel bool
}

// OAuthAdded is the safe result of a completed login.
type OAuthAdded struct {
	Email      string
	AuthMethod string
	Added      bool
}

// OAuthAdder runs the official agy OAuth flow in a short-lived PTY. The PTY is
// an input/output bridge for the one-time sign-in only; it is not a terminal UI.
type OAuthAdder struct {
	Roster    *Roster
	Binary    string
	GeminiDir string
	OwnerFile string
	Timeout   time.Duration
}

func NewOAuthAdder(roster *Roster) *OAuthAdder {
	home, _ := os.UserHomeDir()
	return &OAuthAdder{
		Roster:    roster,
		Binary:    filepath.Join(home, ".local", "bin", "agy"),
		GeminiDir: filepath.Join(home, "Library", "Application Support", "sparkle", "agy-quota"),
		OwnerFile: filepath.Join(home, ".cache", "agy-tools", "slot-owner"),
		Timeout:   oauthAttempt,
	}
}

// Run performs one OAuth attempt. The lock is held from the first slot read
// through identity validation and restoration so refresh/switch cannot race.
func (a *OAuthAdder) Run(ctx context.Context, commands <-chan OAuthCommand, emit func(OAuthEvent)) (result OAuthAdded, err error) {
	if a == nil || a.Roster == nil {
		return OAuthAdded{}, errors.New("OAuth account service is unavailable")
	}
	if emit == nil {
		emit = func(OAuthEvent) {}
	}
	if a.OwnerFile == "" {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			a.OwnerFile = filepath.Join(home, ".cache", "agy-tools", "slot-owner")
		}
	}
	if a.Timeout <= 0 {
		a.Timeout = oauthAttempt
	}
	if _, recoverErr := a.Roster.RecoverOAuthSlot(ctx, a.OwnerFile); recoverErr != nil {
		return OAuthAdded{}, fmt.Errorf("recover interrupted account sign-in: %w", recoverErr)
	}
	emit(OAuthEvent{State: OAuthStateWaiting, Message: "Waiting for account slot lock"})
	err = a.Roster.WithSlotLock(ctx, false, func(slot *LockedSlot) (callbackErr error) {
		original, readErr := slot.CurrentRaw(ctx)
		if errors.Is(readErr, ErrNotFound) {
			original = ""
		} else if readErr != nil {
			return fmt.Errorf("read current agy login: %w", readErr)
		}

		known, knownErr := knownRefreshTokens(ctx, slot, original, a.Roster.paths.AccountsFile)
		if knownErr != nil {
			return knownErr
		}
		ownerEmail := ""
		if original != "" {
			metadata, parseErr := ParseLoginMetadata(original)
			if parseErr != nil {
				return fmt.Errorf("current agy login is invalid: %w", parseErr)
			}
			ownerEmail = metadata.Email
			if err := ensureOriginalBackup(ctx, slot, ownerEmail, original); err != nil {
				return err
			}
			if err := writeOwnerMarker(a.OwnerFile, ownerEmail); err != nil {
				return fmt.Errorf("write account recovery marker: %w", err)
			}
		}

		defer func() {
			restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			current, currentErr := slot.CurrentRaw(restoreCtx)
			if errors.Is(currentErr, ErrNotFound) {
				current = ""
			} else if currentErr != nil {
				callbackErr = errors.Join(callbackErr, fmt.Errorf("read slot before restore: %w", currentErr))
				return
			}
			if original != "" && current != original {
				if restoreErr := slot.WriteCurrent(restoreCtx, original); restoreErr != nil {
					callbackErr = errors.Join(callbackErr, fmt.Errorf("restore original agy login: %w", restoreErr))
					return
				}
			}
			if ownerEmail != "" {
				if removeErr := os.Remove(a.OwnerFile); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
					callbackErr = errors.Join(callbackErr, fmt.Errorf("remove account recovery marker: %w", removeErr))
				}
			}
		}()

		if err := slot.ClearCurrent(ctx); err != nil {
			return fmt.Errorf("clear live agy login: %w", err)
		}
		result, callbackErr = a.runPTY(ctx, slot, original, known, commands, emit)
		return callbackErr
	})
	if err != nil {
		emit(OAuthEvent{State: "error", Message: safeOAuthError(err)})
		return OAuthAdded{}, err
	}
	emit(OAuthEvent{State: OAuthStateAdded, Email: result.Email, Message: "Account safely saved and added"})
	return result, nil
}

func (a *OAuthAdder) runPTY(ctx context.Context, slot *LockedSlot, original string, known map[string]struct{}, commands <-chan OAuthCommand, emit func(OAuthEvent)) (OAuthAdded, error) {
	geminiDir := strings.TrimSpace(a.GeminiDir)
	if geminiDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return OAuthAdded{}, fmt.Errorf("resolve agy sign-in profile: %w", err)
		}
		geminiDir = filepath.Join(home, "Library", "Application Support", "sparkle", "agy-quota")
	}
	if err := prepareOAuthDirs(geminiDir); err != nil {
		return OAuthAdded{}, fmt.Errorf("prepare isolated agy sign-in profile: %w", err)
	}
	stubDir, err := os.MkdirTemp("", "agy-add-open-")
	if err != nil {
		return OAuthAdded{}, fmt.Errorf("prepare browser handoff: %w", err)
	}
	defer os.RemoveAll(stubDir)
	marker := filepath.Join(stubDir, "browser-requested")
	if err := installOAuthOpenStub(stubDir); err != nil {
		return OAuthAdded{}, fmt.Errorf("prepare browser handoff: %w", err)
	}
	binary := a.Binary
	if binary == "" {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			binary = filepath.Join(home, ".local", "bin", "agy")
		}
	}
	if binary == "" {
		return OAuthAdded{}, errors.New("official agy CLI path is unavailable")
	}
	cmd := exec.Command(binary, "--gemini_dir", geminiDir, "-p", "/usage")
	cmd.Env = replaceOAuthEnv(os.Environ(), map[string]string{
		"TERM":         "xterm-256color",
		"PATH":         stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		oauthMarkerEnv: marker,
	})
	terminal, err := pty.Start(cmd)
	if err != nil {
		return OAuthAdded{}, fmt.Errorf("start official agy sign-in: %w", err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	reads := make(chan []byte, 4)
	readDone := make(chan struct{})
	defer close(readDone)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := terminal.Read(buf)
			if n > 0 {
				copyOfChunk := append([]byte(nil), buf[:n]...)
				select {
				case reads <- copyOfChunk:
				case <-readDone:
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()
	defer terminal.Close()
	defer stopOAuthProcess(cmd, wait)

	deadline := time.NewTimer(a.Timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(oauthPoll)
	defer ticker.Stop()
	var printed strings.Builder
	urlSent := false
	processEnded := false
	var processErr error
	var endedAt time.Time

	for {
		if current, readErr := slot.CurrentRaw(ctx); readErr == nil && current != "" && current != original {
			metadata, parseErr := ParseLoginMetadata(current)
			if parseErr != nil {
				return OAuthAdded{}, errors.New("agy wrote an unrecognized login; account was not added")
			}
			if metadata.Email == "unknown" {
				return OAuthAdded{}, errors.New("agy did not report the signed-in email")
			}
			if metadata.RefreshToken != "" {
				if _, duplicate := known[metadata.RefreshToken]; duplicate {
					return OAuthAdded{}, errors.New("sign-in was interrupted by an already-running agy session; retry after it finishes")
				}
			}
			if err := slot.WriteBackup(ctx, metadata.Email, current); err != nil {
				return OAuthAdded{}, fmt.Errorf("save verified login backup: %w", err)
			}
			added, addErr := a.Roster.Add(ctx, metadata.Email)
			if addErr != nil {
				return OAuthAdded{}, fmt.Errorf("add account to roster: %w", addErr)
			}
			return OAuthAdded{Email: metadata.Email, AuthMethod: metadata.AuthMethod, Added: added}, nil
		} else if readErr != nil && !errors.Is(readErr, ErrNotFound) {
			return OAuthAdded{}, fmt.Errorf("read agy login after sign-in: %w", readErr)
		}

		if processEnded && time.Since(endedAt) >= 500*time.Millisecond {
			if processErr != nil {
				return OAuthAdded{}, errors.New("official agy sign-in exited before the account was saved")
			}
			return OAuthAdded{}, errors.New("authorization did not finish before agy exited")
		}
		select {
		case <-ctx.Done():
			return OAuthAdded{}, errors.New("sign-in cancelled")
		case <-deadline.C:
			return OAuthAdded{}, errors.New("sign-in timed out; please start again")
		case err := <-wait:
			processEnded, processErr, endedAt = true, err, time.Now()
			wait = nil
		case chunk := <-reads:
			if printed.Len() < 64*1024 {
				remaining := 64*1024 - printed.Len()
				if len(chunk) > remaining {
					chunk = chunk[:remaining]
				}
				printed.Write(chunk)
			}
			if !urlSent {
				if match := findAuthorizationURL(printed.String()); match != "" {
					urlSent = true
					expires := int(a.Timeout.Round(time.Second).Seconds())
					if expires > 60 {
						expires = 60
					}
					emit(OAuthEvent{State: OAuthStateAuthorize, URL: match, ExpiresIn: expires, Message: "Open Google sign-in link and return the authorization code"})
				}
			}
		case command, ok := <-commands:
			if !ok || command.Cancel {
				return OAuthAdded{}, errors.New("sign-in cancelled")
			}
			code := strings.TrimSpace(command.Code)
			if code == "" || len(code) > 2048 || strings.ContainsAny(code, "\r\n") || hasControl(code) {
				continue
			}
			if err := writeAll(terminal, []byte(code+"\r")); err != nil {
				return OAuthAdded{}, errors.New("could not submit authorization code")
			}
		case <-ticker.C:
		}
	}
}

func knownRefreshTokens(ctx context.Context, slot *LockedSlot, current, rosterPath string) (map[string]struct{}, error) {
	known := make(map[string]struct{})
	add := func(raw string) {
		if raw == "" {
			return
		}
		if metadata, err := ParseLoginMetadata(raw); err == nil && metadata.RefreshToken != "" {
			known[metadata.RefreshToken] = struct{}{}
		}
	}
	add(current)
	emails, err := readEmails(rosterPath)
	if err != nil {
		return nil, err
	}
	for _, email := range emails {
		raw, readErr := slot.BackupRaw(ctx, email)
		if errors.Is(readErr, ErrNotFound) {
			continue
		}
		if readErr != nil {
			return nil, fmt.Errorf("read saved login for %s: %w", email, readErr)
		}
		add(raw)
	}
	return known, nil
}

func ensureOriginalBackup(ctx context.Context, slot *LockedSlot, email, raw string) error {
	backup, err := slot.BackupRaw(ctx, email)
	if err == nil {
		backupEmail, parseErr := parseLoginEmail(backup)
		if parseErr != nil || backupEmail != email {
			return fmt.Errorf("existing login backup does not match current account %s", email)
		}
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("check current account backup: %w", err)
	}
	if err := slot.WriteBackup(ctx, email, raw); err != nil {
		return fmt.Errorf("save current account for crash recovery: %w", err)
	}
	return nil
}

// RecoverOAuthSlot restores the previous login when a process died after
// clearing the live slot. If a new login is already present it is left intact.
func (r *Roster) RecoverOAuthSlot(ctx context.Context, ownerFile string) (bool, error) {
	if ownerFile == "" {
		return false, nil
	}
	data, err := os.ReadFile(ownerFile)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	email, err := validateEmail(string(data))
	if err != nil {
		return false, errors.New("account recovery marker is invalid")
	}
	restored := false
	err = r.WithSlotLock(ctx, false, func(slot *LockedSlot) error {
		_, readErr := slot.CurrentRaw(ctx)
		if errors.Is(readErr, ErrNotFound) {
			if activateErr := slot.Activate(ctx, email); activateErr != nil {
				return activateErr
			}
			restored = true
			return nil
		}
		if readErr != nil {
			return readErr
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if err := os.Remove(ownerFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return restored, err
	}
	return restored, nil
}

func writeOwnerMarker(path, email string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".slot-owner-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.WriteString(tmp, email); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func prepareOAuthDirs(geminiDir string) error {
	if geminiDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		geminiDir = filepath.Join(home, "Library", "Application Support", "sparkle", "agy-quota")
	}
	profile := filepath.Join(geminiDir, "antigravity-cli")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	installID := filepath.Join(profile, "installation_id")
	if _, err := os.Stat(installID); errors.Is(err, os.ErrNotExist) {
		data, readErr := os.ReadFile(filepath.Join(home, ".gemini", "antigravity-cli", "installation_id"))
		if readErr == nil {
			_ = os.WriteFile(installID, data, 0o600)
		}
	}
	return nil
}

func installOAuthOpenStub(directory string) error {
	body := "#!/bin/sh\ncase \"$*\" in\n  *accounts.google.com*) : > \"${" + oauthMarkerEnv + "}\" ;;\nesac\nexit 0\n"
	path := filepath.Join(directory, "open")
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func replaceOAuthEnv(environment []string, values map[string]string) []string {
	result := make([]string, 0, len(environment)+len(values))
	for _, item := range environment {
		name, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, replace := values[name]; replace {
			continue
		}
		result = append(result, item)
	}
	for name, value := range values {
		result = append(result, name+"="+value)
	}
	return result
}

func stopOAuthProcess(cmd *exec.Cmd, wait chan error) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	if wait != nil {
		select {
		case <-wait:
			return
		case <-time.After(oauthStopGrace):
		}
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if wait != nil {
		select {
		case <-wait:
		case <-time.After(2 * time.Second):
		}
	}
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func hasControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func findAuthorizationURL(output string) string {
	output = ansiPattern.ReplaceAllString(output, "")
	for _, bounds := range authURLPattern.FindAllStringIndex(output, -1) {
		if bounds[1] >= len(output) {
			continue
		}
		if output[bounds[1]] == '\r' || output[bounds[1]] == '\n' || output[bounds[1]] == '\t' || output[bounds[1]] == ' ' {
			return output[bounds[0]:bounds[1]]
		}
	}
	return ""
}

func safeOAuthError(err error) string {
	message := err.Error()
	if strings.Contains(strings.ToLower(message), "token") || strings.Contains(message, "base64") {
		return "账号登录未完成，请重试"
	}
	return message
}
