package usage

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"time"
)

const (
	CacheVersion = 2
	cacheTTL     = 30 * time.Minute
	refusalTTL   = 24 * time.Hour
	retryBase    = time.Minute
	retryMax     = 30 * time.Minute
)

// Quota is the safe, normalized result of one official agy /usage probe.
type Quota struct {
	Status     string              `json:"status"`
	Buckets    map[string]Bucket   `json:"buckets,omitempty"`
	Members    map[string][]string `json:"members,omitempty"`
	Message    string              `json:"message,omitempty"`
	TierID     string              `json:"tierId,omitempty"`
	TierName   string              `json:"tierName,omitempty"`
	RegionCode string              `json:"regionCode,omitempty"`
}

// AccountView is safe for CLI and Wails consumers. A transient probe failure
// keeps the last successful buckets visible and exposes the failure separately.
type AccountView struct {
	Status     string              `json:"status"`
	Buckets    map[string]Bucket   `json:"buckets,omitempty"`
	Members    map[string][]string `json:"members,omitempty"`
	CheckedAt  int64               `json:"checkedAt,omitempty"`
	Error      string              `json:"error,omitempty"`
	TierID     string              `json:"tierId,omitempty"`
	TierName   string              `json:"tierName,omitempty"`
	RegionCode string              `json:"regionCode,omitempty"`
}

type snapshot struct {
	At    float64
	Quota Quota
}

type cacheEntry struct {
	Latest   snapshot
	Good     *snapshot
	Failures int
}

// Cache persists quota observations using the same per-account JSON layout as
// agylib. A missing or malformed cache is treated as empty.
type Cache struct {
	entries map[string]cacheEntry
	path    string
}

func DefaultCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "agy-tools", "quota_cache.json"), nil
}

func LoadCache(path string) *Cache {
	cache := &Cache{entries: make(map[string]cacheEntry), path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		return cache
	}
	var root struct {
		Accounts map[string]json.RawMessage `json:"accounts"`
	}
	if json.Unmarshal(data, &root) != nil {
		return cache
	}
	for email, raw := range root.Accounts {
		var item struct {
			At       *float64        `json:"at"`
			Result   json.RawMessage `json:"result"`
			Failures int             `json:"failures"`
			Good     *struct {
				At     *float64        `json:"at"`
				Result json.RawMessage `json:"result"`
			} `json:"good"`
		}
		if json.Unmarshal(raw, &item) != nil || item.At == nil || !validTimestamp(*item.At) || item.Failures < 0 {
			continue
		}
		quota, ok := decodeQuota(item.Result)
		if !ok {
			continue
		}
		entry := cacheEntry{Latest: snapshot{At: *item.At, Quota: quota}, Failures: item.Failures}
		if quota.Status == "ok" {
			good := entry.Latest
			entry.Good = &good
		} else if item.Good != nil && item.Good.At != nil && validTimestamp(*item.Good.At) {
			goodQuota, valid := decodeQuota(item.Good.Result)
			if valid && goodQuota.Status == "ok" {
				good := snapshot{At: *item.Good.At, Quota: goodQuota}
				entry.Good = &good
			}
		}
		cache.entries[email] = entry
	}
	return cache
}

func (c *Cache) Path() string { return c.path }

// Absorb keeps whichever process has the newer observation for each account.
func (c *Cache) Absorb(other *Cache) {
	if other == nil {
		return
	}
	for email, entry := range other.entries {
		mine, ok := c.entries[email]
		if !ok || entry.Latest.At > mine.Latest.At {
			c.entries[email] = entry
		}
	}
}

func (c *Cache) View(email string) (AccountView, bool) {
	entry, ok := c.entries[email]
	if !ok {
		return AccountView{}, false
	}
	view := AccountView{Status: entry.Latest.Quota.Status, CheckedAt: int64(entry.Latest.At)}
	if entry.Latest.Quota.failed() && entry.Good != nil {
		view.Status = "stale"
		view.CheckedAt = int64(entry.Good.At)
		view.Error = entry.Latest.Quota.Message
		view.Buckets = cloneBuckets(entry.Good.Quota.Buckets)
		view.Members = cloneMembers(entry.Good.Quota.Members)
		view.TierID = entry.Good.Quota.TierID
		view.TierName = entry.Good.Quota.TierName
		view.RegionCode = entry.Good.Quota.RegionCode
		return view, true
	}
	view.Error = entry.Latest.Quota.Message
	view.Buckets = cloneBuckets(entry.Latest.Quota.Buckets)
	view.Members = cloneMembers(entry.Latest.Quota.Members)
	view.TierID = entry.Latest.Quota.TierID
	view.TierName = entry.Latest.Quota.TierName
	view.RegionCode = entry.Latest.Quota.RegionCode
	return view, true
}

// LastKnownQuota returns the latest usable quota snapshot for candidate ranking.
// A transient failure uses the last successful snapshot when one is available.
func (c *Cache) LastKnownQuota(email string) (Quota, bool) {
	if c == nil {
		return Quota{}, false
	}
	entry, ok := c.entries[email]
	if !ok {
		return Quota{}, false
	}
	quota := entry.Latest.Quota
	if quota.failed() && entry.Good != nil {
		quota = entry.Good.Quota
	}
	quota.Buckets = cloneBuckets(quota.Buckets)
	quota.Members = cloneMembers(quota.Members)
	return quota, true
}

