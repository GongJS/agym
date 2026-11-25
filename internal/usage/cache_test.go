package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadPythonCachePreservesQuotaAndMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota_cache.json")
	legacy := `{"version":2,"accounts":{"a@example.com":{"at":1730000000,"result":{"status":"ok","gw":{"pct":42,"reset":"tomorrow"},"models":{"gemini":["gemini-2.5-pro"]},"tier_id":"ultra","tier_name":"Ultra","region_code":"US"}}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := LoadCache(path)
	view, ok := cache.View("a@example.com")
	if !ok || view.Status != "ok" || view.Buckets["gw"].Percent != 42 || view.Buckets["gw"].Reset != "tomorrow" {
		t.Fatalf("View() = %#v, %v", view, ok)
	}
	if !cache.NeedsProbe("a@example.com", time.Unix(1730002000, 0), time.Unix(1730002000, 0), false) {
		t.Fatal("cache older than 30 minutes should be stale")
	}
	cache.Record("a@example.com", Quota{Status: "error", Message: "temporary failure"}, time.Unix(1730000600, 0))
	view, ok = cache.View("a@example.com")
	if !ok || view.Status != "stale" || view.Buckets["gw"].Percent != 42 || view.Error != "temporary failure" {
		t.Fatalf("failed probe did not preserve last good quota: %#v", view)
	}
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded := LoadCache(path)
	view, ok = reloaded.View("a@example.com")
	if !ok || view.Status != "stale" || view.Buckets["gw"].Percent != 42 {
		t.Fatalf("round trip lost last good quota: %#v", view)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{`"pct": 42`, `"tier_id": "ultra"`, `"tier_name": "Ultra"`, `"region_code": "US"`} {
		if !containsString(string(data), wanted) {
			t.Errorf("saved cache missing %s", wanted)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("cache permissions = %v, %v; want 0600", info, err)
	}
}

func TestLoadCacheRejectsMissingAndMalformedTimestamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota_cache.json")
	data := `{"accounts":{"missing-at":{"result":{"status":"ok"}},"bad-good-at":{"at":1730000000,"result":{"status":"error"},"good":{"result":{"status":"ok","gw":{"pct":50}}}}}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := LoadCache(path)
	if _, ok := cache.View("missing-at"); ok {
		t.Fatal("cache entry without an at timestamp was accepted")
	}
	view, ok := cache.View("bad-good-at")
	if !ok || view.Status != "error" || len(view.Buckets) != 0 {
		t.Fatalf("cache accepted a good snapshot without its timestamp: %#v, %v", view, ok)
	}
}

func TestCacheFailureBackoffAndRetention(t *testing.T) {
	cache := &Cache{entries: map[string]cacheEntry{}, path: filepath.Join(t.TempDir(), "cache.json")}
	now := time.Now()
	cache.Record("a@example.com", Quota{Status: "error"}, now)
	requestedAt := now.Add(time.Second)
	if cache.NeedsProbe("a@example.com", now.Add(30*time.Second), requestedAt, false) {
		t.Fatal("first failure should not be due before the one-minute retry delay")
	}
	if !cache.NeedsProbe("a@example.com", now.Add(2*time.Minute), requestedAt, false) {
		t.Fatal("first failure should be due after one minute")
	}
	cache.Record("b@example.com", Quota{Status: "ok"}, now)
	cache.Retain([]string{"b@example.com"})
	if _, ok := cache.View("a@example.com"); ok {
		t.Fatal("Retain kept an account no longer in the roster")
	}
}

func containsString(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
