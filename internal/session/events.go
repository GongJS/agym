package session

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// Verdict is deliberately conservative: only an exact per-account Individual
// quota exhaustion may trigger an account switch.
type Verdict string

const (
	Ignore  Verdict = "ignore"
	Quota   Verdict = "quota"
	Suspect Verdict = "suspect"
)

var (
	quotaPattern   = regexp.MustCompile(`(?i)RESOURCE_EXHAUSTED \(code 429\):\s*Individual quota reached`)
	suspectPattern = regexp.MustCompile(`(?i)RESOURCE_EXHAUSTED|\b429\b|quota`)
	resetPattern   = regexp.MustCompile(`Resets in ([0-9hms]+)`)
)

// StopEvent is the safe subset of a hook Stop payload needed for handoff.
type StopEvent struct {
	ConversationID    string `json:"conversationId"`
	ModelName         string `json:"modelName"`
	TerminationReason string `json:"terminationReason"`
	Error             string `json:"error"`
	LastUserInput     string `json:"lastUserInput,omitempty"`
	TranscriptPath    string `json:"transcriptPath,omitempty"`
	FullyIdle         bool   `json:"fullyIdle"`
}

func ParseEvent(line string) (StopEvent, bool) {
	var payload struct {
		HookEventName     string `json:"hookEventName"`
		ConversationID    string `json:"conversationId"`
		ModelName         string `json:"modelName"`
		TerminationReason string `json:"terminationReason"`
		Error             string `json:"error"`
		LastUserInput     string `json:"lastUserInput"`
		TranscriptPath    string `json:"transcriptPath"`
		FullyIdle         *bool  `json:"fullyIdle"`
	}
	if err := json.Unmarshal([]byte(line), &payload); err != nil || payload.HookEventName != "Stop" {
		return StopEvent{}, false
	}
	fullyIdle := true
	if payload.FullyIdle != nil {
		fullyIdle = *payload.FullyIdle
	}
	return StopEvent{
		ConversationID:    payload.ConversationID,
		ModelName:         payload.ModelName,
		TerminationReason: payload.TerminationReason,
		Error:             payload.Error,
		LastUserInput:     payload.LastUserInput,
		TranscriptPath:    payload.TranscriptPath,
		FullyIdle:         fullyIdle,
	}, true
}

func Classify(event StopEvent) Verdict {
	if event.TerminationReason != "ERROR" {
		return Ignore
	}
	if quotaPattern.MatchString(event.Error) {
		return Quota
	}
	if suspectPattern.MatchString(event.Error) {
		return Suspect
	}
	return Ignore
}

func ResetHint(errorText string) string {
	match := resetPattern.FindStringSubmatch(errorText)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

// EventTail reads complete newline-terminated JSONL records only.
type EventTail struct {
	Path   string
	offset int64
}

func NewEventTail(path string) *EventTail { return &EventTail{Path: path} }

func (t *EventTail) Poll() []StopEvent {
	info, err := os.Stat(t.Path)
	if err != nil {
		return nil
	}
	if info.Size() < t.offset {
		t.offset = 0
	}
	file, err := os.Open(t.Path)
	if err != nil {
		return nil
	}
	defer file.Close()
	if _, err := file.Seek(t.offset, 0); err != nil {
		return nil
	}
	data := make([]byte, info.Size()-t.offset)
	n, _ := file.Read(data)
	data = data[:n]
	lastNewline := strings.LastIndexByte(string(data), '\n')
	if lastNewline < 0 {
		return nil
	}
	complete := data[:lastNewline+1]
	t.offset += int64(len(complete))
	lines := strings.Split(string(complete), "\n")
	var events []StopEvent
	for _, line := range lines {
		if event, ok := ParseEvent(line); ok {
			events = append(events, event)
		}
	}
	return events
}
