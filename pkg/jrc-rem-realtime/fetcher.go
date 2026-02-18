package jrcremrealtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
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
				for _, s := range stations {
					if s.ID == "" || s.MeasuredAt == 0 {
						continue
					}
					if s.Lat == 0 && s.Lon == 0 {
						continue
					}
					if _, ok := FromRealtime(s.ValueNSvH, "nSv/h"); !ok {
						continue
					}

					country := strings.ToUpper(strings.TrimSpace(s.Country))
					if resolved, _ := countryresolver.Resolve(s.Lat, s.Lon); resolved != "" {
						country = resolved
					}

					m := database.RealtimeMeasurement{
						DeviceID:   "jrc-rem:" + s.ID,
						Transport:  "jrc-rem",
						DeviceName: s.Name,
						Tube:       "JRC REM",
						Country:    country,
						Value:      s.ValueNSvH,
						Unit:       "nSv/h",
						Lat:        s.Lat,
						Lon:        s.Lon,
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
				}
				reports <- active
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
		s.Lat = firstFloat(m, "lat", "latitude", "Latitude")
		s.Lon = firstFloat(m, "lon", "lng", "longitude", "Longitude")
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
