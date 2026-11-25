package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelpCommands(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("Run(%v) = %d, stderr=%q", args, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "agym") || !strings.Contains(stdout.String(), "tray") {
			t.Fatalf("help output missing agym/tray usage: %q", stdout.String())
		}
	}
}

func TestRunCompletionCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"completion", "zsh"}, &stdout, &stderr); code != 0 {
		t.Fatalf("completion zsh = %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "#compdef agym") {
		t.Fatalf("zsh completion missing #compdef agym: %q", stdout.String())
	}
}

func TestRunArgumentValidationReturnsExitCodeTwo(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"unknown-cmd"}, "unknown command"},
		{[]string{"switch"}, "switch requires an email address"},
		{[]string{"add"}, "add requires an email address"},
		{[]string{"remove"}, "remove requires an email address"},
		{[]string{"rotation", "user@example.com"}, "rotation requires an email and on|off"},
		{[]string{"rotation", "user@example.com", "maybe"}, "rotation must be on or off"},
		{[]string{"quota", "--invalid"}, "unknown flag"},
		{[]string{"quota", "extra"}, "quota accepts only --refresh"},
	}
	for _, tc := range cases {
		var stdout, stderr bytes.Buffer
		code := Run(tc.args, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("Run(%v) = %d, want 2 (stderr=%q)", tc.args, code, stderr.String())
		}
		if !strings.Contains(stderr.String(), tc.want) {
			t.Fatalf("Run(%v) stderr = %q, want substring %q", tc.args, stderr.String(), tc.want)
		}
	}
}
