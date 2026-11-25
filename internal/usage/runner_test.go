package usage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestAgyRunnerUsesPrivateUsageProfileAndOfficialArguments(t *testing.T) {
	home := t.TempDir()
	binary := filepath.Join(t.TempDir(), "agy")
	argLog := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("AGY_ARG_LOG", argLog)
	writeExecutable(t, binary, "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$AGY_ARG_LOG\"\nprintf '%s' '"+usageReport+"'\n")
	result := (AgyRunner{Binary: binary, Home: home}).QueryUsage(context.Background(), nil)
	if result.Outcome != "exited" || result.ExitCode != 0 {
		t.Fatalf("QueryUsage() = %#v", result)
	}
	if result.Stdout != usageReport {
		t.Fatalf("stdout = %q, want usage report", result.Stdout)
	}
	args, err := os.ReadFile(argLog)
	if err != nil {
		t.Fatal(err)
	}
	joined := string(args)
	for _, expected := range []string{"--gemini_dir", filepath.Join(home, "Library", "Application Support", "sparkle", "agy-quota"), "-p", "/usage", "--output-format", "json", "--print-timeout", "25s"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("agy args missing %q: %s", expected, joined)
		}
	}
	profile := filepath.Join(home, "Library", "Application Support", "sparkle", "agy-quota", "antigravity-cli")
	info, err := os.Stat(profile)
	if err != nil || !info.IsDir() {
		t.Fatalf("private profile was not created: %v, %v", info, err)
	}
}

func TestAgyRunnerStopsBrowserOAuthFallback(t *testing.T) {
	home := t.TempDir()
	binary := filepath.Join(t.TempDir(), "agy")
	writeExecutable(t, binary, "#!/bin/sh\nopen https://accounts.google.com/signin\nexit 1\n")
	result := (AgyRunner{Binary: binary, Home: home}).QueryUsage(context.Background(), nil)
	if result.Outcome != "oauth_prompt" {
		t.Fatalf("QueryUsage() outcome = %q, want oauth_prompt: %#v", result.Outcome, result)
	}
}
