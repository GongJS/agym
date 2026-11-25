package usage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gongjs/agym/internal/accounts"
	"github.com/gongjs/agym/internal/catalog"
)

var regionRefusal = regexp.MustCompile(`not (?:currently )?available in your location`)

// Refresher coordinates cached quota/catalog reads with locked Keychain slot swaps.
type Refresher struct {
	Roster      *accounts.Roster
	Runner      Runner
	CachePath   string
	CatalogPath string
}

func NewRefresher(roster *accounts.Roster, runner Runner, cachePath string) *Refresher {
	if runner == nil {
		runner = AgyRunner{}
	}
	catalogPath, _ := DefaultModelCatalogPath()
	return &Refresher{Roster: roster, Runner: runner, CachePath: cachePath, CatalogPath: catalogPath}
}

func DefaultModelCatalogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "agy-tools", "models.json"), nil
}

// Refresh updates stale accounts, or all accounts when force is true. Quota is
// persisted before any optional model query so a model timeout cannot lose it.
func (r *Refresher) Refresh(ctx context.Context, emails []string, force bool) (*Cache, error) {
	if r == nil {
		return nil, errors.New("quota refresher is unavailable")
	}
	runner := r.Runner
	if runner == nil {
		runner = AgyRunner{}
	}
	cache := LoadCache(r.CachePath)
	catalogPath := r.CatalogPath
	if catalogPath == "" {
		catalogPath, _ = DefaultModelCatalogPath()
	}
	models := catalog.Load(catalogPath)
	requestedAt := time.Now()
	cache.Retain(emails)
	models.Retain(emails)
	targets := make([]string, 0, len(emails))
	for _, email := range emails {
		quotaDue := cache.NeedsProbe(email, requestedAt, requestedAt, force)
		if quotaDue || shouldFetchModels(models, email, latestMembers(cache, email), requestedAt) {
			targets = append(targets, email)
		}
	}
	if len(targets) == 0 {
		return cache, nil
	}

	firstPass := make(map[string]string, len(targets))
	for _, email := range targets {
		attempted, err := r.refreshOne(ctx, runner, cache, models, email, emails, requestedAt, force, false, catalogPath)
		if err != nil {
			return cache, err
		}
		if attempted {
			if entry, ok := cache.entries[email]; ok {
				firstPass[email] = entry.Latest.Quota.Status
			}
		}
	}
	if retryEmails := retryCandidates(firstPass); len(retryEmails) > 0 {
		for _, email := range retryEmails {
			if _, err := r.refreshOne(ctx, runner, cache, models, email, emails, requestedAt, true, true, catalogPath); err != nil {
				return cache, err
			}
		}
	}
	return cache, nil
}

func retryCandidates(firstPass map[string]string) []string {
	if len(firstPass) == 0 {
		return nil
	}
	reachedGoogle := false
	for _, status := range firstPass {
		if status == "ok" || status == "ineligible" || status == "unsupported_region" {
			reachedGoogle = true
			break
		}
	}
	if !reachedGoogle && len(firstPass) != 1 {
		return nil
	}
	var retries []string
	for email, status := range firstPass {
		if status == "parse_err" || status == "auth_failed" || status == "timeout" || status == "error" {
			retries = append(retries, email)
		}
	}
	sort.Strings(retries)
	return retries
}

func latestMembers(cache *Cache, email string) map[string][]string {
	if cache == nil {
		return nil
	}
	entry, ok := cache.entries[email]
	if !ok {
		return nil
	}
	return entry.Latest.Quota.Members
}

func shouldFetchModels(models *catalog.Catalog, email string, members map[string][]string, now time.Time) bool {
	if models == nil {
		return false
	}
	listing, exists := models.Listing(email)
	if !exists {
		return true
	}
	if listing.Failures > 0 {
		return models.NeedsFetch(email, float64(now.UnixNano())/1e9)
	}
	if models.NeedsFetch(email, float64(now.UnixNano())/1e9) {
		return true
	}
	catalogModels, known := models.Models(email)
	if !known || len(members) == 0 {
		return false
	}
	catalogNames := make([]string, 0, len(catalogModels))
	for _, model := range catalogModels {
		catalogNames = append(catalogNames, strings.ToLower(model.Name))
	}
	for _, groupMembers := range members {
		for _, member := range groupMembers {
			member = strings.ToLower(member)
			if member == "" {
				continue
			}
			found := false
			for _, name := range catalogNames {
				if strings.Contains(name, member) {
					found = true
					break
				}
			}
			if !found {
				return true
			}
		}
	}
	return false
}

