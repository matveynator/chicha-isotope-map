package jrcremrealtime

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

	"chicha-isotope-map/pkg/countryresolver"
	"chicha-isotope-map/pkg/database"
)

const (
	defaultURL     = "https://remap.jrc.ec.europa.eu/api/stations"
	pollInterval   = 5 * time.Minute
	networkTimeout = 20 * time.Second
)

type stationPayload struct {
	ID         string
	Name       string
	Lat        float64
	Lon        float64
	ValueNSvH  float64
	MeasuredAt int64
	Country    string
}

func FromRealtime(value float64, unit string) (float64, bool) {
	if value <= 0 {
		return 0, false
	}
	norm := normalizeUnit(unit)
	if norm == "" {
		return 0, false
	}
	if strings.Contains(norm, "nsv") {
		return value / 1000.0, true
	}
	if strings.Contains(norm, "usv") {
		return value, true
	}
	return 0, false
}

func normalizeUnit(unit string) string {
	replacer := strings.NewReplacer("µ", "u", "μ", "u", " ", "")
	return strings.ToLower(replacer.Replace(strings.TrimSpace(unit)))
}

func resolveCountryCode(lat, lon float64, payloadCode string) string {
	if resolved, _ := countryresolver.Resolve(lat, lon); isISOAlpha2(resolved) {
		return strings.ToUpper(strings.TrimSpace(resolved))
	}
	if isISOAlpha2(payloadCode) {
		return strings.ToUpper(strings.TrimSpace(payloadCode))
	}
	return ""
}

func isISOAlpha2(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) != 2 {
		return false
	}
	for _, r := range trimmed {
		if r < 'A' || (r > 'Z' && r < 'a') || r > 'z' {
			return false
		}
	}
	return true
}

func countryLabel(code string) string {
	if strings.TrimSpace(code) == "" {
		return "Unknown"
	}
	upper := strings.ToUpper(strings.TrimSpace(code))
	if name := countryresolver.NameFor(upper); name != "" {
		return fmt.Sprintf("%s (%s)", name, upper)
	}
	return upper
}

func normalizeCoordinates(lat, lon float64) (float64, float64, bool) {
	candidates := [][2]float64{{lat, lon}, {lon, lat}}
	for _, c := range candidates {
		if nLat, nLon, ok := normalizeCoordinatePair(c[0], c[1]); ok {
			return nLat, nLon, true
		}
	}
	return 0, 0, false
}

func normalizeCoordinatePair(lat, lon float64) (float64, float64, bool) {
	if isValidCoordinatePair(lat, lon) {
		return lat, lon, true
	}
	if math.Abs(lat) < 10000 && math.Abs(lon) < 10000 {
		return 0, 0, false
	}
	scales := []float64{10000, 100000, 1000000, 10000000}
	for _, scale := range scales {
		scaledLat := lat / scale
		scaledLon := lon / scale
		if isValidCoordinatePair(scaledLat, scaledLon) {
			return scaledLat, scaledLon, true
		}
	}
	return 0, 0, false
}

func isValidCoordinatePair(lat, lon float64) bool {
	if lat < -90 || lat > 90 {
		return false
	}
	if lon < -180 || lon > 180 {
		return false
	}
	if lat == 0 && lon == 0 {
		return false
	}
	return true
}

