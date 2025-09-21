package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
	"chicha-isotope-map/pkg/kmlarchive"
)

// =======================
// Public API entry points
// =======================

// Handler wires together the database and archive generator so HTTP routes
// can stay small and focused on translating query parameters into the
// asynchronous building blocks behind the scenes.
type Handler struct {
	DB      *database.Database
	DBType  string
	Archive *kmlarchive.Generator
	Logf    func(string, ...any)
}

// NewHandler constructs a Handler with sane defaults.
// Logf is optional; pass nil if logging is not required.
func NewHandler(db *database.Database, dbType string, archive *kmlarchive.Generator, logf func(string, ...any)) *Handler {
	return &Handler{DB: db, DBType: dbType, Archive: archive, Logf: logf}
}

// Register attaches API routes to the provided mux. We keep the method tiny
// and declarative: it simply wires URLs to helpers, avoiding clever routing
// that could obscure how pages are served.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api", h.handleOverview)
	mux.HandleFunc("/api/tracks", h.handleTracksList)
	mux.HandleFunc("/api/tracks/index/", h.handleTrackDataByIndex)
	mux.HandleFunc("/api/tracks/years/", h.handleTracksByYear)
	mux.HandleFunc("/api/tracks/months/", h.handleTracksByMonth)
	mux.HandleFunc("/api/tracks/", h.handleTrackData)
	mux.HandleFunc("/api/kml/daily.tar.gz", h.handleArchiveDownload)
}

// handleOverview publishes machine-readable docs so developers understand
// which endpoints to call and how to iterate through data sets.
func (h *Handler) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	totalTracks, latestTrackID, err := h.latestTrackInfo(ctx)
	if err != nil {
		http.Error(w, "count tracks", http.StatusInternalServerError)
		return
	}

	overview := struct {
		Disclaimers      map[string]string `json:"disclaimers"`
		Endpoints        map[string]any    `json:"endpoints"`
		TotalTracks      int64             `json:"totalTracks"`
		LatestTrackIndex int64             `json:"latestTrackIndex"`
		LatestTrackID    string            `json:"latestTrackID,omitempty"`
	}{
		Disclaimers:      disclaimerTexts,
		TotalTracks:      totalTracks,
		LatestTrackIndex: totalTracks,
		LatestTrackID:    latestTrackID,
		Endpoints: map[string]any{
			"listTracks": map[string]any{
				"method":      "GET",
				"path":        "/api/tracks",
				"query":       []string{"startAfter", "limit"},
				"description": "Returns track summaries sorted alphabetically. Each summary exposes an index and apiURL. Use nextStartAfter to continue pagination.",
			},
			"trackByNumber": map[string]any{
				"method":      "GET",
				"path":        "/api/tracks/index/{number}",
				"description": "Resolves a track by its 1-based numeric index and streams markers just like /api/tracks/{trackID}.",
			},
			"trackMarkers": map[string]any{
				"method":      "GET",
				"path":        "/api/tracks/{trackID}",
				"query":       []string{"from", "limit", "to"},
				"description": "Streams markers for the requested track. Provide 'from' using the firstID from summaries and increment using nextFromID.",
			},
			"tracksByYear": map[string]any{
				"method":      "GET",
				"path":        "/api/tracks/years/{year}",
				"query":       []string{"startAfter", "limit"},
				"description": "Lists tracks that contain markers within the given year. Pagination mirrors /api/tracks and keeps global indices.",
			},
			"tracksByMonth": map[string]any{
				"method":      "GET",
				"path":        "/api/tracks/months/{year}/{month}",
				"query":       []string{"startAfter", "limit"},
				"description": "Lists tracks for a calendar month using the same pagination fields.",
			},
			"dailyKML": map[string]any{
				"method":      "GET",
				"path":        "/api/kml/daily.tar.gz",
				"description": "Downloads the current tar.gz bundle of all published KML files.",
				"frequency":   "Updated once per day",
			},
		},
	}

	h.respondJSON(w, overview)
}

