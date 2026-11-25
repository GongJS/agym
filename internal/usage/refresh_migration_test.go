package usage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gongjs/agym/internal/catalog"
	"github.com/gongjs/agym/internal/cloudmeta"
)

type scriptedRunner struct {
	usageResults []ProbeResult
	modelResults []ProbeResult
	usageCalls   int
	modelCalls   int
}

func (r *scriptedRunner) QueryUsage(context.Context, func() bool) ProbeResult {
	result := ProbeResult{ExitCode: 1, Outcome: "exited"}
	if r.usageCalls < len(r.usageResults) {
		result = r.usageResults[r.usageCalls]
	}
	r.usageCalls++
	return result
}

func (r *scriptedRunner) QueryModels(context.Context, func() bool) ProbeResult {
	result := ProbeResult{ExitCode: 1, Outcome: "exited"}
	if r.modelCalls < len(r.modelResults) {
		result = r.modelResults[r.modelCalls]
	}
	r.modelCalls++
	return result
}

func refreshKeychain() fakeKeychain {
	a, b := encodedLogin("a@example.com"), encodedLogin("b@example.com")
	return fakeKeychain{
		"gemini/antigravity":              a,
		"gemini-agy-backup/a@example.com": a,
		"gemini-agy-backup/b@example.com": b,
	}
}

func successfulUsage() ProbeResult {
	return ProbeResult{ExitCode: 0, Stdout: usageReport, Outcome: "exited"}
}

func TestRefreshRetriesFailedAccountAfterAnotherReachesGoogle(t *testing.T) {
	kc := refreshKeychain()
	roster, cachePath := setupRefresh(t, kc)
	runner := &scriptedRunner{usageResults: []ProbeResult{
		{ExitCode: 1, Stderr: "temporary network failure", Outcome: "exited"},
		successfulUsage(),
		successfulUsage(),
	}}
	refresher := NewRefresher(roster, runner, cachePath)
	refresher.CatalogPath = filepath.Join(t.TempDir(), "models.json")
	cache, err := refresher.Refresh(context.Background(), []string{"a@example.com", "b@example.com"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if runner.usageCalls != 3 {
		t.Fatalf("usage calls = %d, want failed account retried once after reachable account", runner.usageCalls)
	}
	if view, ok := cache.View("a@example.com"); !ok || view.Status != "ok" {
		t.Fatalf("retried account quota = %#v, %v; want ok", view, ok)
	}
	if kc["gemini/antigravity"] != encodedLogin("a@example.com") {
		t.Fatal("retry left the wrong login in the live slot")
	}
}

func TestRefreshSuppressesRetryWhenAllAccountsFail(t *testing.T) {
	kc := refreshKeychain()
	roster, cachePath := setupRefresh(t, kc)
	runner := &scriptedRunner{usageResults: []ProbeResult{
		{ExitCode: 1, Stderr: "network unavailable", Outcome: "exited"},
		{ExitCode: 1, Stderr: "network unavailable", Outcome: "exited"},
	}}
	refresher := NewRefresher(roster, runner, cachePath)
	refresher.CatalogPath = filepath.Join(t.TempDir(), "models.json")
	if _, err := refresher.Refresh(context.Background(), []string{"a@example.com", "b@example.com"}, true); err != nil {
		t.Fatal(err)
	}
	if runner.usageCalls != 2 {
		t.Fatalf("usage calls = %d, want no outage retry pass", runner.usageCalls)
	}
}

func TestRefreshRetriesSingleAccountOnceAndKeepsFailureBackoff(t *testing.T) {
	kc := refreshKeychain()
	roster, cachePath := setupRefresh(t, kc)
	runner := &scriptedRunner{usageResults: []ProbeResult{
		{ExitCode: 1, Stderr: "temporary error", Outcome: "exited"},
		{ExitCode: 1, Stderr: "still unavailable", Outcome: "exited"},
	}}
	refresher := NewRefresher(roster, runner, cachePath)
	refresher.CatalogPath = filepath.Join(t.TempDir(), "models.json")
	cache, err := refresher.Refresh(context.Background(), []string{"a@example.com"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if runner.usageCalls != 2 {
		t.Fatalf("usage calls = %d, want one immediate retry", runner.usageCalls)
	}
	entry := cache.entries["a@example.com"]
	if entry.Failures != 2 {
		t.Fatalf("consecutive failures = %d, want 2", entry.Failures)
	}
}

func TestRefreshCollectsMetadataAndModelCatalog(t *testing.T) {
	kc := refreshKeychain()
	roster, cachePath := setupRefresh(t, kc)
	catalogPath := filepath.Join(t.TempDir(), "models.json")
	runner := &scriptedRunner{
		usageResults: []ProbeResult{{
			ExitCode: 0, Stdout: usageReport, Outcome: "exited",
			Meta: &cloudmeta.CloudMeta{TierID: "ultra", TierName: "Ultra", RegionCode: "JP"},
		}},
		modelResults: []ProbeResult{{
			ExitCode: 0, Stdout: "gemini-4-neon\tGemini 4 Neon\n", Outcome: "exited",
			Meta: &cloudmeta.CloudMeta{DefaultModel: "gemini-4-neon", ModelTags: map[string]string{"gemini-4-neon": "New"}},
		}},
	}
	refresher := NewRefresher(roster, runner, cachePath)
	refresher.CatalogPath = catalogPath
	cache, err := refresher.Refresh(context.Background(), []string{"a@example.com"}, true)
	if err != nil {
		t.Fatal(err)
	}
	quota, ok := cache.View("a@example.com")
	if !ok || quota.TierID != "ultra" || quota.TierName != "Ultra" || quota.RegionCode != "JP" {
		t.Fatalf("quota metadata = %#v, %v", quota, ok)
	}
	models := catalog.Load(catalogPath)
	listed, ok := models.Models("a@example.com")
	if !ok || len(listed) != 1 || listed[0].ID != "gemini-4-neon" || listed[0].Tag != "New" {
		t.Fatalf("model listing = %#v, %v", listed, ok)
	}
	if models.DefaultModel("a@example.com") != "gemini-4-neon" || runner.modelCalls != 1 {
		t.Fatalf("default model=%q, model calls=%d", models.DefaultModel("a@example.com"), runner.modelCalls)
	}
}

func TestQuotaFromResultCarriesMetadataEvenOnRefusal(t *testing.T) {
	quota := quotaFromResult(ProbeResult{
		ExitCode: 1, Outcome: "exited", Stderr: "not eligible",
		Meta: &cloudmeta.CloudMeta{TierID: "pro", TierName: "Pro", RegionCode: "US"},
	})
	if quota.Status != "ineligible" || quota.TierID != "pro" || quota.RegionCode != "US" {
		t.Fatalf("refusal quota metadata = %#v", quota)
	}
}
