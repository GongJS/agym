package projects

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListProjectsAggregatesTranscriptAndPresence(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "repo-alpha")
	convID := "conv-123"

	if err := os.MkdirAll(filepath.Join(dir, "presence"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "presence", convID+".lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	historyLine := `{"workspace":"` + filepath.ToSlash(ws) + `","conversationId":"` + convID + `","timestamp":1700000000000}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(historyLine), 0o600); err != nil {
		t.Fatal(err)
	}

	logDir := filepath.Join(dir, "brain", convID, ".system_generated", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	transcript := "" +
		`{"step_index":0,"type":"PLANNER_RESPONSE","status":"DONE","model":"claude-sonnet-4-6","input_tokens":10000,"cache_read_tokens":2000,"output_tokens":300,"tool_calls":[{"name":"write_file","arguments":{"Path":"` + filepath.ToSlash(filepath.Join(ws, "main.go")) + `"}}]}` + "\n" +
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"RUNNING","model":"claude-sonnet-4-6","input_tokens":15000,"cache_read_tokens":8000,"output_tokens":500,"tool_calls":[{"name":"edit_file","arguments":{"Path":"` + filepath.ToSlash(filepath.Join(ws, "main.go")) + `"}},{"name":"edit_file","arguments":{"Path":"` + filepath.ToSlash(filepath.Join(ws, "README.md")) + `"}}]}` + "\n"
	if err := os.WriteFile(filepath.Join(logDir, "transcript.jsonl"), []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}

	views, err := ListProjects(dir)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("len(views) = %d, want 1", len(views))
	}
	v := views[0]
	if v.Name != "repo-alpha" || !v.Active || v.Model != "claude-sonnet-4-6" {
		t.Fatalf("unexpected project header: %#v", v)
	}
	if v.ContextLimit != 200_000 || v.LatestInput != 15000 || v.LatestCacheRead != 8000 || v.TotalInput != 25000 || v.TotalOutput != 800 {
		t.Fatalf("unexpected token stats: %#v", v)
	}

	if isModelRunning(true, "PLANNER_RESPONSE", "DONE") {
		t.Fatal("completed model response with presence lock should be idle, not running")
	}
	if !isModelRunning(true, "USER_INPUT", "DONE") {
		t.Fatal("pending user input with presence lock should be treated as running")
	}
}

func TestDismissAndReappearProjectWithoutDuplicates(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "repo-beta")
	historyPath := filepath.Join(dir, "history.jsonl")

	if err := os.MkdirAll(filepath.Join(dir, "presence"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "presence", "conv-1.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	line1 := `{"workspace":"` + filepath.ToSlash(ws) + `","conversationId":"conv-1","timestamp":1700000000000}` + "\n"
	if err := os.WriteFile(historyPath, []byte(line1), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := DismissProject(dir, ws); err != nil {
		t.Fatalf("DismissProject() error = %v", err)
	}
	views, err := ListProjects(dir)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(views) != 0 {
		t.Fatalf("expected dismissed project (even with idle presence lock) to stay hidden, got %d views", len(views))
	}

	// Simulate agy running again in the same workspace (with trailing slash) at a later timestamp.
	line2 := `{"workspace":"` + filepath.ToSlash(ws) + `/","conversationId":"conv-2","timestamp":9999999999999}` + "\n"
	if err := os.WriteFile(historyPath, []byte(line1+line2), 0o600); err != nil {
		t.Fatal(err)
	}

	views, err = ListProjects(dir)
	if err != nil {
		t.Fatalf("ListProjects() after new activity error = %v", err)
	}
	if len(views) != 1 || views[0].ConversationID != "conv-2" {
		t.Fatalf("expected 1 deduplicated project with conv-2, got %#v", views)
	}
}

func TestListProjectsWaitsForNewlineBeforeAdvancingTranscriptOffset(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "repo-partial")
	historyPath := filepath.Join(dir, "history.jsonl")
	transcriptDir := filepath.Join(dir, "brain", "conv-partial", ".system_generated", "logs")
	if err := os.MkdirAll(transcriptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(historyPath, []byte(`{"workspace":"`+filepath.ToSlash(ws)+`","conversationId":"conv-partial","timestamp":1700000000000}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	transcriptPath := filepath.Join(transcriptDir, "transcript.jsonl")
	partial := `{"type":"assistant","status":"done","model":"MODEL_CLAUDE_4_6_OPUS","input_tokens":1200,"output_tokens":300`
	if err := os.WriteFile(transcriptPath, []byte(partial), 0o600); err != nil {
		t.Fatal(err)
	}

	views, err := ListProjects(dir)
	if err != nil {
		t.Fatalf("ListProjects() initial error = %v", err)
	}
	if len(views) != 1 || views[0].StepCount != 0 {
		t.Fatalf("expected 0 steps for partial EOF line, got %#v", views)
	}

	if err := os.WriteFile(transcriptPath, []byte(partial+"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	views, err = ListProjects(dir)
	if err != nil {
		t.Fatalf("ListProjects() after completion error = %v", err)
	}
	if len(views) != 1 || views[0].StepCount != 1 || views[0].TotalInput != 1200 {
		t.Fatalf("expected 1 step with 1200 input tokens after newline, got %#v", views)
	}
}
