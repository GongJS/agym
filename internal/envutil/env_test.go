package envutil

import (
	"strings"
	"testing"
)

func TestReplaceRemovesPreviousValueAndAppendsNew(t *testing.T) {
	got := Replace(
		[]string{"A=1", "AGY_TOOLS_SESSION_EVENTS=old", "MALFORMED", "B=2"},
		map[string]string{"AGY_TOOLS_SESSION_EVENTS": "new", "C": "3"},
	)
	joined := strings.Join(got, "\n")
	if strings.Count(joined, "AGY_TOOLS_SESSION_EVENTS=") != 1 {
		t.Fatalf("duplicate key in %#v", got)
	}
	if !strings.Contains(joined, "AGY_TOOLS_SESSION_EVENTS=new") || !strings.Contains(joined, "C=3") {
		t.Fatalf("missing overridden/appended keys in %#v", got)
	}
	if strings.Contains(joined, "MALFORMED") {
		t.Fatalf("malformed entry should be skipped in %#v", got)
	}
}