// handleTracksList exposes paginated track summaries.
func (h *Handler) handleTracksList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	startAfter := q.Get("startAfter")
	limit := clampInt(parseIntDefault(q.Get("limit"), 100), 1, 1000)

	tracksCh, errCh := h.DB.StreamTrackSummaries(ctx, startAfter, limit, h.DBType)

	summaries, lastTrackID, err := collectTrackSummaries(ctx, tracksCh, errCh, limit)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
			return
		}
		http.Error(w, "track list error", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("track list error: %v", err)
		}
		return
	}

	startIndex, err := h.finalizeSummaries(ctx, startAfter, summaries)
	if err != nil {
		http.Error(w, "track index error", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("track index error: %v", err)
		}
		return
	}

	next := ""
	if len(summaries) == limit && lastTrackID != "" {
		next = lastTrackID
	}

	totalTracks, latestTrackID, err := h.latestTrackInfo(ctx)
	if err != nil {
		http.Error(w, "count tracks", http.StatusInternalServerError)
		return
	}

	resp := struct {
		StartAfter     string                  `json:"startAfter"`
		Limit          int                     `json:"limit"`
		StartIndex     int64                   `json:"startIndex,omitempty"`
		Tracks         []database.TrackSummary `json:"tracks"`
		NextStartAfter string                  `json:"nextStartAfter,omitempty"`
		TotalTracks    int64                   `json:"totalTracks"`
		LatestTrackID  string                  `json:"latestTrackID,omitempty"`
		Disclaimers    map[string]string       `json:"disclaimers"`
	}{
		StartAfter:     startAfter,
		Limit:          limit,
		StartIndex:     startIndex,
		Tracks:         summaries,
		NextStartAfter: next,
		TotalTracks:    totalTracks,
		LatestTrackID:  latestTrackID,
		Disclaimers:    disclaimerTexts,
	}

	h.respondJSON(w, resp)
}

// handleTrackData streams markers from a single track using ID ranges.
func (h *Handler) handleTrackData(w http.ResponseWriter, r *http.Request) {
	trackID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tracks/"), "/")
	if trackID == "" {
		http.NotFound(w, r)
		return
	}
	h.serveTrackData(w, r, trackID)
}

// handleTrackDataByIndex resolves a numeric track number and reuses the
// standard track handler so consumers can iterate sequentially.
func (h *Handler) handleTrackDataByIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	raw := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tracks/index/"), "/")
	if raw == "" {
		http.NotFound(w, r)
		return
	}

	index, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || index <= 0 {
		http.Error(w, "invalid track index", http.StatusBadRequest)
		return
	}

	trackID, err := h.DB.GetTrackIDByIndex(ctx, index, h.DBType)
	if err != nil {
		http.Error(w, "resolve track index", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("resolve track index %d: %v", index, err)
		}
		return
	}
	if trackID == "" {
		http.NotFound(w, r)
		return
	}

	h.serveTrackData(w, r, trackID)
}