func (r *Refresher) refreshOne(ctx context.Context, runner Runner, cache *Cache, models *catalog.Catalog, email string, retain []string, requestedAt time.Time, force, retry bool, catalogPath string) (attempted bool, err error) {
	if r.Roster == nil {
		return false, errors.New("account roster is unavailable")
	}
	err = r.Roster.WithSlotLock(ctx, true, func(slot *accounts.LockedSlot) (callbackErr error) {
		cache.Absorb(LoadCache(r.CachePath))
		models.Absorb(catalog.Load(catalogPath))
		now := time.Now()
		quotaDue := cache.NeedsProbe(email, now, requestedAt, force)
		modelDue := shouldFetchModels(models, email, latestMembers(cache, email), now)
		if !retry && !quotaDue && !modelDue {
			return nil
		}
		original, readErr := slot.CurrentRaw(ctx)
		if errors.Is(readErr, accounts.ErrNotFound) || original == "" {
			return errors.New("agy has no active login; quota refresh cannot safely restore the account slot")
		}
		if readErr != nil {
			return fmt.Errorf("read current agy login: %w", readErr)
		}
		originalEmail, parseErr := accounts.LoginEmail(original)
		if parseErr != nil {
			return fmt.Errorf("current agy login is invalid: %w", parseErr)
		}

		token := original
		if originalEmail != email {
			token, readErr = slot.BackupRaw(ctx, email)
			if errors.Is(readErr, accounts.ErrNotFound) {
				cache.Record(email, Quota{Status: "no_keychain"}, time.Now())
				cache.Retain(retain)
				attempted = true
				return cache.Save()
			}
			if readErr != nil {
				return fmt.Errorf("read saved login for %s: %w", email, readErr)
			}
			backupEmail, backupErr := accounts.LoginEmail(token)
			if backupErr != nil || backupEmail != email {
				return fmt.Errorf("saved login backup does not match account %s", email)
			}
		}

		defer func() {
			restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			current, readErr := slot.CurrentRaw(restoreCtx)
			if readErr != nil && !errors.Is(readErr, accounts.ErrNotFound) {
				callbackErr = errors.Join(callbackErr, fmt.Errorf("read slot before restore: %w", readErr))
				return
			}
			if current != original {
				if restoreErr := slot.WriteCurrent(restoreCtx, original); restoreErr != nil {
					callbackErr = errors.Join(callbackErr, fmt.Errorf("restore original agy login: %w", restoreErr))
				}
			}
		}()

		if token != original {
			if err := slot.WriteCurrent(ctx, token); err != nil {
				return fmt.Errorf("activate %s for quota probe: %w", email, err)
			}
		}
		result := runner.QueryUsage(ctx, slot.Contended)
		if result.Outcome == "yielded" {
			return nil
		}
		quota := quotaFromResult(result)
		cache.Record(email, quota, time.Now())
		cache.Retain(retain)
		if err := cache.Save(); err != nil {
			return fmt.Errorf("save quota cache: %w", err)
		}
		attempted = true

		modelRunner, canQueryModels := runner.(ModelRunner)
		if quota.Status != "ok" || !canQueryModels || !shouldFetchModels(models, email, quota.Members, time.Now()) {
			return nil
		}
		modelResult := modelRunner.QueryModels(ctx, slot.Contended)
		if modelResult.Outcome == "yielded" {
			return nil
		}
		var listed []catalog.Model
		defaultModel := ""
		var tags map[string]string
		if modelResult.Meta != nil {
			tags = modelResult.Meta.ModelTags
			defaultModel = modelResult.Meta.DefaultModel
		}
		if modelResult.Outcome == "exited" && modelResult.ExitCode == 0 {
			listed = catalog.ParseModels(modelResult.Stdout, tags)
		}
		models.Record(email, listed, float64(time.Now().UnixNano())/1e9, defaultModel)
		models.Retain(retain)
		_ = models.Save() // a catalog write failure costs another fetch, not the successful quota.
		return nil
	})
	return attempted, err
}

func quotaFromResult(result ProbeResult) Quota {
	quota := Quota{Status: "error"}
	if result.Meta != nil {
		quota.TierID = result.Meta.TierID
		quota.TierName = result.Meta.TierName
		quota.RegionCode = result.Meta.RegionCode
	}
	if result.Outcome == "oauth_prompt" {
		quota.Status = "auth_failed"
		quota.Message = "agy requested a browser sign-in; refresh was stopped to avoid opening a login flow"
		return quota
	}
	if result.Outcome == "timeout" {
		quota.Status = "timeout"
		quota.Message = "agy usage query timed out"
		return quota
	}
	if result.Outcome == "yielded" {
		quota.Status = "error"
		quota.Message = "refresh yielded to an interactive account operation"
		return quota
	}
	if result.ExitCode == 0 && result.Outcome == "exited" {
		parsed, err := Parse([]byte(result.Stdout))
		if err != nil || !hasReportedBucket(parsed) {
			quota.Status = "parse_err"
			quota.Message = "agy returned an unrecognized usage report"
			return quota
		}
		quota.Status = "ok"
		quota.Buckets = parsed.Buckets
		quota.Members = parsed.Members
		return quota
	}
	lowered := strings.ToLower(result.Stderr)
	if regionRefusal.MatchString(lowered) {
		quota.Status = "unsupported_region"
		return quota
	}
	if strings.Contains(lowered, "not eligible") {
		quota.Status = "ineligible"
		quota.Message = "agy reports that this account is not eligible"
		return quota
	}
	quota.Status = "error"
	quota.Message = "agy usage query failed"
	return quota
}

// QuotaFromProbe normalizes the result of an official agy /usage invocation.
// Session handoff uses it for candidate account probes as well as the refresher.
func QuotaFromProbe(result ProbeResult) Quota { return quotaFromResult(result) }

func hasReportedBucket(parsed Usage) bool {
	for _, bucket := range parsed.Buckets {
		if bucket.Percent >= 0 {
			return true
		}
	}
	return false
}
