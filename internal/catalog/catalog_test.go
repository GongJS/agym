package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const now = 1_000_000.0

var listed = []Model{{ID: "gemini", Name: "Gemini", Tag: "fast"}, {ID: "claude", Name: "Claude"}}

func TestParseModels(t *testing.T) {
	got := ParseModels("\n lonely\n\t\nid\t Name \n", nil)
	if len(got) != 1 || got[0] != (Model{ID: "id", Name: "Name"}) {
		t.Fatalf("ParseModels = %#v", got)
	}
	got = ParseModels("m\tModel\n", map[string]string{"m": "premium"})
	if got[0].Tag != "premium" {
		t.Fatalf("tags = %#v", got)
	}
}

func TestUnknownAndListingQueries(t *testing.T) {
	c := New("")
	if _, ok := c.Models("a"); ok || !c.NeedsFetch("a", now) {
		t.Fatal("unknown account should be unchecked")
	}
	c.Record("a", listed, now, "gemini")
	if got, ok := c.Find("a", "claude"); !ok || got.Name != "Claude" || !c.Offers("a", "gemini") {
		t.Fatal("listing lookup failed")
	}
	if c.DefaultModel("a") != "gemini" {
		t.Fatal("default model lost")
	}
}

func TestTTLClockRollbackAndBackoff(t *testing.T) {
	c := New("")
	c.Record("a", listed, now)
	if c.NeedsFetch("a", now+CatalogTTLSeconds-1) || !c.NeedsFetch("a", now+CatalogTTLSeconds) {
		t.Fatal("TTL mismatch")
	}
	if !c.NeedsFetch("a", now-1) {
		t.Fatal("clock rollback must refetch")
	}
	for failures, delay := 1, RetryBaseSeconds; failures <= 8; failures, delay = failures+1, min(delay*2, RetryMaxSeconds) {
		c.Record("b", nil, now+float64(failures))
		at := now + float64(failures)
		if c.NeedsFetch("b", at+float64(delay-1)) || !c.NeedsFetch("b", at+float64(delay)) {
			t.Fatalf("backoff %d = %d", failures, delay)
		}
	}
}

func TestFailurePreservesOldListingAndSuccessResets(t *testing.T) {
	c := New("")
	c.Record("a", listed, now, "gemini")
	c.Record("a", nil, now+1)
	got, ok := c.Models("a")
	if !ok || len(got) != 2 || c.DefaultModel("a") != "gemini" {
		t.Fatalf("failure dropped old listing: %#v", got)
	}
	c.Record("a", listed[:1], now+2)
	listing, _ := c.Listing("a")
	if listing.Failures != 0 || len(listing.Models) != 1 {
		t.Fatalf("success did not reset: %#v", listing)
	}
}

func TestSaveLoadLegacyJSONAndAtomicFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "models.json")
	c := New(path)
	c.Record("a", listed, now, "gemini")
	c.Record("b", nil, now+1)
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	loaded := Load(path)
	if got, ok := loaded.Models("a"); !ok || len(got) != 2 || loaded.DefaultModel("a") != "gemini" {
		t.Fatalf("round trip failed: %#v", got)
	}
	var raw map[string]any
	data, _ := os.ReadFile(path)
	if json.Unmarshal(data, &raw) != nil {
		t.Fatal("invalid JSON")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), ".models-*.tmp")); err == nil {
		t.Fatal("temporary file remains")
	}
	legacy := `{"accounts":{"a":{"at":1000000,"models":[{"id":"m","name":"M"}],"default_model":"m"},"bad":{"models":[]}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := Load(path).Models("a"); !ok || got[0].ID != "m" {
		t.Fatal("legacy JSON not loaded")
	}
}

func TestAbsorbAndRetain(t *testing.T) {
	mine, other := New(""), New("")
	mine.Record("a", listed[:1], now)
	other.Record("a", listed, now+1)
	other.Record("b", listed, now)
	mine.Absorb(other)
	if got, _ := mine.Models("a"); len(got) != 2 {
		t.Fatal("newer listing not absorbed")
	}
	mine.Retain([]string{"a"})
	if _, ok := mine.Models("b"); ok {
		t.Fatal("Retain kept removed account")
	}
}

func TestRetryDelayCap(t *testing.T) {
	if RetryDelay(1) != 60 || RetryDelay(2) != 120 || RetryDelay(100) != RetryMaxSeconds {
		t.Fatal("retry curve incorrect")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