// handleTracksByYear lists tracks that contain markers within a specific year.
func (h *Handler) handleTracksByYear(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tracks/years/"), "/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}

	year, err := strconv.Atoi(trimmed)
	if err != nil || year <= 0 {
		http.Error(w, "invalid year", http.StatusBadRequest)
		return
	}

	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(1, 0, 0)

	q := r.URL.Query()
	startAfter := q.Get("startAfter")
	limit := clampInt(parseIntDefault(q.Get("limit"), 100), 1, 1000)

	tracksCh, errCh := h.DB.StreamTrackSummariesByDateRange(ctx, startAfter, limit, start.Unix(), end.Unix(), h.DBType)

	summaries, lastTrackID, streamErr := collectTrackSummaries(ctx, tracksCh, errCh, limit)
	if streamErr != nil {
		if errors.Is(streamErr, context.Canceled) || errors.Is(streamErr, context.DeadlineExceeded) {
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
			return
		}
		http.Error(w, "track list error", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("tracks by year %d: %v", year, streamErr)
		}
		return
	}

	startIndex, err := h.finalizeSummaries(ctx, startAfter, summaries)
	if err != nil {
		http.Error(w, "track index error", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("tracks by year %d index error: %v", year, err)
		}
		return
	}

	next := ""
	if len(summaries) == limit && lastTrackID != "" {
		next = lastTrackID
	}

	rangeTotal, err := h.DB.CountTracksInRange(ctx, start.Unix(), end.Unix(), h.DBType)
	if err != nil {
		http.Error(w, "count tracks", http.StatusInternalServerError)
		return
	}

	totalTracks, latestTrackID, err := h.latestTrackInfo(ctx)
	if err != nil {
		http.Error(w, "count tracks", http.StatusInternalServerError)
		return
	}

	resp := struct {
		Year           int                     `json:"year"`
		RangeStart     int64                   `json:"rangeStart"`
		RangeEnd       int64                   `json:"rangeEnd"`
		StartAfter     string                  `json:"startAfter"`
		StartIndex     int64                   `json:"startIndex,omitempty"`
		Limit          int                     `json:"limit"`
		Tracks         []database.TrackSummary `json:"tracks"`
		NextStartAfter string                  `json:"nextStartAfter,omitempty"`
		RangeTotal     int64                   `json:"rangeTotal"`
		TotalTracks    int64                   `json:"totalTracks"`
		LatestTrackID  string                  `json:"latestTrackID,omitempty"`
		Disclaimers    map[string]string       `json:"disclaimers"`
	}{
		Year:           year,
		RangeStart:     start.Unix(),
		RangeEnd:       end.Unix(),
		StartAfter:     startAfter,
		StartIndex:     startIndex,
		Limit:          limit,
		Tracks:         summaries,
		NextStartAfter: next,
		RangeTotal:     rangeTotal,
		TotalTracks:    totalTracks,
		LatestTrackID:  latestTrackID,
		Disclaimers:    disclaimerTexts,
	}

	h.respondJSON(w, resp)
}

// handleTracksByMonth narrows track summaries to a calendar month.
func (h *Handler) handleTracksByMonth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tracks/months/"), "/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		http.Error(w, "invalid month path", http.StatusBadRequest)
		return
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil || year <= 0 {
		http.Error(w, "invalid year", http.StatusBadRequest)
		return
	}
	month, err := strconv.Atoi(parts[1])
	if err != nil || month < 1 || month > 12 {
		http.Error(w, "invalid month", http.StatusBadRequest)
		return
	}

	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	q := r.URL.Query()
	startAfter := q.Get("startAfter")
	limit := clampInt(parseIntDefault(q.Get("limit"), 100), 1, 1000)

	tracksCh, errCh := h.DB.StreamTrackSummariesByDateRange(ctx, startAfter, limit, start.Unix(), end.Unix(), h.DBType)

	summaries, lastTrackID, streamErr := collectTrackSummaries(ctx, tracksCh, errCh, limit)
	if streamErr != nil {
		if errors.Is(streamErr, context.Canceled) || errors.Is(streamErr, context.DeadlineExceeded) {
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
			return
		}
		http.Error(w, "track list error", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("tracks by month %04d-%02d: %v", year, month, streamErr)
		}
		return
	}

	startIndex, err := h.finalizeSummaries(ctx, startAfter, summaries)
	if err != nil {
		http.Error(w, "track index error", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("tracks by month %04d-%02d index error: %v", year, month, err)
		}
		return
	}

	next := ""
	if len(summaries) == limit && lastTrackID != "" {
		next = lastTrackID
	}

	rangeTotal, err := h.DB.CountTracksInRange(ctx, start.Unix(), end.Unix(), h.DBType)
	if err != nil {
		http.Error(w, "count tracks", http.StatusInternalServerError)
		return
	}

	totalTracks, latestTrackID, err := h.latestTrackInfo(ctx)
	if err != nil {
		http.Error(w, "count tracks", http.StatusInternalServerError)
		return
	}

	resp := struct {
		Year           int                     `json:"year"`
		Month          int                     `json:"month"`
		RangeStart     int64                   `json:"rangeStart"`
		RangeEnd       int64                   `json:"rangeEnd"`
		StartAfter     string                  `json:"startAfter"`
		StartIndex     int64                   `json:"startIndex,omitempty"`
		Limit          int                     `json:"limit"`
		Tracks         []database.TrackSummary `json:"tracks"`
		NextStartAfter string                  `json:"nextStartAfter,omitempty"`
		RangeTotal     int64                   `json:"rangeTotal"`
		TotalTracks    int64                   `json:"totalTracks"`
		LatestTrackID  string                  `json:"latestTrackID,omitempty"`
		Disclaimers    map[string]string       `json:"disclaimers"`
	}{
		Year:           year,
		Month:          month,
		RangeStart:     start.Unix(),
		RangeEnd:       end.Unix(),
		StartAfter:     startAfter,
		StartIndex:     startIndex,
		Limit:          limit,
		Tracks:         summaries,
		NextStartAfter: next,
		RangeTotal:     rangeTotal,
		TotalTracks:    totalTracks,
		LatestTrackID:  latestTrackID,
		Disclaimers:    disclaimerTexts,
	}

	h.respondJSON(w, resp)
}

