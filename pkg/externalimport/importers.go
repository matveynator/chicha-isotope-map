package externalimport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
	"chicha-isotope-map/pkg/trackjson"
)

// Logger mirrors the safecast importer logging helper so callers can keep
// dependency injection lightweight.
type Logger func(string, ...any)

// StoreFunc matches processAndStoreMarkers from main.go.
type StoreFunc func([]database.Marker, string, *database.Database, string, string, string) (database.Bounds, string, error)

// StartAtomFast launches an hourly poller that fetches AtomFast cloud data.
func StartAtomFast(ctx context.Context, db *database.Database, dbType, endpoint string, store StoreFunc, logf Logger) {
	origin := originURLFromEndpoint(endpoint, "/maps/")
	startTimedPoller(ctx, db, dbType, store, logf, endpoint, "atomfast", "AtomFast Cloud", origin, fetchAtomFast)
}

// StartRadiaverse launches an hourly poller that fetches Radiaverse measurements.
func StartRadiaverse(ctx context.Context, db *database.Database, dbType, endpoint string, store StoreFunc, logf Logger) {
	origin := originURLFromEndpoint(endpoint, "/")
	startTimedPoller(ctx, db, dbType, store, logf, endpoint, "radiaverse", "Radiacode via radiaverse.com", origin, fetchRadiaverse)
}

// StartChichaMirror mirrors tracks from another chicha instance.
func StartChichaMirror(ctx context.Context, baseURL string, db *database.Database, dbType string, store StoreFunc, logf Logger) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return
	}
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		trimmed = "https://" + trimmed
	}
	startMirrorPoller(ctx, trimmed, db, dbType, store, logf)
}

type fetchFunc func(context.Context, *http.Client, time.Time, time.Time, string, Logger) ([]database.Marker, string, time.Time, error)

func startTimedPoller(ctx context.Context, db *database.Database, dbType string, store StoreFunc, logf Logger, endpoint, cursorKey, sourceLabel, sourceURL string, fetch fetchFunc) {
	if ctx == nil || db == nil || store == nil {
		return
	}
	client := &http.Client{Timeout: 45 * time.Second}
	jobs := make(chan time.Time, 1)
	go schedule(ctx, jobs)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ts, ok := <-jobs:
				if !ok {
					return
				}
				runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
				if err := runTimedPoller(runCtx, ts, db, dbType, store, client, endpoint, cursorKey, sourceLabel, sourceURL, fetch, logf); err != nil {
					if logf != nil {
						logf("%s sync failed: %v", sourceLabel, err)
					}
				}
				cancel()
			}
		}
	}()
}

func schedule(ctx context.Context, jobs chan<- time.Time) {
	defer close(jobs)
	now := time.Now().UTC()
	select {
	case <-ctx.Done():
		return
	case jobs <- now:
	}
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			select {
			case jobs <- t.UTC():
			default:
			}
		}
	}
}

func runTimedPoller(ctx context.Context, ts time.Time, db *database.Database, dbType string, store StoreFunc, client *http.Client, endpoint, cursorKey, sourceLabel, sourceURL string, fetch fetchFunc, logf Logger) error {
	since, err := db.ImportCursor(ctx, dbType, cursorKey)
	if err != nil {
		return fmt.Errorf("load %s cursor: %w", cursorKey, err)
	}
	if since.IsZero() {
		since = ts.Add(-24 * time.Hour)
	} else {
		since = since.Add(-5 * time.Minute)
	}
	until := ts
	if !until.After(since) {
		until = since.Add(time.Hour)
	}
	markers, trackID, cursor, err := fetch(ctx, client, since, until, endpoint, logf)
	if err != nil {
		return err
	}
	if len(markers) == 0 {
		if logf != nil {
			logf("%s sync: no new measurements", sourceLabel)
		}
		if err := db.UpdateImportCursor(ctx, dbType, cursorKey, until); err != nil {
			return fmt.Errorf("update %s cursor: %w", cursorKey, err)
		}
		return nil
	}
	if trackID == "" {
		trackID = fmt.Sprintf("%s-%d", cursorKey, ts.Unix())
	}
	_, storedTrackID, err := store(markers, trackID, db, dbType, sourceLabel, sourceURL)
	if err != nil {
		return fmt.Errorf("store %s markers: %w", sourceLabel, err)
	}
	if logf != nil {
		logf("%s sync: stored track %s (%d markers)", sourceLabel, storedTrackID, len(markers))
	}
	if cursor.IsZero() {
		cursor = until
	}
	if err := db.UpdateImportCursor(ctx, dbType, cursorKey, cursor); err != nil {
		return fmt.Errorf("update %s cursor: %w", cursorKey, err)
	}
	return nil
}

