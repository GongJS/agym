package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gongjs/agym/internal/accounts"
	"github.com/gongjs/agym/internal/catalog"
	"github.com/gongjs/agym/internal/usage"
)

func TestClassifyRequiresExactIndividualQuotaStop(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		error  string
		want   Verdict
	}{
		{"exact", "ERROR", "RESOURCE_EXHAUSTED (code 429): Individual quota reached. Resets in 12m", Quota},
		{"case insensitive", "ERROR", "resource_exhausted (code 429): individual quota reached", Quota},
		{"rate limited", "ERROR", "RESOURCE_EXHAUSTED (code 429): rate limited", Suspect},
		{"generic 429", "ERROR", "HTTP 429 from server", Suspect},
		{"not an error stop", "SUCCESS", "RESOURCE_EXHAUSTED (code 429): Individual quota reached", Ignore},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(StopEvent{TerminationReason: tc.reason, Error: tc.error}); got != tc.want {
				t.Fatalf("Classify() = %q, want %q", got, tc.want)
			}
		})
	}
	if got := ResetHint("quota failed. Resets in 33m32s"); got != "33m32s" {
		t.Fatalf("ResetHint() = %q", got)
	}
}

func TestParseEventAndEventTailKeepPartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	tail := NewEventTail(path)
	line := `{"hookEventName":"Stop","conversationId":"conv-1","modelName":"gemini-3-pro","terminationReason":"ERROR","error":"RESOURCE_EXHAUSTED (code 429): Individual quota reached"}`
	if err := os.WriteFile(path, []byte(line[:len(line)-3]), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := tail.Poll(); len(got) != 0 {
		t.Fatalf("partial event emitted: %#v", got)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(line[len(line)-3:] + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	got := tail.Poll()
	if len(got) != 1 || got[0].ConversationID != "conv-1" || Classify(got[0]) != Quota {
		t.Fatalf("tail event = %#v", got)
	}
	if got := tail.Poll(); len(got) != 0 {
		t.Fatalf("duplicate event emitted: %#v", got)
	}
	if _, ok := ParseEvent(`{"hookEventName":"PreInvocation"}`); ok {
		t.Fatal("non-Stop hook event was accepted")
	}
}

func TestResumeArgsDropsOldSessionAndPinsModel(t *testing.T) {
	input := []string{"--model=old", "--continue", "--conversation", "old-id", "--new-project", "--color", "always", "-i", "old prompt", "--verbose"}
	want := []string{"--color", "always", "--verbose", "--model", "gemini-3-pro", "--conversation=new-id", "-i", "Continue from the previous response."}
	if got := ResumeArgs(input, "new-id", "Continue from the previous response.", "gemini-3-pro"); !reflect.DeepEqual(got, want) {
		t.Fatalf("ResumeArgs() = %#v, want %#v", got, want)
	}
	if ShouldSupervise([]string{"--print", "hello"}, true, true) || ShouldSupervise([]string{"models"}, true, true) || ShouldSupervise(nil, true, false) {
		t.Fatal("non-interactive command was marked supervised")
	}
	if !ShouldSupervise([]string{"--model", "gemini-3-pro"}, true, true) {
		t.Fatal("interactive session was not marked supervised")
	}
}

func TestCandidateOrderFiltersCompatibilityRefusalsAndRanksQuota(t *testing.T) {
	modelCatalog := catalog.New("")
	modelCatalog.Record("spent@example.com", []catalog.Model{{ID: "gemini-3-pro", Name: "Gemini 3 Pro"}}, 1)
	modelCatalog.Record("best@example.com", []catalog.Model{{ID: "gemini-3-pro", Name: "Gemini 3 Pro"}}, 1)
	modelCatalog.Record("low@example.com", []catalog.Model{{ID: "gemini-3-pro", Name: "Gemini 3 Pro"}}, 1)
	modelCatalog.Record("other-model@example.com", []catalog.Model{{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro"}}, 1)

	cachePath := filepath.Join(t.TempDir(), "quota.json")
	cache := usage.LoadCache(cachePath)
	cache.Record("best@example.com", usage.Quota{Status: "ok", Buckets: map[string]usage.Bucket{"gw": {Percent: 72}, "g5": {Percent: 61}}}, time.Now())
	cache.Record("low@example.com", usage.Quota{Status: "ok", Buckets: map[string]usage.Bucket{"gw": {Percent: 29}, "g5": {Percent: 24}}}, time.Now())
	cache.Record("refused@example.com", usage.Quota{Status: "ineligible"}, time.Now())
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	cache = usage.LoadCache(cachePath)
	pool := []accounts.Account{
		{Email: "low@example.com", Rotation: true},
		{Email: "best@example.com", Rotation: true},
		{Email: "other-model@example.com", Rotation: true},
		{Email: "refused@example.com", Rotation: true},
		{Email: "manual@example.com", Rotation: false},
	}
	modelID, got := CandidateOrder(pool, "spent@example.com", "gemini-3-pro", cache, modelCatalog)
	want := []string{"best@example.com", "low@example.com"}
	if modelID != "gemini-3-pro" || !reflect.DeepEqual(got, want) {
		t.Fatalf("CandidateOrder() = (%q, %v), want (%q, %v)", modelID, got, "gemini-3-pro", want)
	}
	modelID, got = CandidateOrder(pool, "spent@example.com", "Gemini 3 Pro", cache, modelCatalog)
	if modelID != "gemini-3-pro" || !reflect.DeepEqual(got, want) {
		t.Fatalf("CandidateOrder(display name) = (%q, %v), want (%q, %v)", modelID, got, "gemini-3-pro", want)
	}
	if score := QuotaScore(usage.Quota{Status: "ok", Buckets: map[string]usage.Bucket{"gw": {Percent: 95}, "g5": {Percent: 0}}}, "gemini-3-pro"); score != 0 {
		t.Fatalf("family score = %d, want 0", score)
	}
	serialized, err := json.Marshal(got)
	if err != nil || len(serialized) == 0 {
		t.Fatalf("candidate serialization failed: %v", err)
	}
}