// serveTrackData centralises the marker streaming logic so both ID-based and
// index-based handlers produce identical responses.
type trackMarkerPayload struct {
	ID               int64    `json:"id"`
	TimeUnix         int64    `json:"timeUnix"`
	TimeUTC          string   `json:"timeUTC"`
	Lat              float64  `json:"lat"`
	Lon              float64  `json:"lon"`
	AltitudeM        float64  `json:"altitudeM"`
	DoseRateMicroSvH float64  `json:"doseRateMicroSvH"`
	DoseRateMilliSvH float64  `json:"doseRateMilliSvH"`
	DoseRateMilliRH  float64  `json:"doseRateMilliRH"`
	CountRateCPS     float64  `json:"countRateCPS"`
	SpeedMS          float64  `json:"speedMS"`
	SpeedKMH         float64  `json:"speedKMH"`
	TemperatureC     float64  `json:"temperatureC"`
	HumidityPercent  float64  `json:"humidityPercent"`
	DetectorType     string   `json:"detectorType,omitempty"`
	RadiationTypes   []string `json:"radiationTypes,omitempty"`
}

func (h *Handler) serveTrackData(w http.ResponseWriter, r *http.Request, trackID string) {
	ctx := r.Context()

	summary, err := h.DB.GetTrackSummary(ctx, trackID, h.DBType)
	if err != nil {
		http.Error(w, "summary error", http.StatusInternalServerError)
		return
	}
	if summary.MarkerCount == 0 {
		http.Error(w, "track not found", http.StatusNotFound)
		return
	}

	trackIndex, err := h.DB.CountTrackIDsUpTo(ctx, trackID, h.DBType)
	if err != nil {
		http.Error(w, "track index error", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("track %s index error: %v", trackID, err)
		}
		return
	}

	q := r.URL.Query()
	from := parseInt64Default(q.Get("from"), summary.FirstID)
	if from < summary.FirstID {
		from = summary.FirstID
	}
	limit := clampInt(parseIntDefault(q.Get("limit"), 1000), 1, 5000)
	to := parseInt64Default(q.Get("to"), summary.LastID)
	if to > summary.LastID {
		to = summary.LastID
	}

	markersCh, errCh := h.DB.StreamMarkersByTrackRange(ctx, trackID, from, to, limit, h.DBType)

	markers := make([]database.Marker, 0, limit)
	var lastID int64

	for markersCh != nil {
		select {
		case <-ctx.Done():
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
			return
		case marker, ok := <-markersCh:
			if !ok {
				markersCh = nil
				continue
			}
			markers = append(markers, marker)
			lastID = marker.ID
		}
	}

	if err := <-errCh; err != nil {
		http.Error(w, "markers error", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("track %s markers error: %v", trackID, err)
		}
		return
	}

	nextFrom := int64(0)
	if lastID > 0 && lastID < summary.LastID {
		nextFrom = lastID + 1
	}

	payload := make([]trackMarkerPayload, 0, len(markers))
	for _, marker := range markers {
		ts, unixSeconds := normalizeMarkerTime(marker.Date)
		detector := strings.TrimSpace(marker.Detector)
		pm := trackMarkerPayload{
			ID:               marker.ID,
			TimeUnix:         unixSeconds,
			TimeUTC:          ts.Format(time.RFC3339),
			Lat:              marker.Lat,
			Lon:              marker.Lon,
			AltitudeM:        marker.Altitude,
			DoseRateMicroSvH: marker.DoseRate,
			DoseRateMilliSvH: marker.DoseRate / 1000.0,
			DoseRateMilliRH:  marker.DoseRate / 10.0,
			CountRateCPS:     marker.CountRate,
			SpeedMS:          marker.Speed,
			SpeedKMH:         marker.Speed * 3.6,
			TemperatureC:     marker.Temperature,
			HumidityPercent:  marker.Humidity,
		}
		if detector != "" {
			pm.DetectorType = detector
		}
		if channels := splitRadiationChannels(marker.Radiation); len(channels) > 0 {
			pm.RadiationTypes = channels
		}
		payload = append(payload, pm)
	}

	resp := struct {
		TrackID     string               `json:"trackID"`
		TrackIndex  int64                `json:"trackIndex"`
		APIURL      string               `json:"apiURL"`
		From        int64                `json:"from"`
		To          int64                `json:"to"`
		Limit       int                  `json:"limit"`
		NextFrom    int64                `json:"nextFrom,omitempty"`
		LastID      int64                `json:"lastID"`
		Markers     []trackMarkerPayload `json:"markers"`
		Disclaimers map[string]string    `json:"disclaimers"`
	}{
		TrackID:     trackID,
		TrackIndex:  trackIndex,
		APIURL:      h.trackAPIURL(trackID),
		From:        from,
		To:          to,
		Limit:       limit,
		NextFrom:    nextFrom,
		LastID:      summary.LastID,
		Markers:     payload,
		Disclaimers: disclaimerTexts,
	}

	h.respondJSON(w, resp)
}