func fetchAtomFast(ctx context.Context, client *http.Client, since, until time.Time, endpoint string, logf Logger) ([]database.Marker, string, time.Time, error) {
	urlWithRange, err := attachRange(endpoint, since, until)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlWithRange, nil)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	req.Header.Set("User-Agent", "chicha-isotope-map atomfast importer")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("atomfast request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, "", time.Time{}, fmt.Errorf("atomfast http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("atomfast read: %w", err)
	}
	var payload []struct {
		Dose      float64 `json:"d"`
		DoseAlt   float64 `json:"doseRate"`
		Lat       float64 `json:"lat"`
		Lon       float64 `json:"lon"`
		Lng       float64 `json:"lng"`
		TimeMS    int64   `json:"t"`
		TimeS     int64   `json:"time"`
		Timestamp string  `json:"timestamp"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, "", time.Time{}, fmt.Errorf("atomfast decode: %w", err)
	}
	markers := make([]database.Marker, 0, len(payload))
	latest := since
	for _, item := range payload {
		dose := item.Dose
		if dose == 0 {
			dose = item.DoseAlt
		}
		lat := item.Lat
		if lat == 0 {
			lat = item.Lon // fallback for swapped keys
		}
		lon := item.Lon
		if lon == 0 {
			lon = item.Lng
		}
		ts := normalizeTimestamp(item.TimeMS, item.TimeS, item.Timestamp)
		if ts <= 0 {
			continue
		}
		marker := database.Marker{
			DoseRate:  dose,
			CountRate: dose,
			Lat:       lat,
			Lon:       lon,
			Date:      ts,
			Detector:  "AtomFast",
			Radiation: "gamma",
		}
		markers = append(markers, marker)
		if t := time.Unix(ts, 0).UTC(); t.After(latest) {
			latest = t
		}
	}
	sort.Slice(markers, func(i, j int) bool { return markers[i].Date < markers[j].Date })
	return markers, fmt.Sprintf("AF-%d", until.Unix()), latest, nil
}

func fetchRadiaverse(ctx context.Context, client *http.Client, since, until time.Time, endpoint string, logf Logger) ([]database.Marker, string, time.Time, error) {
	urlWithRange, err := attachRange(endpoint, since, until)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlWithRange, nil)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	req.Header.Set("User-Agent", "chicha-isotope-map radiaverse importer")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("radiaverse request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, "", time.Time{}, fmt.Errorf("radiaverse http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("radiaverse read: %w", err)
	}
	var payload []struct {
		DoseMicroSv float64 `json:"doseRateMicroSvH"`
		DoseMicroRh float64 `json:"doseRateMicroRh"`
		Lat         float64 `json:"lat"`
		Lon         float64 `json:"lon"`
		Detector    string  `json:"detectorType"`
		Device      string  `json:"detectorName"`
		Timestamp   string  `json:"timeUTC"`
		TimeUnix    int64   `json:"timeUnix"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, "", time.Time{}, fmt.Errorf("radiaverse decode: %w", err)
	}
	markers := make([]database.Marker, 0, len(payload))
	latest := since
	for _, item := range payload {
		dose := item.DoseMicroSv
		if dose == 0 && item.DoseMicroRh != 0 {
			dose = item.DoseMicroRh / 100.0
		}
		ts := item.TimeUnix
		if ts <= 0 && strings.TrimSpace(item.Timestamp) != "" {
			if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(item.Timestamp)); err == nil {
				ts = parsed.Unix()
			}
		}
		if ts <= 0 {
			continue
		}
		detector := strings.TrimSpace(item.Detector)
		if detector == "" {
			detector = strings.TrimSpace(item.Device)
		}
		name := classifyRadiacode(detector)
		marker := database.Marker{
			DoseRate:         dose,
			CountRate:        dose * 60,
			Lat:              item.Lat,
			Lon:              item.Lon,
			Date:             ts,
			Detector:         name,
			Radiation:        "gamma",
			AltitudeValid:    false,
			TemperatureValid: false,
			HumidityValid:    false,
		}
		markers = append(markers, marker)
		if t := time.Unix(ts, 0).UTC(); t.After(latest) {
			latest = t
		}
	}
	sort.Slice(markers, func(i, j int) bool { return markers[i].Date < markers[j].Date })
	return markers, fmt.Sprintf("RV-%d", until.Unix()), latest, nil
}

// classifyRadiacode normalises detector strings so downstream exports mention
// Radiacode devices explicitly. Many payloads spell the model differently, so
// we collapse common prefixes while keeping unknown names untouched.
func classifyRadiacode(detector string) string {
	trimmed := strings.TrimSpace(detector)
	if trimmed == "" {
		return "Radiacode"
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "radiacode"):
		return "Radiacode"
	case strings.HasPrefix(lower, "rc-"):
		return "Radiacode " + strings.ToUpper(strings.TrimSpace(trimmed))
	}
	return trimmed
}