func (c *Cache) NeedsProbe(email string, now, requestedAt time.Time, force bool) bool {
	entry, exists := c.entries[email]
	if !exists {
		return true
	}
	checked := time.Unix(0, int64(entry.Latest.At*float64(time.Second)))
	if checked.After(now) {
		return true
	}
	if !checked.Before(requestedAt) {
		return false
	}
	if force {
		return true
	}
	quota := entry.Latest.Quota
	wait := cacheTTL
	if quota.failed() {
		wait = retryDelay(entry.Failures)
	} else if quota.Status == "ineligible" || quota.Status == "unsupported_region" {
		wait = refusalTTL
	}
	return now.Sub(checked) >= wait
}

func (c *Cache) Record(email string, quota Quota, at time.Time) {
	entry := cacheEntry{Latest: snapshot{At: float64(at.UnixNano()) / 1e9, Quota: quota}}
	if quota.Status == "ok" {
		good := entry.Latest
		entry.Good = &good
	} else if quota.failed() {
		previous, exists := c.entries[email]
		if exists {
			entry.Good = previous.Good
			entry.Failures = previous.Failures
		}
		entry.Failures++
	}
	c.entries[email] = entry
}

func (c *Cache) Retain(emails []string) {
	keep := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		keep[email] = struct{}{}
	}
	for email := range c.entries {
		if _, ok := keep[email]; !ok {
			delete(c.entries, email)
		}
	}
}

func (c *Cache) Save() error {
	if c.path == "" {
		return errors.New("quota cache path is empty")
	}
	accounts := make(map[string]any, len(c.entries))
	for email, entry := range c.entries {
		item := map[string]any{"at": entry.Latest.At, "result": quotaJSON(entry.Latest.Quota)}
		if entry.Failures > 0 {
			item["failures"] = entry.Failures
		}
		if entry.Latest.Quota.failed() && entry.Good != nil {
			item["good"] = map[string]any{"at": entry.Good.At, "result": quotaJSON(entry.Good.Quota)}
		}
		accounts[email] = item
	}
	data, err := json.MarshalIndent(map[string]any{"version": CacheVersion, "accounts": accounts}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.path), ".quota-cache-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, c.path); err != nil {
		return err
	}
	return nil
}

func validTimestamp(timestamp float64) bool {
	return !math.IsNaN(timestamp) && !math.IsInf(timestamp, 0)
}

func decodeQuota(raw json.RawMessage) (Quota, bool) {
	var values map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return Quota{}, false
	}
	var quota Quota
	if json.Unmarshal(values["status"], &quota.Status) != nil || !validStatus(quota.Status) {
		return Quota{}, false
	}
	quota.Buckets = make(map[string]Bucket)
	for _, key := range []string{"gw", "g5", "cw", "c5"} {
		if len(values[key]) == 0 {
			continue
		}
		var bucket struct {
			Percent int    `json:"pct"`
			Reset   string `json:"reset"`
		}
		if json.Unmarshal(values[key], &bucket) == nil {
			quota.Buckets[key] = Bucket{Percent: bucket.Percent, Reset: bucket.Reset}
		}
	}
	if len(values["models"]) > 0 {
		_ = json.Unmarshal(values["models"], &quota.Members)
	}
	_ = json.Unmarshal(values["msg"], &quota.Message)
	_ = json.Unmarshal(values["tier_id"], &quota.TierID)
	_ = json.Unmarshal(values["tier_name"], &quota.TierName)
	_ = json.Unmarshal(values["region_code"], &quota.RegionCode)
	return quota, true
}

func quotaJSON(quota Quota) map[string]any {
	data := map[string]any{"status": quota.Status}
	for key, bucket := range quota.Buckets {
		data[key] = map[string]any{"pct": bucket.Percent, "reset": bucket.Reset}
	}
	if len(quota.Members) > 0 {
		data["models"] = quota.Members
	}
	if quota.Message != "" {
		data["msg"] = quota.Message
	}
	if quota.TierID != "" {
		data["tier_id"] = quota.TierID
	}
	if quota.TierName != "" {
		data["tier_name"] = quota.TierName
	}
	if quota.RegionCode != "" {
		data["region_code"] = quota.RegionCode
	}
	return data
}

func (q Quota) failed() bool {
	return q.Status == "parse_err" || q.Status == "auth_failed" || q.Status == "timeout" || q.Status == "error"
}

func validStatus(status string) bool {
	switch status {
	case "ok", "no_keychain", "ineligible", "unsupported_region", "parse_err", "auth_failed", "timeout", "error":
		return true
	default:
		return false
	}
}

func retryDelay(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	delay := retryBase
	for i := 1; i < failures && delay < retryMax; i++ {
		delay *= 2
	}
	if delay > retryMax {
		return retryMax
	}
	return delay
}

func cloneBuckets(in map[string]Bucket) map[string]Bucket {
	if in == nil {
		return nil
	}
	out := make(map[string]Bucket, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneMembers(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for key, value := range in {
		out[key] = append([]string(nil), value...)
	}
	return out
}
