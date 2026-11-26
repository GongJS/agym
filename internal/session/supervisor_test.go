package session

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongjs/agym/internal/proc"
)

func TestRunSessionHookStoresOnlySafeStopFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events", "events.jsonl")
	t.Setenv("AGY_TOOLS_SESSION_EVENTS", path)
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"hookEventName":"Stop","conversationId":"conv-1","modelName":"gemini-3-pro","terminationReason":"ERROR","error":"RESOURCE_EXHAUSTED (code 429): Individual quota reached","lastUserInput":"private user prompt","transcriptPath":"/private/transcript.jsonl","fullyIdle":true}`
	if _, err := io.WriteString(writer, payload); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	oldStdin := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = reader.Close()
	})
	var stdout, stderr bytes.Buffer
	if code := runSessionHook(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("runSessionHook() = %d, stderr=%q", code, stderr.String())
	}
	if got := stdout.String(); got != "{}\n" {
		t.Fatalf("hook stdout = %q, want {}", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private user prompt") || strings.Contains(string(data), "transcriptPath") {
		t.Fatalf("hook persisted private payload fields: %s", data)
	}
	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &record); err != nil {
		t.Fatal(err)
	}
	if record["conversationId"] != "conv-1" || record["hookEventName"] != "Stop" {
		t.Fatalf("safe event = %#v", record)
	}
}

func TestStopProcessTreeTerminatesChildAndHelpers(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30 & wait")
	proc.ConfigureDetached(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	start := time.Now()
	var stderr bytes.Buffer
	if err := stopProcessTree(cmd, wait, &stderr); err != nil {
		t.Fatalf("stopProcessTree() = %v, stderr=%q", err, stderr.String())
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("process tree took %s to stop", elapsed)
	}
	if cmd.ProcessState == nil {
		t.Fatal("root process has not been reaped")
	}
}
