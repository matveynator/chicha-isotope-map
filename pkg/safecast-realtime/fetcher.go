package safecastrealtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"chicha-isotope-map/pkg/countryresolver"
	"chicha-isotope-map/pkg/database"
)

// devicePayload maps the minimal fields we need from the Safecast JSON.
// A custom UnmarshalJSON keeps the struct small while flexibly handling
// different upstream field names like "lat" vs "latitude". This follows
// the Go Proverb "Clear is better than clever" by keeping decoding logic
// explicit and easy to inspect.
type devicePayload struct {
	ID      string
	Type    string  // transport tag such as car or walk
	Class   string  // upstream device_class retained for unit heuristics
	Name    string  // human friendly device title from the feed
	Tube    string  // detector type as advertised by the feed
	Value   float64 // dose rate
	Unit    string  // unit of Value
	Lat     float64 // latitude in degrees
	Lon     float64 // longitude in degrees
	Time    int64   // measurement timestamp
	Country string  // optional country hint
	Metrics map[string]float64
}

// measurementCandidate records a possible radiation reading parsed from the
// JSON payload.  We keep the struct tiny and local so the selection logic in
// UnmarshalJSON stays readable and follows "Clear is better than clever".
type measurementCandidate struct {
	value    float64
	unit     string
	key      string
	priority int
}

const (
	priorityDoseRate        = iota // explicit µSv/h or µR/h hints
	priorityCountsPerSecond        // CPS fields
	priorityCountsPerMinute        // CPM fields
	priorityGenericCounts          // any remaining detector field
)