func Start(ctx context.Context, db *database.Database, dbType string, logf func(string, ...any)) {
	if logf == nil {
		logf = log.Printf
	}
	logf("jrc-rem poller start: url=%s interval=%s", defaultURL, pollInterval)

	measurements := make(chan database.RealtimeMeasurement)
	reports := make(chan int)

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
			case count := <-reports:
				if errs > 0 {
					logf("jrc-rem poll: sensors %d stored %d errors %d last=%v next=%s", count, stored, errs, lastErr, pollInterval)
				} else {
					logf("jrc-rem poll: sensors %d stored %d next=%s", count, stored, pollInterval)
				}
				stored, errs, lastErr = 0, 0, nil
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		// prevIDs tracks station churn between polls so operators can quickly
		// spot topology changes without scanning raw station lists.
		prevIDs := make(map[string]struct{})

		for {
			now := time.Now().UTC()
			rangeStart := now.Add(-7 * 24 * time.Hour)
			stations, err := fetch(ctx, rangeStart, now)
			if err != nil {
				logf("jrc-rem fetch error: %v", err)
			} else {
				logf("jrc-rem fetch: sensors %d", len(stations))
				nowUnix := now.Unix()
				active := 0
				stats := make(map[string]struct {
					sum   float64
					count int
				})
				curr := make(map[string]struct{})
				skippedNoTimestamp := 0
				skippedNoCoords := 0
				skippedNoRadiation := 0

				for _, s := range stations {
					if s.ID == "" || s.MeasuredAt == 0 {
						skippedNoTimestamp++
						continue
					}
					lat, lon, ok := normalizeCoordinates(s.Lat, s.Lon)
					if !ok {
						skippedNoCoords++
						continue
					}

					converted, ok := FromRealtime(s.ValueNSvH, "nSv/h")
					if !ok {
						skippedNoRadiation++
						continue
					}

					country := resolveCountryCode(lat, lon, s.Country)

					m := database.RealtimeMeasurement{
						DeviceID:   "jrc-rem:" + s.ID,
						Transport:  "jrc-rem",
						DeviceName: s.Name,
						Tube:       "JRC REM",
						Country:    country,
						Value:      s.ValueNSvH,
						Unit:       "nSv/h",
						Lat:        lat,
						Lon:        lon,
						MeasuredAt: s.MeasuredAt,
						FetchedAt:  nowUnix,
					}
					select {
					case <-ctx.Done():
						close(measurements)
						return
					case measurements <- m:
					}

					active++
					curr[m.DeviceID] = struct{}{}
					statsKey := country
					if statsKey == "" {
						statsKey = "??"
					}
					summary := stats[statsKey]
					summary.sum += converted
					summary.count++
					stats[statsKey] = summary
				}
				reports <- active

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

				parts := make([]string, 0, len(stats))
				for countryCode, summary := range stats {
					avg := summary.sum / float64(summary.count)
					label := countryLabel(countryCode)
					parts = append(parts, fmt.Sprintf("%s:%d avg=%.2f", label, summary.count, avg))
				}
				sort.Strings(parts)
				logf("jrc-rem summary: %s added=%d removed=%d skipped(timestamp=%d coords=%d value=%d)", strings.Join(parts, " "), added, removed, skippedNoTimestamp, skippedNoCoords, skippedNoRadiation)
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

func fetch(ctx context.Context, start, end time.Time) ([]stationPayload, error) {
	startDate := start.UTC().Format("20060102150405")
	endDate := end.UTC().Format("20060102150405")
	url := fmt.Sprintf("%s?type=Last&startDate=%s&endDate=%s", defaultURL, startDate, endDate)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: networkTimeout, Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 8 * time.Second}).DialContext, TLSHandshakeTimeout: 8 * time.Second}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("status %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	return decodeStations(b)
}

func decodeStations(b []byte) ([]stationPayload, error) {
	var list []map[string]any
	if err := json.Unmarshal(b, &list); err != nil {
		var wrapped map[string]any
		if err2 := json.Unmarshal(b, &wrapped); err2 != nil {
			return nil, err
		}
		for _, key := range []string{"stations", "data", "items", "results"} {
			if arr, ok := wrapped[key].([]any); ok {
				for _, raw := range arr {
					if m, ok := raw.(map[string]any); ok {
						list = append(list, m)
					}
				}
				break
			}
		}
	}

	out := make([]stationPayload, 0, len(list))
	for _, m := range list {
		s := stationPayload{}
		s.ID = firstString(m, "id", "stationId", "station_id", "code")
		s.Name = firstString(m, "name", "stationName", "station_name", "label")
		s.Country = firstString(m, "country", "countryCode", "country_code")
		s.Lat = firstFloat(m, "lat", "latitude", "Latitude", "y", "loc_lat", "Lat")
		s.Lon = firstFloat(m, "lon", "lng", "longitude", "Longitude", "x", "loc_lon", "Lon")
		s.ValueNSvH = firstFloat(m, "nsv", "nSv", "doseRate", "dose_rate", "value")
		s.MeasuredAt = parseTimestamp(firstString(m, "date", "timestamp", "time", "lastUpdate", "measuredAt"))
		if s.MeasuredAt == 0 {
			s.MeasuredAt = int64(firstFloat(m, "timestamp", "time", "measuredAt"))
		}
		if s.MeasuredAt > 1_000_000_000_000 {
			s.MeasuredAt /= 1000
		}
		if s.ID == "" && s.Name != "" {
			s.ID = s.Name
		}
		out = append(out, s)
	}
	return out, nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch x := v.(type) {
			case string:
				if strings.TrimSpace(x) != "" {
					return strings.TrimSpace(x)
				}
			case float64:
				return strconv.FormatInt(int64(x), 10)
			}
		}
	}
	return ""
}

func firstFloat(m map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch x := v.(type) {
			case float64:
				return x
			case string:
				f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
				if err == nil {
					return f
				}
			}
		}
	}
	return 0
}

func parseTimestamp(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}

	// JRC often uses compact UTC datetime like 20260218102644.
	// Parse this shape before Unix parsing so we avoid misreading it as
	// milliseconds and accidentally aging fresh rows into 1970-era timestamps.
	if len(raw) == 14 {
		allDigits := true
		for _, r := range raw {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			if t, err := time.Parse("20060102150405", raw); err == nil {
				return t.UTC().Unix()
			}
		}
	}

	if ts, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if ts > 1_000_000_000_000 {
			return ts / 1000
		}
		return ts
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "20060102150405"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC().Unix()
		}
	}
	return 0
}
