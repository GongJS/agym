package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

var ErrInvalidReport = errors.New("agy usage report is not valid JSON")

const membersPrefix = "Models within this group: "

// Bucket represents one shared Gemini/Claude quota window. Percent is -1 when unknown.
type Bucket struct {
	Percent int    `json:"percent"`
	Reset   string `json:"reset"`
}

// Usage is the normalized quota view shared by the CLI and desktop app.
type Usage struct {
	Buckets map[string]Bucket   `json:"buckets"`
	Members map[string][]string `json:"members,omitempty"`
}

type report struct {
	Command struct {
		Data struct {
			Groups []group `json:"groups"`
		} `json:"data"`
	} `json:"command"`
}

type group struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Buckets     []bucket `json:"buckets"`
}

type bucket struct {
	Window            string  `json:"window"`
	RemainingFraction float64 `json:"remaining_fraction"`
	ResetTime         string  `json:"reset_time"`
}

// Parse converts the JSON output of `agy -p /usage --output-format json` into
// stable bucket keys (gw, g5, cw, c5). Separate per-model groups are ignored
// when a family-level group explicitly lists its shared models.
func Parse(data []byte) (Usage, error) {
	var parsed report
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Usage{}, fmt.Errorf("%w: %v", ErrInvalidReport, err)
	}
	if parsed.Command.Data.Groups == nil {
		return Usage{}, fmt.Errorf("%w: missing command.data.groups", ErrInvalidReport)
	}

	usage := Usage{
		Buckets: map[string]Bucket{"gw": {Percent: -1}, "g5": {Percent: -1}, "cw": {Percent: -1}, "c5": {Percent: -1}},
		Members: make(map[string][]string),
	}
	sharedFamilies := make(map[string]bool)
	for _, item := range parsed.Command.Data.Groups {
		if item.Name == "" {
			continue
		}
		family := familyOfGroup(item.Name)
		models := parseMembers(item.Description)
		if len(models) > 0 {
			sharedFamilies[family] = true
			usage.Members[family] = append(usage.Members[family], models...)
		}
	}
	for _, item := range parsed.Command.Data.Groups {
		if item.Name == "" {
			continue
		}
		family := familyOfGroup(item.Name)
		models := parseMembers(item.Description)
		if sharedFamilies[family] && len(models) == 0 {
			continue
		}
		for _, row := range item.Buckets {
			window, ok := map[string]string{"weekly": "w", "5h": "5"}[row.Window]
			if !ok || math.IsNaN(row.RemainingFraction) || math.IsInf(row.RemainingFraction, 0) {
				continue
			}
			key := familyPrefix(family) + window
			usage.Buckets[key] = Bucket{Percent: int(math.Floor(row.RemainingFraction*100 + 0.5)), Reset: row.ResetTime}
		}
	}
	if len(usage.Members) == 0 {
		usage.Members = nil
	}
	return usage, nil
}

func familyOfGroup(name string) string {
	if strings.Contains(strings.ToLower(name), "gemini") {
		return "gemini"
	}
	return "claude"
}

func familyPrefix(family string) string {
	if family == "gemini" {
		return "g"
	}
	return "c"
}

func parseMembers(description string) []string {
	if !strings.HasPrefix(description, membersPrefix) {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(description, membersPrefix), ",")
	members := make([]string, 0, len(parts))
	for _, part := range parts {
		if member := strings.TrimSpace(part); member != "" {
			members = append(members, member)
		}
	}
	return members
}