func attachRange(endpoint string, since, until time.Time) (string, error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return "", fmt.Errorf("endpoint required")
	}
	base, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	q := base.Query()
	q.Set("since", since.UTC().Format(time.RFC3339))
	q.Set("until", until.UTC().Format(time.RFC3339))
	base.RawQuery = q.Encode()
	return base.String(), nil
}

func normalizeTimestamp(ms, seconds int64, timestamp string) int64 {
	switch {
	case ms > 0:
		return ms / 1000
	case seconds > 0:
		return seconds
	case strings.TrimSpace(timestamp) != "":
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(timestamp)); err == nil {
			return parsed.Unix()
		}
	}
	return 0
}

// originURLFromEndpoint maps an API endpoint to a human friendly landing page
// so stored markers point readers at the original dataset. We strip query
// strings and optionally override the path to highlight the public map.
func originURLFromEndpoint(endpoint, fallbackPath string) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	path := strings.TrimSpace(fallbackPath)
	if path != "" {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		parsed.Path = path
	} else if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String()
}

func startMirrorPoller(ctx context.Context, baseURL string, db *database.Database, dbType string, store StoreFunc, logf Logger) {
	if ctx == nil || db == nil || store == nil {
		return
	}
	jobs := make(chan time.Time, 1)
	client := &http.Client{Timeout: 60 * time.Second}
	go schedule(ctx, jobs)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ts, ok := <-jobs:
				if !ok {
					return
				}
				runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
				if err := mirrorOnce(runCtx, baseURL, ts, db, dbType, store, client, logf); err != nil && logf != nil {
					logf("mirror %s failed: %v", baseURL, err)
				}
				cancel()
			}
		}
	}()
}

func mirrorOnce(ctx context.Context, baseURL string, ts time.Time, db *database.Database, dbType string, store StoreFunc, client *http.Client, logf Logger) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tracks", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "chicha-isotope-map mirror importer")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("mirror list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("mirror list http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("mirror list read: %w", err)
	}
	var payload struct {
		Tracks []struct {
			TrackID string `json:"trackID"`
			APIURL  string `json:"apiURL"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("mirror list decode: %w", err)
	}
	if len(payload.Tracks) == 0 {
		return nil
	}
	imported := 0
	host := baseURL
	if parsed, err := url.Parse(baseURL); err == nil {
		host = parsed.Host
	}
	for _, summary := range payload.Tracks {
		if imported >= 5 {
			break
		}
		id := strings.TrimSpace(summary.TrackID)
		if id == "" {
			continue
		}
		existing, err := db.GetTrackSummary(ctx, id, dbType)
		if err != nil {
			return fmt.Errorf("mirror track summary %s: %w", id, err)
		}
		if existing.MarkerCount > 0 {
			continue
		}
		trackURL := summary.APIURL
		if strings.TrimSpace(trackURL) == "" {
			trackURL = fmt.Sprintf("/api/track/%s.cim", url.PathEscape(id))
		}
		fullURL := trackURL
		if !strings.HasPrefix(trackURL, "http://") && !strings.HasPrefix(trackURL, "https://") {
			fullURL = strings.TrimRight(baseURL, "/") + trackURL
		}
		reqTrack, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
		if err != nil {
			return fmt.Errorf("mirror track request %s: %w", id, err)
		}
		reqTrack.Header.Set("User-Agent", "chicha-isotope-map mirror importer")
		respTrack, err := client.Do(reqTrack)
		if err != nil {
			return fmt.Errorf("mirror track %s: %w", id, err)
		}
		body, err := io.ReadAll(respTrack.Body)
		_ = respTrack.Body.Close()
		if err != nil {
			return fmt.Errorf("mirror track %s read: %w", id, err)
		}
		if respTrack.StatusCode >= 400 {
			return fmt.Errorf("mirror track %s http %d", id, respTrack.StatusCode)
		}
		parsedID, markers, err := trackjson.DecodeTrackJSON(body)
		if err != nil {
			if logf != nil {
				logf("mirror %s: decode %s failed: %v", host, id, err)
			}
			continue
		}
		if len(markers) == 0 {
			continue
		}
		if parsedID == "" {
			parsedID = id
		}
		_, storedID, err := store(markers, parsedID, db, dbType, "mirror:"+host, fullURL)
		if err != nil {
			return fmt.Errorf("mirror track %s store: %w", id, err)
		}
		imported++
		if logf != nil {
			logf("mirror %s: imported %s", host, storedID)
		}
	}
	if err := db.UpdateImportCursor(ctx, dbType, "mirror:"+host, ts); err != nil {
		return fmt.Errorf("mirror cursor: %w", err)
	}
	return nil
}
