package usage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gongjs/agym/internal/cloudmeta"
	"github.com/gongjs/agym/internal/envutil"
)

const (
	usageTimeout   = 25 * time.Second
	oauthMarkerEnv = "AGYQ_OAUTH_MARKER"
)

// ProbeResult is the bounded output of one official agy query. Outcome is one
// of exited, oauth_prompt, timeout, yielded, or error.
type ProbeResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Outcome  string
	Meta     *cloudmeta.CloudMeta
}

// Runner abstracts the official agy subprocess for deterministic core tests.
type Runner interface {
	QueryUsage(context.Context, func() bool) ProbeResult
}

// ModelRunner is implemented by runners that can query account-specific model
// availability while the same Keychain slot lock is held.
type ModelRunner interface {
	QueryModels(context.Context, func() bool) ProbeResult
}

// AgyRunner runs the installed official agy binary in the private quota profile.
type AgyRunner struct {
	Binary       string
	Home         string
	CloudCodeURL string
}

func (r AgyRunner) QueryUsage(ctx context.Context, shouldYield func() bool) ProbeResult {
	return r.runQuery(ctx, shouldYield, []string{"-p", "/usage", "--output-format", "json", "--print-timeout", "25s"}, usageTimeout)
}

func (r AgyRunner) QueryModels(ctx context.Context, shouldYield func() bool) ProbeResult {
	return r.runQuery(ctx, shouldYield, []string{"models"}, 30*time.Second)
}

func (r AgyRunner) runQuery(ctx context.Context, shouldYield func() bool, queryArgs []string, timeout time.Duration) (result ProbeResult) {
	home := r.Home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return ProbeResult{ExitCode: -1, Stderr: err.Error(), Outcome: "error"}
		}
	}
	binary := r.Binary
	if binary == "" {
		binary = filepath.Join(home, ".local", "bin", "agy")
	}
	quotaDir := filepath.Join(home, "Library", "Application Support", "sparkle", "agy-quota")
	if err := prepareProbeDirs(home, quotaDir); err != nil {
		return ProbeResult{ExitCode: -1, Stderr: err.Error(), Outcome: "error"}
	}
	stubDir := filepath.Join(home, ".cache", "agy-tools", "probe-bin")
	if err := installOpenStub(stubDir); err != nil {
		return ProbeResult{ExitCode: -1, Stderr: err.Error(), Outcome: "error"}
	}
	relay, relayErr := cloudmeta.Start(r.CloudCodeURL)
	if relayErr == nil {
		defer func() {
			result.Meta = nonEmptyCloudMeta(relay.Meta())
			_ = relay.Close()
		}()
	}
	scratch, err := os.MkdirTemp("", "agyq-")
	if err != nil {
		return ProbeResult{ExitCode: -1, Stderr: err.Error(), Outcome: "error"}
	}
	defer os.RemoveAll(scratch)
	marker := filepath.Join(scratch, "oauth-prompt")
	args := append([]string{"--gemini_dir", quotaDir}, queryArgs...)
	cmd := exec.Command(binary, args...)
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	envValues := map[string]string{
		"TERM":         "dumb",
		"PATH":         stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		oauthMarkerEnv: marker,
	}
	if relay != nil {
		envValues["CLOUD_CODE_URL"] = relay.URL
	}
	cmd.Env = envutil.Replace(os.Environ(), envValues)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		return ProbeResult{ExitCode: -1, Stderr: err.Error(), Outcome: "error"}
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-wait:
			if _, statErr := os.Stat(marker); statErr == nil {
				return ProbeResult{ExitCode: -1, Stderr: "login refresh failed; agy attempted to open a browser", Outcome: "oauth_prompt"}
			}
			code := 0
			if err != nil {
				code = processExitCode(err)
			}
			return ProbeResult{ExitCode: code, Stdout: stdout.String(), Stderr: stderr.String(), Outcome: "exited"}
		case <-ctx.Done():
			killProcessGroup(cmd)
			<-wait
			return ProbeResult{ExitCode: -1, Stderr: ctx.Err().Error(), Outcome: "error"}
		case <-deadline.C:
			killProcessGroup(cmd)
			<-wait
			return ProbeResult{ExitCode: -1, Stderr: "agy query timed out", Outcome: "timeout"}
		case <-ticker.C:
			if shouldYield != nil && shouldYield() {
				killProcessGroup(cmd)
				<-wait
				return ProbeResult{ExitCode: -1, Stderr: "yielded to an interactive account operation", Outcome: "yielded"}
			}
		}
	}
}

func nonEmptyCloudMeta(meta cloudmeta.CloudMeta) *cloudmeta.CloudMeta {
	if meta.TierID == "" && meta.TierName == "" && meta.RegionCode == "" && meta.DefaultModel == "" && len(meta.ModelTags) == 0 {
		return nil
	}
	return &meta
}

func prepareProbeDirs(home, quotaDir string) error {
	profile := filepath.Join(quotaDir, "antigravity-cli")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return err
	}
	target := filepath.Join(profile, "installation_id")
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		source := filepath.Join(home, ".gemini", "antigravity-cli", "installation_id")
		if data, readErr := os.ReadFile(source); readErr == nil {
			_ = os.WriteFile(target, data, 0o600)
		}
	}
	return nil
}

func installOpenStub(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	stub := filepath.Join(directory, "open")
	body := "#!/bin/sh\ncase \"$*\" in\n  *accounts.google.com*) : > \"${" + oauthMarkerEnv + "}\" ;;\nesac\nexit 0\n"
	if existing, err := os.ReadFile(stub); err == nil && string(existing) == body {
		if info, statErr := os.Stat(stub); statErr == nil && info.Mode()&0o111 != 0 {
			return nil
		}
	}
	tmpFile, err := os.CreateTemp(directory, ".open-*")
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	if err := tmpFile.Chmod(0o700); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return err
	}
	if _, err := tmpFile.WriteString(body); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, stub); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func killProcessGroup(command *exec.Cmd) {
	if command.Process == nil {
		return
	}
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
		_ = command.Process.Kill()
	}
}

func processExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func (r ProbeResult) String() string {
	return fmt.Sprintf("%s (%d)", r.Outcome, r.ExitCode)
}