// numericValue converts different JSON value representations into float64.
// Safecast occasionally serialises environmental metrics as strings; normalising
// them here keeps the rest of the pipeline simple and mirrors "Clear is better
// than clever" by exposing plain numbers downstream.
func numericValue(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// newMeasurementCandidate normalises a possible "lnd_*" measurement field into
// a measurementCandidate.  Returning false keeps the caller compact when the
// value is missing or malformed.  Explicit dose rate hints trump counts so the
// selected reading reflects the most calibrated number available.
func newMeasurementCandidate(key string, raw any) (measurementCandidate, bool) {
	val, ok := numericValue(raw)
	if !ok {
		return measurementCandidate{}, false
	}
	lower := strings.ToLower(key)
	candidate := measurementCandidate{
		value:    val,
		unit:     key,
		key:      lower,
		priority: priorityGenericCounts,
	}
	if strings.Contains(lower, "usv") {
		candidate.unit = "µSv/h"
		candidate.priority = priorityDoseRate
		return candidate, true
	}
	if strings.Contains(lower, "urh") {
		candidate.value = val / 100.0
		candidate.unit = "µSv/h"
		candidate.priority = priorityDoseRate
		return candidate, true
	}
	if strings.Contains(lower, "cps") {
		candidate.priority = priorityCountsPerSecond
		return candidate, true
	}
	if strings.Contains(lower, "cpm") {
		candidate.priority = priorityCountsPerMinute
		return candidate, true
	}
	return candidate, true
}

// metricKeyFor recognises temperature, humidity, and pressure hints from the
// payload map.  Returning a normalised key allows us to keep the stored JSON
// compact and easy to translate in the UI later.
func metricKeyFor(raw string) (string, bool) {
	if strings.Contains(raw, "lnd") {
		return "", false // measurement fields are handled separately
	}
	if strings.Contains(raw, "temperature") || strings.HasPrefix(raw, "temp") || strings.Contains(raw, "_temp") {
		if strings.Contains(raw, "_f") || strings.Contains(raw, "tempf") {
			return "temperature_f", true
		}
		return "temperature_c", true
	}
	if strings.Contains(raw, "humidity") || strings.HasPrefix(raw, "humid") {
		return "humidity_percent", true
	}
	if strings.Contains(raw, "press") || strings.Contains(raw, "baro") {
		return "pressure_hpa", true
	}
	return "", false
}

// reinterpretCountsUnit upgrades bare detector fields from specific hardware
// into explicit CPS units.  Blues Radnote and Airnote devices expose counts under
// "lnd_7318*" without suffixes, so mapping them here keeps downstream
// conversion consistent with Safecast documentation while avoiding extra
// conditionals elsewhere.
func reinterpretCountsUnit(class, unit string) string {
	if class == "" || unit == "" {
		return unit
	}
	loweredClass := strings.ToLower(class)
	if !strings.Contains(loweredClass, "radnote") && !strings.Contains(loweredClass, "airnote") {
		return unit
	}
	loweredUnit := strings.ToLower(unit)
	if !strings.HasPrefix(loweredUnit, "lnd_7318") {
		return unit
	}
	if strings.Contains(loweredUnit, "cps") || strings.Contains(loweredUnit, "cpm") || strings.Contains(loweredUnit, "usv") || strings.Contains(loweredUnit, "urh") {
		return unit
	}
	return unit + "_cps"
}

// addMetric records a derived environmental metric when present.
func (d *devicePayload) addMetric(key string, value float64) {
	if d.Metrics == nil {
		d.Metrics = make(map[string]float64)
	}
	d.Metrics[key] = value
}

// encodeMetrics serialises optional metrics so they can be stored alongside realtime rows.
// Returning an empty string keeps SQL inserts simple when no additional data is present.
func encodeMetrics(metrics map[string]float64) string {
	if len(metrics) == 0 {
		return ""
	}
	b, err := json.Marshal(metrics)
	if err != nil {
		log.Printf("encode metrics: %v", err)
		return ""
	}
	return string(b)
}

// UnmarshalJSON decodes a devicePayload from a generic map so we can
// tolerate field name variations in the upstream feed. Safecast's
// /devices endpoint uses verbose names like "loc_lat" and embeds the
// last reading in fields such as "lnd_7318u". Parsing into a map keeps
// the code straightforward and mirrors the Go Proverb "Clear is better
// than clever".
func (d *devicePayload) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	// Device identifier
	if v, ok := m["device_urn"].(string); ok {
		d.ID = v
	} else if v, ok := m["id"].(string); ok {
		d.ID = v
	} else if v, ok := m["device"].(float64); ok {
		d.ID = fmt.Sprintf("%d", int64(v))
	}

	// Transport or class info
	if v, ok := m["device_class"].(string); ok {
		d.Class = v
		if d.Type == "" {
			d.Type = v
		}
	}
	if v, ok := m["service_transport"].(string); ok {
		parts := strings.Split(v, ":")
		if len(parts) > 0 {
			d.Type = strings.TrimSpace(parts[0])
		}
		for _, p := range parts[1:] {
			p = strings.TrimSpace(p)
			if p != "" {
				d.Tube = p
				break
			}
		}
	}

	// Capture the descriptive title so downstream logic can recognise
	// Safecast Air units by name.  We accept several field variants
	// because the upstream feed does not always use the same keys.
	if v, ok := m["device_title"].(string); ok {
		d.Name = v
	} else if v, ok := m["device_name"].(string); ok {
		d.Name = v
	} else if v, ok := m["title"].(string); ok {
		d.Name = v
	}

	if d.Tube == "" {
		if v, ok := m["tube_type"].(string); ok {
			d.Tube = v
		} else if v, ok := m["tube"].(string); ok {
			d.Tube = v
		}
	}

	// Coordinates arrive under loc_lat/loc_lon.
	if v, ok := m["loc_lat"].(float64); ok {
		d.Lat = v
	} else if v, ok := m["lat"].(float64); ok {
		d.Lat = v
	} else if v, ok := m["latitude"].(float64); ok {
		d.Lat = v
	}
	if v, ok := m["loc_lon"].(float64); ok {
		d.Lon = v
	} else if v, ok := m["lon"].(float64); ok {
		d.Lon = v
	} else if v, ok := m["longitude"].(float64); ok {
		d.Lon = v
	}

	// Measurement value: pick the most explicit "lnd_" field.  Older feeds
	// expose a mix of CPM, CPS, and legacy µR/h integers, so we collect all
	// candidates and prefer explicit dose rates first, then per-second
	// counts, then per-minute counts, and finally any remaining raw field.
	// This keeps the selection deterministic and follows "Clear is better
	// than clever" by centralising the heuristics.
	best := measurementCandidate{priority: math.MaxInt}
	for k, raw := range m {
		if !strings.HasPrefix(k, "lnd_") {
			continue
		}
		candidate, ok := newMeasurementCandidate(k, raw)
		if !ok {
			continue
		}
		if candidate.priority < best.priority || (candidate.priority == best.priority && candidate.key < best.key) {
			best = candidate
		}
	}
	if best.priority != math.MaxInt {
		d.Value = best.value
		d.Unit = reinterpretCountsUnit(d.Class, best.unit)
	}

	// Timestamp is provided as RFC3339 string.
	if v, ok := m["when_captured"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			d.Time = t.Unix()
		}
	} else if v, ok := m["value_time"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			d.Time = t.Unix()
		}
	} else if v, ok := m["captured_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			d.Time = t.Unix()
		}
	} else if v, ok := m["timestamp"].(float64); ok {
		d.Time = int64(v)
	} else if v, ok := m["time"].(float64); ok {
		d.Time = int64(v)
	}

	// Optional country hint if provided.
	if v, ok := m["country"].(string); ok {
		d.Country = v
	}

	for k, v := range m {
		key, ok := metricKeyFor(strings.ToLower(k))
		if !ok {
			continue
		}
		val, ok := numericValue(v)
		if !ok {
			continue
		}
		d.addMetric(key, val)
	}
	return nil
}

