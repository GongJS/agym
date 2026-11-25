package usage

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseSharedFamiliesAndPercentRounding(t *testing.T) {
	input := map[string]any{
		"command": map[string]any{
			"data": map[string]any{
				"groups": []any{
					map[string]any{"name": "gemini-shared", "description": "Models within this group: gemini-2.5-pro, gemini-2.5-flash", "buckets": []any{map[string]any{"window": "weekly", "remaining_fraction": 0.735, "reset_time": "tomorrow"}}},
					map[string]any{"name": "gemini-special", "description": "Independent model limit", "buckets": []any{map[string]any{"window": "weekly", "remaining_fraction": 0.1}}},
					map[string]any{"name": "claude", "description": "", "buckets": []any{map[string]any{"window": "5h", "remaining_fraction": 0.5}}},
				},
			},
		},
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Buckets["gw"].Percent != 74 || got.Buckets["gw"].Reset != "tomorrow" {
		t.Fatalf("Gemini weekly bucket = %#v", got.Buckets["gw"])
	}
	if got.Buckets["c5"].Percent != 50 {
		t.Fatalf("Claude 5h bucket = %#v", got.Buckets["c5"])
	}
	if got.Buckets["cw"].Percent != -1 {
		t.Fatalf("unknown bucket should remain -1: %#v", got.Buckets["cw"])
	}
	if len(got.Members["gemini"]) != 2 {
		t.Fatalf("Gemini members = %v", got.Members["gemini"])
	}
}

func TestParseRejectsNonJSON(t *testing.T) {
	if _, err := Parse([]byte("not json")); !errors.Is(err, ErrInvalidReport) {
		t.Fatalf("Parse error = %v", err)
	}
}
