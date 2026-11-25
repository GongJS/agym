// Package cloudmeta passively extracts Cloud Code metadata through a local relay.
package cloudmeta

import (
	"encoding/json"
	"strings"
	"sync"
)

// CloudMeta contains metadata observed from Cloud Code responses.
type CloudMeta struct {
	TierID       string
	TierName     string
	RegionCode   string
	DefaultModel string
	ModelTags    map[string]string
}

func stringField(v any, key string) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

// ParseLoadCodeAssist returns the paid tier when present, otherwise current tier.
func ParseLoadCodeAssist(body []byte) (tierID, tierName string) {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return "", ""
	}
	for _, key := range []string{"paidTier", "currentTier"} {
		if tier, ok := payload[key].(map[string]any); ok {
			id := stringField(tier, "id")
			name := stringField(tier, "name")
			if id != "" || name != "" {
				return id, name
			}
		}
	}
	return "", ""
}

// ParseUserInfo returns the regionCode from fetchUserInfo.
func ParseUserInfo(body []byte) string {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	return stringField(payload, "regionCode")
}

// ParseAvailableModels returns the default model and non-empty model tag titles.
func ParseAvailableModels(body []byte) (defaultModel string, tags map[string]string) {
	tags = make(map[string]string)
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return "", tags
	}
	defaultModel = stringField(payload, "defaultAgentModelId")
	models, _ := payload["models"].(map[string]any)
	for id, raw := range models {
		if tag := stringField(raw, "tagTitle"); tag != "" {
			tags[id] = tag
		}
	}
	return defaultModel, tags
}

// Collector safely accumulates metadata from concurrent relay requests.
type Collector struct {
	mu   sync.RWMutex
	meta CloudMeta
}

// NewCollector creates an empty metadata collector.
func NewCollector() *Collector {
	return &Collector{meta: CloudMeta{ModelTags: make(map[string]string)}}
}

// Observe parses a successful response associated with path.
func (c *Collector) Observe(path string, body []byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case strings.Contains(path, "loadCodeAssist"):
		id, name := ParseLoadCodeAssist(body)
		if id != "" || name != "" {
			c.meta.TierID, c.meta.TierName = id, name
		}
	case strings.Contains(path, "fetchUserInfo"):
		if region := ParseUserInfo(body); region != "" {
			c.meta.RegionCode = region
		}
	case strings.Contains(path, "fetchAvailableModels"):
		model, tags := ParseAvailableModels(body)
		if model != "" {
			c.meta.DefaultModel = model
		}
		for id, tag := range tags {
			c.meta.ModelTags[id] = tag
		}
	}
}

// Snapshot returns a race-free copy of the collected metadata.
func (c *Collector) Snapshot() CloudMeta {
	if c == nil {
		return CloudMeta{ModelTags: map[string]string{}}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	m := c.meta
	m.ModelTags = make(map[string]string, len(c.meta.ModelTags))
	for k, v := range c.meta.ModelTags {
		m.ModelTags[k] = v
	}
	return m
}

// Meta is an alias for Snapshot.
func (c *Collector) Meta() CloudMeta { return c.Snapshot() }