// handleArchiveDownload streams the daily tar.gz produced by the generator.
func (h *Handler) handleArchiveDownload(w http.ResponseWriter, r *http.Request) {
	if h.Archive == nil {
		http.Error(w, "archive disabled", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	info, err := h.Archive.Fetch(ctx)
	if err != nil {
		http.Error(w, "archive unavailable", http.StatusServiceUnavailable)
		if h.Logf != nil {
			h.Logf("archive fetch error: %v", err)
		}
		return
	}

	file, err := os.Open(info.Path)
	if err != nil {
		http.Error(w, "archive open error", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "archive stat error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(info.Path)))
	http.ServeContent(w, r, filepath.Base(info.Path), stat.ModTime(), file)
}

// =====================
// Utility helpers
// =====================

// collectTrackSummaries drains the streaming channel into a slice so handlers
// can annotate the results before responding. Returning the last TrackID keeps
// pagination logic straightforward.
func collectTrackSummaries(
	ctx context.Context,
	stream <-chan database.TrackSummary,
	errCh <-chan error,
	limit int,
) ([]database.TrackSummary, string, error) {
	summaries := make([]database.TrackSummary, 0, limit)
	var lastTrackID string

	for stream != nil {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case summary, ok := <-stream:
			if !ok {
				stream = nil
				continue
			}
			summaries = append(summaries, summary)
			lastTrackID = summary.TrackID
		}
	}

	if err := <-errCh; err != nil {
		return nil, "", err
	}

	return summaries, lastTrackID, nil
}

// finalizeSummaries attaches indices and API URLs so responses remain self-
// descriptive. We compute the base index lazily to avoid extra SQL calls when
// no rows were returned.
func (h *Handler) finalizeSummaries(
	ctx context.Context,
	startAfter string,
	summaries []database.TrackSummary,
) (int64, error) {
	if len(summaries) == 0 {
		return 0, nil
	}

	var base int64
	var err error

	if trimmed := strings.TrimSpace(startAfter); trimmed != "" {
		base, err = h.DB.CountTrackIDsUpTo(ctx, trimmed, h.DBType)
		if err != nil {
			return 0, err
		}
	} else {
		base, err = h.DB.CountTrackIDsUpTo(ctx, summaries[0].TrackID, h.DBType)
		if err != nil {
			return 0, err
		}
		base--
		if base < 0 {
			base = 0
		}
	}

	for i := range summaries {
		idx := base + int64(i) + 1
		summaries[i].Index = idx
		summaries[i].APIURL = h.trackAPIURL(summaries[i].TrackID)
	}

	return summaries[0].Index, nil
}

// latestTrackInfo reports the highest known track index and its ID so API
// callers know when they reached the end of the catalogue.
func (h *Handler) latestTrackInfo(ctx context.Context) (int64, string, error) {
	total, err := h.DB.CountTracks(ctx)
	if err != nil {
		return 0, "", err
	}
	if total == 0 {
		return 0, "", nil
	}

	trackID, err := h.DB.GetTrackIDByIndex(ctx, total, h.DBType)
	if err != nil {
		return 0, "", err
	}
	return total, trackID, nil
}

// trackAPIURL builds the canonical API link for a track, escaping the ID so it
// remains safe even with unusual characters.
func (h *Handler) trackAPIURL(trackID string) string {
	return "/api/tracks/" + url.PathEscape(trackID)
}

func (h *Handler) respondJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func normalizeMarkerTime(unixValue int64) (time.Time, int64) {
	if unixValue <= 0 {
		ts := time.Unix(0, 0).UTC()
		return ts, ts.Unix()
	}
	if unixValue > 1_000_000_000_000 {
		ts := time.UnixMilli(unixValue).UTC()
		return ts, ts.Unix()
	}
	ts := time.Unix(unixValue, 0).UTC()
	return ts, unixValue
}

func splitRadiationChannels(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	fields := strings.FieldsFunc(trimmed, func(r rune) bool {
		switch r {
		case ',', ';', '|', '/', '\\':
			return true
		case ' ', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	})
	seen := make(map[string]struct{})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		channel := strings.ToLower(strings.TrimSpace(f))
		if channel == "" {
			continue
		}
		if _, ok := seen[channel]; ok {
			continue
		}
		seen[channel] = struct{}{}
		out = append(out, channel)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseIntDefault(v string, def int) int {
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func parseInt64Default(v string, def int64) int64 {
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

var disclaimerTexts = map[string]string{
	"en": "Data is free to download. We do not create it and take no responsibility for its contents.",
	"ru": "Данные доступны для свободного скачивания. Мы их не создаём и не несём за них никакой ответственности.",
	"es": "Los datos se pueden descargar libremente. No los creamos y no asumimos ninguna responsabilidad por su contenido.",
	"fr": "Les données sont libres de téléchargement. Nous ne les créons pas et n'assumons aucune responsabilité quant à leur contenu.",
	"de": "Die Daten können frei heruntergeladen werden. Wir erstellen sie nicht und übernehmen keine Verantwortung für ihren Inhalt.",
	"pt": "Os dados são livres para download. Não os criamos e não assumimos qualquer responsabilidade pelo seu conteúdo.",
	"it": "I dati sono scaricabili liberamente. Non li creiamo e non ci assumiamo alcuna responsabilità per il loro contenuto.",
	"zh": "数据可自由下载。我们不创建这些数据，对其内容不承担任何责任。",
	"ja": "データは自由にダウンロードできます。私たちはデータを作成しておらず、その内容について一切の責任を負いません。",
	"ar": "البيانات متاحة للتنزيل مجانًا. نحن لا ننشئها ولا نتحمل أي مسؤولية عن محتواها.",
	"hi": "डेटा मुक्त रूप से डाउनलोड किया जा सकता है। हम इसे नहीं बनाते हैं और इसकी सामग्री के लिए कोई ज़िम्मेदारी नहीं लेते हैं।",
	"tr": "Veriler ücretsiz olarak indirilebilir. Biz bu verileri üretmiyoruz ve içeriklerinden hiçbir sorumluluk kabul etmiyoruz.",
	"ko": "데이터는 자유롭게 다운로드할 수 있습니다. 우리는 데이터를 만들지 않으며 그 내용에 대해 어떠한 책임도 지지 않습니다.",
}
