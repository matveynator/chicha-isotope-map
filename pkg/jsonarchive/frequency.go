package jsonarchive

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// =================
// Archive frequency
// =================

// Frequency expresses how often the archive generator should refresh the tgz
// snapshot. We keep it as a string-backed type so flag parsing remains simple
// while helper methods translate it into durations, labels, and filenames.
type Frequency string

const (
	FrequencyDaily   Frequency = "daily"
	FrequencyWeekly  Frequency = "weekly"
	FrequencyMonthly Frequency = "monthly"
	FrequencyYearly  Frequency = "yearly"
)

// ParseFrequency normalises user supplied cadence strings into a known value.
// We accept blanks as "weekly" so existing configurations keep working while the
// new flag remains optional.
func ParseFrequency(raw string) (Frequency, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	switch trimmed {
	case "", string(FrequencyWeekly):
		return FrequencyWeekly, nil
	case string(FrequencyDaily):
		return FrequencyDaily, nil
	case string(FrequencyMonthly):
		return FrequencyMonthly, nil
	case string(FrequencyYearly):
		return FrequencyYearly, nil
	default:
		return Frequency(""), fmt.Errorf("unsupported archive frequency %q", raw)
	}
}

// Slug returns the canonical lowercase token used in URLs and filenames. An
// unknown value defaults to the weekly slug so handlers fall back to the safe
// behaviour instead of crashing.
func (f Frequency) Slug() string {
	switch f {
	case FrequencyDaily:
		return "daily"
	case FrequencyWeekly:
		return "weekly"
	case FrequencyMonthly:
		return "monthly"
	case FrequencyYearly:
		return "yearly"
	default:
		return "weekly"
	}
}

// Interval describes roughly how long to wait between archive rebuilds. Month
// and year cadences use fixed day counts to keep scheduling deterministic
// without introducing calendar-specific complexity.
func (f Frequency) Interval() time.Duration {
	switch f {
	case FrequencyDaily:
		return 24 * time.Hour
	case FrequencyWeekly:
		return 7 * 24 * time.Hour
	case FrequencyMonthly:
		return 30 * 24 * time.Hour
	case FrequencyYearly:
		return 365 * 24 * time.Hour
	default:
		return FrequencyWeekly.Interval()
	}
}

// HumanInterval provides a readable label such as "week" for documentation and
// overview JSON responses.
func (f Frequency) HumanInterval() string {
	switch f {
	case FrequencyDaily:
		return "day"
	case FrequencyWeekly:
		return "week"
	case FrequencyMonthly:
		return "month"
	case FrequencyYearly:
		return "year"
	default:
		return FrequencyWeekly.HumanInterval()
	}
}

// Description returns a short sentence describing how often the archive is
// refreshed. Embedding the cadence keeps the API overview honest.
func (f Frequency) Description() string {
	return fmt.Sprintf("Updated once per %s", f.HumanInterval())
}

// RoutePath exposes the API URL that serves the archive for this cadence.
func (f Frequency) RoutePath() string {
	return fmt.Sprintf("/api/json/%s.tgz", f.Slug())
}

// FileName derives the on-disk tgz name by combining the domain (when present)
// with the cadence slug. Separators are trimmed to keep filesystem writes safe.
func FileName(domain string, f Frequency) string {
	slug := f.Slug()
	cleanedDomain := strings.TrimSpace(strings.ToLower(domain))
	if cleanedDomain == "" {
		return fmt.Sprintf("%s-json.tgz", slug)
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", string(filepath.Separator), "-", " ", "-")
	safeDomain := replacer.Replace(cleanedDomain)
	safeDomain = strings.Trim(safeDomain, "-._")
	if safeDomain == "" {
		return fmt.Sprintf("%s-json.tgz", slug)
	}
	return fmt.Sprintf("%s-%s-json.tgz", safeDomain, slug)
}