// fetch pulls device data once.
// Returning raw payload keeps this function simple and lets callers derive
// both measurements and summaries without another pass.
func fetch(ctx context.Context, url string) ([]devicePayload, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var raw []devicePayload
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// containsAir reports whether the descriptor hints at Safecast Air hardware.
// Air units do not produce radiation readings, so we filter them eagerly.
func containsAir(s string) bool {
	if s == "" {
		return false
	}
	return strings.Contains(strings.ToLower(s), "air")
}

// cleanDetector strips IP-like tokens and normalises whitespace so the UI
// displays meaningful detector labels.  The helper follows "Clear is better
// than clever" by keeping the sanitising logic small and transparent.
func cleanDetector(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.Trim(trimmed, "\"'")
	if trimmed == "" {
		return ""
	}
	if ip := net.ParseIP(trimmed); ip != nil {
		return ""
	}
	hasLetter := false
	for _, r := range trimmed {
		if unicode.IsLetter(r) {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return ""
	}
	return strings.Join(strings.Fields(trimmed), " ")
}

// DetectorLabel chooses the most descriptive string out of the available
// Safecast fields.  We prefer explicit tube information, then fall back to the
// transport hint, and finally the device name.  Returning a single string keeps
// the calling code tidy and avoids leaking implementation details elsewhere.
func DetectorLabel(tube, transport, name string) string {
	candidates := []string{tube, transport, name}
	for _, c := range candidates {
		if label := cleanDetector(c); label != "" {
			return label
		}
	}
	return ""
}

// convertIfRadiation filters Safecast Air units and returns the converted
// radiation value when the reading looks usable.  Returning the µSv/h value
// here keeps the calling loop simple and lets us reuse the same conversion
// for summaries without reprocessing the payload.
func convertIfRadiation(d devicePayload) (float64, bool) {
	if containsAir(d.ID) || containsAir(d.Type) || containsAir(d.Tube) || containsAir(d.Name) {
		return 0, false
	}
	return FromRealtime(d.Value, d.Unit)
}

// Start launches background workers that keep the live table updated.
// We poll every five minutes so active counters stay fresh while still being
// polite to the upstream service.
// Two goroutines communicate over a channel; no mutex is needed.
// logf defines where progress messages are written.
func Start(ctx context.Context, db *database.Database, dbType string, logf func(string, ...any)) {
	const url = "https://tt.safecast.org/devices"
	const pollInterval = 5 * time.Minute

	if logf == nil {
		logf = log.Printf
	}

	// Announce poller start once so operators know interval and source.
	logf("realtime poller start: url=%s interval=%s", url, pollInterval)

	measurements := make(chan database.RealtimeMeasurement)
	reports := make(chan int)

	// DB writer goroutine.
	// It counts successes and errors per batch and logs once per report.
	go func() {
		var stored, errs int
		var lastErr error
		for {
			select {
			case <-ctx.Done():
				return
			case m := <-measurements:
				if err := db.InsertRealtimeMeasurement(m, dbType); err != nil {
					errs++
					lastErr = err
				} else {
					stored++
				}
			case n := <-reports:
				if errs > 0 {
					logf("realtime poll: devices %d stored %d errors %d last=%v next=%s", n, stored, errs, lastErr, pollInterval)
				} else {
					logf("realtime poll: devices %d stored %d next=%s", n, stored, pollInterval)
				}
				stored, errs, lastErr = 0, 0, nil
			}
		}
	}()

	// Fetcher goroutine runs once immediately and then every pollInterval.
	// Running the first fetch before waiting on the ticker gives operators
	// instant feedback after startup, embodying "Make interfaces easy to
	// use correctly".
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		// prevIDs remembers devices from prior fetch to compute add/remove counts.
		prevIDs := make(map[string]struct{})

		for {
			now := time.Now()
			nowUnix := now.Unix()
			cutoff := now.Add(-24 * time.Hour).Unix()

			data, err := fetch(ctx, url)
			if err != nil {
				logf("realtime fetch error: %v", err)
			} else {
				// Log how many devices were returned to understand coverage.
				logf("realtime fetch: devices %d", len(data))

				// Show the first payload for debugging when the map looks empty.
				if len(data) > 0 {
					d0 := data[0]
					logf("realtime sample: id=%s name=%q lat=%f lon=%f val=%f unit=%s", d0.ID, d0.Name, d0.Lat, d0.Lon, d0.Value, d0.Unit)
				}

				// Summaries are computed while forwarding measurements to DB.
				stats := make(map[string]struct {
					sum   float64
					count int
				})
				curr := make(map[string]struct{})

				for _, d := range data {
					if d.ID == "" {
						continue
					}
					converted, ok := convertIfRadiation(d)
					if !ok {
						continue
					}
					if d.Lat == 0 && d.Lon == 0 {
						continue
					}
					if d.Time == 0 {
						continue
					}

					resolvedCode, _ := countryresolver.Resolve(d.Lat, d.Lon)
					// Prefer the resolver result so map legends remain consistent even
					// when upstream payloads provide stale or incorrect hints.
					country := resolvedCode
					if country == "" {
						country = strings.ToUpper(strings.TrimSpace(d.Country))
					}
					detector := DetectorLabel(d.Tube, d.Type, d.Name)

					m := database.RealtimeMeasurement{
						DeviceID:   d.ID,
						Transport:  d.Type,
						DeviceName: d.Name,
						Tube:       detector,
						Country:    country,
						Value:      d.Value,
						Unit:       d.Unit,
						Lat:        d.Lat,
						Lon:        d.Lon,
						MeasuredAt: d.Time,
						FetchedAt:  nowUnix,
						Extra:      encodeMetrics(d.Metrics),
					}
					select {
					case <-ctx.Done():
						close(measurements)
						return
					case measurements <- m:
					}

					if d.Time < cutoff {
						// Store the sample for history but skip it in live summaries once
						// the reading is older than a day. This keeps the map clean while
						// preserving data for future charts.
						continue
					}

					curr[d.ID] = struct{}{}
					statsKey := country
					if statsKey == "" {
						statsKey = "??"
					}
					// We already converted the value once; reuse it for
					// summaries so statistics reflect what we display.
					s := stats[statsKey]
					s.sum += converted
					s.count++
					stats[statsKey] = s
				}
				reports <- len(curr)

				// Calculate device churn since last poll.
				added, removed := 0, 0
				for id := range curr {
					if _, ok := prevIDs[id]; !ok {
						added++
					}
				}
				for id := range prevIDs {
					if _, ok := curr[id]; !ok {
						removed++
					}
				}
				prevIDs = curr

				// Build country summary lines.
				parts := make([]string, 0, len(stats))
				for country, s := range stats {
					avg := s.sum / float64(s.count)
					label := country
					if name := countryresolver.NameFor(country); name != "" {
						label = fmt.Sprintf("%s (%s)", name, country)
					}
					parts = append(parts, fmt.Sprintf("%s:%d avg=%.2f", label, s.count, avg))
				}
				sort.Strings(parts)

				// Log per-country averages and churn without extra links.
				logf("realtime summary: %s added=%d removed=%d", strings.Join(parts, " "), added, removed)

				// Promote stale device histories to normal tracks once a day.
				go db.PromoteStaleRealtime(cutoff, dbType)
			}
			select {
			case <-ctx.Done():
				close(measurements)
				return
			case <-ticker.C:
			}
		}
	}()
}
