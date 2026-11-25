package catalog

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	CatalogTTLSeconds = 24 * 60 * 60
	RetryBaseSeconds  = 60
	RetryMaxSeconds   = 30 * 60
)

// Model is one model offered by an account.
type Model struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Tag  string `json:"tag,omitempty"`
}

// Listing is the latest observation for one account. FetchedAt is the epoch
// timestamp at which the latest attempt finished. Models remain the last good
// list when the latest attempt failed.
type Listing struct {
	Models       []Model           `json:"models"`
	FetchedAt    float64           `json:"-"`
	Failures     int               `json:"failures,omitempty"`
	DefaultModel string            `json:"defaultModel,omitempty"`
	ModelTags    map[string]string `json:"modelTags,omitempty"`
}

// Catalog stores model listings by account email. A missing account is
// deliberately different from an account whose successful list is empty.
type Catalog struct {
	listings map[string]Listing
	path     string
}

// ModelCatalog is retained as a migration-friendly name.
type ModelCatalog = Catalog

func New(path string) *Catalog {
	return &Catalog{listings: make(map[string]Listing), path: path}
}

func Load(path string) *Catalog {
	c := New(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	var root struct {
		Accounts map[string]json.RawMessage `json:"accounts"`
	}
	if json.Unmarshal(data, &root) != nil {
		return c
	}
	for email, raw := range root.Accounts {
		if listing, ok := decodeListing(raw); ok {
			c.listings[email] = listing
		}
	}
	return c
}

func (c *Catalog) Path() string { return c.path }

// ParseModels parses one id<TAB>display name record per line. Malformed lines
// and rows missing either column are ignored, matching agylib/catalog.py.
func ParseModels(output string, tags map[string]string) []Model {
	models := make([]Model, 0)
	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		id, name := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if id == "" || name == "" {
			continue
		}
		models = append(models, Model{ID: id, Name: name, Tag: tags[id]})
	}
	return models
}

func Parse(output string) []Model { return ParseModels(output, nil) }

func (c *Catalog) Models(email string) ([]Model, bool) {
	listing, ok := c.listings[email]
	if !ok || len(listing.Models) == 0 {
		return nil, false
	}
	return append([]Model(nil), listing.Models...), true
}

func (c *Catalog) Listing(email string) (Listing, bool) {
	listing, ok := c.listings[email]
	if !ok {
		return Listing{}, false
	}
	listing.Models = append([]Model(nil), listing.Models...)
	listing.ModelTags = cloneTags(listing.ModelTags)
	return listing, true
}

func (c *Catalog) DefaultModel(email string) string {
	listing, ok := c.listings[email]
	if !ok {
		return ""
	}
	return listing.DefaultModel
}

func (c *Catalog) Find(email, modelID string) (Model, bool) {
	models, ok := c.Models(email)
	if !ok {
		return Model{}, false
	}
	for _, model := range models {
		if model.ID == modelID {
			return model, true
		}
	}
	return Model{}, false
}

func (c *Catalog) Offers(email, modelID string) bool {
	_, ok := c.Find(email, modelID)
	return ok
}

func (c *Catalog) NeedsFetch(email string, now float64) bool {
	listing, ok := c.listings[email]
	if !ok || listing.FetchedAt > now {
		return true
	}
	lifetime := float64(CatalogTTLSeconds)
	if listing.Failures > 0 {
		lifetime = float64(RetryDelay(listing.Failures))
	}
	return now-listing.FetchedAt >= lifetime
}

// Record stores a successful non-empty result, or records a failure for nil/
// empty models. A failure preserves the old models and default model.
func (c *Catalog) Record(email string, models []Model, fetchedAt float64, defaultModel ...string) {
	if len(models) > 0 {
		def := ""
		if len(defaultModel) > 0 {
			def = defaultModel[0]
		}
		c.listings[email] = Listing{Models: append([]Model(nil), models...), FetchedAt: fetchedAt, DefaultModel: def, ModelTags: tagsFor(models)}
		return
	}
	previous := c.listings[email]
	previous.FetchedAt = fetchedAt
	previous.Failures++
	c.listings[email] = previous
}

func RetryDelay(failures int) int {
	if failures < 1 {
		failures = 1
	}
	delay := RetryBaseSeconds
	for i := 1; i < failures && delay < RetryMaxSeconds; i++ {
		delay *= 2
	}
	if delay > RetryMaxSeconds {
		return RetryMaxSeconds
	}
	return delay
}

// Absorb takes the newest observation per account from another loaded catalog.
func (c *Catalog) Absorb(other *Catalog) {
	if other == nil {
		return
	}
	for email, listing := range other.listings {
		mine, ok := c.listings[email]
		if !ok || listing.FetchedAt > mine.FetchedAt {
			c.listings[email] = listing
		}
	}
}

func (c *Catalog) Retain(emails []string) {
	keep := make(map[string]bool, len(emails))
	for _, email := range emails {
		keep[email] = true
	}
	for email := range c.listings {
		if !keep[email] {
			delete(c.listings, email)
		}
	}
}

func (c *Catalog) Save() error {
	accounts := make(map[string]any, len(c.listings))
	for email, listing := range c.listings {
		item := map[string]any{"at": listing.FetchedAt, "models": listing.Models}
		if listing.Failures > 0 {
			item["failures"] = listing.Failures
		}
		if listing.DefaultModel != "" {
			item["default_model"] = listing.DefaultModel
		}
		accounts[email] = item
	}
	data, err := json.MarshalIndent(map[string]any{"accounts": accounts}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.path), ".models-*.tmp")
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
	return os.Rename(tmpPath, c.path)
}

func decodeListing(raw json.RawMessage) (Listing, bool) {
	var item struct {
		At                *float64          `json:"at"`
		FetchedAt         *float64          `json:"fetchedAt"`
		Models            []json.RawMessage `json:"models"`
		Failures          int               `json:"failures"`
		DefaultModel      string            `json:"default_model"`
		DefaultModelCamel string            `json:"defaultModel"`
	}
	if json.Unmarshal(raw, &item) != nil || item.Failures < 0 || (item.At == nil && item.FetchedAt == nil) {
		return Listing{}, false
	}
	at := item.At
	if at == nil {
		at = item.FetchedAt
	}
	if at == nil || math.IsNaN(*at) || math.IsInf(*at, 0) || (len(item.Models) == 0 && item.Failures == 0) {
		return Listing{}, false
	}
	def := item.DefaultModel
	if def == "" {
		def = item.DefaultModelCamel
	}
	models := make([]Model, 0, len(item.Models))
	for _, rawModel := range item.Models {
		var model Model
		if json.Unmarshal(rawModel, &model) == nil && model.ID != "" && model.Name != "" {
			models = append(models, model)
		}
	}
	if len(models) == 0 && item.Failures == 0 {
		return Listing{}, false
	}
	return Listing{Models: models, FetchedAt: *at, Failures: item.Failures, DefaultModel: def, ModelTags: tagsFor(models)}, true
}

func tagsFor(models []Model) map[string]string {
	tags := make(map[string]string)
	for _, model := range models {
		if model.Tag != "" {
			tags[model.ID] = model.Tag
		}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func cloneTags(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
