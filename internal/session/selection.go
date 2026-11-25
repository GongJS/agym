package session

import (
	"sort"
	"strings"

	"github.com/gongjs/agym/internal/accounts"
	"github.com/gongjs/agym/internal/catalog"
	"github.com/gongjs/agym/internal/usage"
)

const MinGoodPercent = 20

// CandidateOrder filters to rotation-enabled accounts and sorts by cached
// remaining quota. A model is a hard requirement only when the exhausted
// account's successful catalog explicitly lists it.
func CandidateOrder(pool []accounts.Account, current string, modelName string, cached *usage.Cache, models *catalog.Catalog) (modelID string, candidates []string) {
	if models != nil {
		if model, ok := models.Find(current, modelName); ok {
			modelID = model.ID
		} else if listed, ok := models.Models(current); ok {
			for _, model := range listed {
				if strings.EqualFold(model.ID, modelName) || strings.EqualFold(model.Name, modelName) {
					modelID = model.ID
					break
				}
			}
		}
	}
	for _, account := range pool {
		if account.Email == current || !account.Rotation {
			continue
		}
		if modelID != "" && (models == nil || !models.Offers(account.Email, modelID)) {
			continue
		}
		if cached != nil {
			if quota, ok := cached.LastKnownQuota(account.Email); ok && (quota.Status == "ineligible" || quota.Status == "unsupported_region") {
				continue
			}
		}
		candidates = append(candidates, account.Email)
	}
	family := modelFamily(modelName)
	sort.SliceStable(candidates, func(i, j int) bool {
		return cachedScore(cached, candidates[i], family) > cachedScore(cached, candidates[j], family)
	})
	return modelID, candidates
}

// QuotaScore returns the smallest remaining percentage in the model family's
// buckets, or -1 when a usable reading is unavailable.
func QuotaScore(quota usage.Quota, modelName string) int {
	if quota.Status != "ok" {
		return -1
	}
	keys := []string{"gw", "g5", "cw", "c5"}
	switch modelFamily(modelName) {
	case "gemini":
		keys = []string{"gw", "g5"}
	case "claude":
		keys = []string{"cw", "c5"}
	}
	min := 101
	for _, key := range keys {
		bucket, ok := quota.Buckets[key]
		if !ok || bucket.Percent < 0 {
			return -1
		}
		if bucket.Percent < min {
			min = bucket.Percent
		}
	}
	if min == 101 {
		return -1
	}
	return min
}

func cachedScore(cache *usage.Cache, email, family string) int {
	if cache == nil {
		return 0
	}
	quota, ok := cache.LastKnownQuota(email)
	if !ok {
		return 0
	}
	if score := QuotaScore(quota, family); score >= 0 {
		return score
	}
	return 0
}

func modelFamily(name string) string {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "gemini"):
		return "gemini"
	case strings.Contains(name, "claude"), strings.Contains(name, "gpt"):
		return "claude"
	default:
		return ""
	}
}
