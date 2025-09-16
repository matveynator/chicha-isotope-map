package safecastrealtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

// metricKeyFor recognises temperature and humidity hints from the payload map.
// Returning a normalised key allows us to keep the stored JSON compact and easy
// to translate in the UI later.
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
	return "", false
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
		d.Type = v
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

	// Measurement value: prioritise keys ending with "u" which Safecast
	// uses for micro roentgen per hour in centi-units (53 → 0.53 µSv/h).
	// We search twice: first for these unit-suffixed fields, then as a
	// fallback for any remaining "lnd_" entry. This avoids picking CPM
	// fields by accident, echoing "Clear is better than clever".

	// pass 1: look for "lnd_*u" values
	for k, v := range m {
		if strings.HasPrefix(k, "lnd_") && strings.HasSuffix(k, "u") {
			if fv, ok := v.(float64); ok {
				d.Value = fv / 100.0
				d.Unit = "µSv/h"
				goto gotValue
			}
		}
	}
	// pass 2: any other "lnd_" field (counts, raw units)
	for k, v := range m {
		if strings.HasPrefix(k, "lnd_") {
			if fv, ok := v.(float64); ok {
				d.Value = fv
				d.Unit = k
				goto gotValue
			}
		}
	}
gotValue:

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
