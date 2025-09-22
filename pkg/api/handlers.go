package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
	"chicha-isotope-map/pkg/jsonarchive"
	"chicha-isotope-map/pkg/trackjson"
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
	Archive *jsonarchive.Generator
	Limiter *RateLimiter
	Logf    func(string, ...any)
}

// NewHandler constructs a Handler with sane defaults.
// Logf is optional; pass nil if logging is not required.
func NewHandler(db *database.Database, dbType string, archive *jsonarchive.Generator, limiter *RateLimiter, logf func(string, ...any)) *Handler {
	return &Handler{DB: db, DBType: dbType, Archive: archive, Limiter: limiter, Logf: logf}
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
	mux.HandleFunc("/api/track/", h.handleTrackData)
	mux.HandleFunc("/api/tracks/", h.handleTrackData) // legacy alias for older clients
	if h.Archive != nil {
		// Expose the tarball endpoint only when archive generation is enabled
		// so clients do not see a dangling route that always fails.
		mux.HandleFunc("/api/json/daily.tar.gz", h.handleArchiveDownload)
	}
}

// acquirePermit enforces the per-IP rate limiter and writes an informative
// response if the caller exceeded their allowance. Returning false means the
// handler should stop processing because a response was already sent.
func (h *Handler) acquirePermit(w http.ResponseWriter, r *http.Request, kind RequestKind) (*Permit, bool) {
	if h.Limiter == nil {
		return nil, true
	}

	ip := clientIP(r)
	permit, err := h.Limiter.Acquire(r.Context(), ip, kind)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
			return nil, false
		}
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		if h.Logf != nil {
			h.Logf("rate limiter rejected %s %s: %v", ip, r.URL.Path, err)
		}
		return nil, false
	}

	if permit != nil && permit.WaitNotice {
		delay := permit.WaitDuration
		if delay <= 0 {
			delay = time.Millisecond
		}
		w.Header().Set("X-RateLimit-Notice", fmt.Sprintf("Delayed %s to protect the API", delay))
		w.Header().Set("X-RateLimit-Delay", fmt.Sprintf("%.3f", permit.WaitDuration.Seconds()))
		if h.Logf != nil {
			h.Logf("rate limiter delayed %s %s by %s", ip, r.URL.Path, delay)
		}
	}

	return permit, true
}

// clientIP extracts the best-effort caller IP address from HTTP headers so the
// limiter can group requests. We prefer the first X-Forwarded-For value because
// many deployments sit behind a reverse proxy.
func clientIP(r *http.Request) string {
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		candidate := strings.TrimSpace(parts[0])
		if candidate != "" {
			return candidate
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	if trimmed := strings.TrimSpace(r.RemoteAddr); trimmed != "" {
		return trimmed
	}
	return "unknown"
}

// handleOverview publishes machine-readable docs so developers understand
// which endpoints to call and how to iterate through data sets.
func (h *Handler) handleOverview(w http.ResponseWriter, r *http.Request) {
	permit, ok := h.acquirePermit(w, r, RequestGeneral)
	if !ok {
		return
	}
	if permit != nil {
		defer permit.Release()
	}

	ctx := r.Context()

	totalTracks, latestTrackID, err := h.latestTrackInfo(ctx)
	if err != nil {
		http.Error(w, "count tracks", http.StatusInternalServerError)
		return
	}

	endpoints := map[string]any{
		"listTracks": map[string]any{
			"method":      "GET",
			"path":        "/api/tracks",
			"query":       []string{"startAfter", "limit"},
			"description": "Returns track summaries sorted alphabetically. Each summary exposes an index and apiURL. Use nextStartAfter to continue pagination.",
		},
		"trackByNumber": map[string]any{
			"method":      "GET",
			"path":        "/api/tracks/index/{number}",
			"description": "Resolves a track by its 1-based numeric index and streams markers just like /api/track/{trackID}.cim.",
		},
		"trackMarkers": map[string]any{
			"method":      "GET",
			"path":        "/api/track/{trackID}.cim",
			"query":       []string{"from", "to"},
			"description": "Downloads the full track as JSON with a .cim extension so browsers save it as a file. Optional 'from'/'to' IDs can narrow the range.",
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
	}
	if h.Archive != nil {
		// Advertise the archive only when generation is active to keep the
		// machine-readable docs truthful.
		endpoints["dailyJSON"] = map[string]any{
			"method":      "GET",
			"path":        "/api/json/daily.tar.gz",
			"description": "Downloads the current tar.gz bundle of all published .cim JSON tracks.",
			"frequency":   "Updated once per day",
		}
	}

	overview := struct {
		Disclaimers      map[string]string `json:"disclaimers"`
		Endpoints        map[string]any    `json:"endpoints"`
		TotalTracks      int64             `json:"totalTracks"`
		LatestTrackIndex int64             `json:"latestTrackIndex"`
		LatestTrackID    string            `json:"latestTrackID,omitempty"`
	}{
		Disclaimers:      trackjson.Disclaimers,
		TotalTracks:      totalTracks,
		LatestTrackIndex: totalTracks,
		LatestTrackID:    latestTrackID,
		Endpoints:        endpoints,
	}

	h.respondJSON(w, overview)
}

// handleTracksList exposes paginated track summaries.
func (h *Handler) handleTracksList(w http.ResponseWriter, r *http.Request) {
	permit, ok := h.acquirePermit(w, r, RequestGeneral)
	if !ok {
		return
	}
	if permit != nil {
		defer permit.Release()
	}

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
		Disclaimers:    trackjson.Disclaimers,
	}

	h.respondJSON(w, resp)
}

// handleTrackData streams markers from a single track using ID ranges.
func (h *Handler) handleTrackData(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	var trimmed string
	switch {
	case strings.HasPrefix(path, "/api/track/"):
		trimmed = strings.TrimPrefix(path, "/api/track/")
	case strings.HasPrefix(path, "/api/tracks/"):
		trimmed = strings.TrimPrefix(path, "/api/tracks/")
	default:
		http.NotFound(w, r)
		return
	}
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(trimmed, ".cim") {
		trimmed = strings.TrimSuffix(trimmed, ".cim")
	}
	decoded, err := url.PathUnescape(trimmed)
	if err != nil || strings.TrimSpace(decoded) == "" {
		http.NotFound(w, r)
		return
	}
	permit, ok := h.acquirePermit(w, r, RequestHeavy)
	if !ok {
		return
	}
	if permit != nil {
		defer permit.Release()
	}
	h.serveTrackData(w, r, decoded)
}

// handleTrackDataByIndex resolves a numeric track number and reuses the
// standard track handler so consumers can iterate sequentially.
func (h *Handler) handleTrackDataByIndex(w http.ResponseWriter, r *http.Request) {
	permit, ok := h.acquirePermit(w, r, RequestHeavy)
	if !ok {
		return
	}
	if permit != nil {
		defer permit.Release()
	}

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
	permit, ok := h.acquirePermit(w, r, RequestGeneral)
	if !ok {
		return
	}
	if permit != nil {
		defer permit.Release()
	}

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
		Disclaimers:    trackjson.Disclaimers,
	}

	h.respondJSON(w, resp)
}

// handleTracksByMonth narrows track summaries to a calendar month.
func (h *Handler) handleTracksByMonth(w http.ResponseWriter, r *http.Request) {
	permit, ok := h.acquirePermit(w, r, RequestGeneral)
	if !ok {
		return
	}
	if permit != nil {
		defer permit.Release()
	}

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
		Disclaimers:    trackjson.Disclaimers,
	}

	h.respondJSON(w, resp)
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
	to := parseInt64Default(q.Get("to"), summary.LastID)
	if to <= 0 || to > summary.LastID {
		to = summary.LastID
	}
	if from > to {
		http.Error(w, "invalid range", http.StatusBadRequest)
		return
	}

	chunkSize := clampInt(parseIntDefault(q.Get("chunk"), 1000), 1, 5000)

	capEstimate := 0
	if summary.MarkerCount > 0 {
		if summary.MarkerCount > 4096 {
			capEstimate = 4096
		} else {
			capEstimate = int(summary.MarkerCount)
		}
	}
	markers := make([]trackjson.MarkerPayload, 0, capEstimate)

	nextID := from
	for nextID <= to {
		select {
		case <-ctx.Done():
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
			return
		default:
		}

		limit := chunkSize
		remaining := to - nextID + 1
		if remaining > 0 && int64(limit) > remaining {
			limit = int(remaining)
		}

		markersCh, errCh := h.DB.StreamMarkersByTrackRange(ctx, trackID, nextID, to, limit, h.DBType)
		got := false
		var lastID int64

		for marker := range markersCh {
			got = true
			lastID = marker.ID
			payload, _ := trackjson.MakeMarkerPayload(marker)
			markers = append(markers, payload)
		}

		if err := <-errCh; err != nil {
			http.Error(w, "markers error", http.StatusInternalServerError)
			if h.Logf != nil {
				h.Logf("track %s markers error: %v", trackID, err)
			}
			return
		}
		if !got {
			break
		}
		nextID = lastID + 1
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", trackjson.SafeCIMFilename(trackID)))

	resp := struct {
		TrackID     string                    `json:"trackID"`
		TrackIndex  int64                     `json:"trackIndex"`
		APIURL      string                    `json:"apiURL"`
		FirstID     int64                     `json:"firstID"`
		LastID      int64                     `json:"lastID"`
		MarkerCount int64                     `json:"markerCount"`
		Markers     []trackjson.MarkerPayload `json:"markers"`
		Disclaimers map[string]string         `json:"disclaimers"`
	}{
		TrackID:     trackID,
		TrackIndex:  trackIndex,
		APIURL:      trackjson.TrackAPIPath(trackID),
		FirstID:     summary.FirstID,
		LastID:      summary.LastID,
		MarkerCount: summary.MarkerCount,
		Markers:     markers,
		Disclaimers: trackjson.Disclaimers,
	}

	h.respondJSON(w, resp)
}

const (
	// archiveThrottleRateBytes limits archive downloads to 5 MiB per second
	// so a single client cannot saturate the network.
	archiveThrottleRateBytes int64 = 5 * 1024 * 1024
	// archiveThrottleTick breaks the throttle into smaller intervals so we
	// can react to cancellations promptly instead of sleeping for a full
	// second.
	archiveThrottleTick = 200 * time.Millisecond
)

// handleArchiveDownload streams the daily tar.gz bundle of .cim JSON tracks produced by the generator.
func (h *Handler) handleArchiveDownload(w http.ResponseWriter, r *http.Request) {
	if h.Archive == nil {
		http.Error(w, "archive disabled", http.StatusServiceUnavailable)
		return
	}

	permit, ok := h.acquirePermit(w, r, RequestHeavy)
	if !ok {
		return
	}
	if permit != nil {
		defer permit.Release()
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
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	w.Header().Set("Last-Modified", stat.ModTime().UTC().Format(http.TimeFormat))

	bytesPerTick := archiveThrottleRateBytes * int64(archiveThrottleTick) / int64(time.Second)
	if bytesPerTick <= 0 {
		bytesPerTick = archiveThrottleRateBytes
	}
	ticker := time.NewTicker(archiveThrottleTick)
	defer ticker.Stop()

	allowed := bytesPerTick
	buf := make([]byte, 64*1024)
	flusher, _ := w.(http.Flusher)

	for {
		if allowed <= 0 {
			select {
			case <-ctx.Done():
				if h.Logf != nil {
					h.Logf("archive stream cancelled for %s", r.URL.Path)
				}
				return
			case <-ticker.C:
				allowed = bytesPerTick
			}
		}

		chunk := len(buf)
		if int64(chunk) > allowed {
			chunk = int(allowed)
		}
		n, readErr := file.Read(buf[:chunk])
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				if h.Logf != nil {
					h.Logf("archive write error: %v", writeErr)
				}
				return
			}
			allowed -= int64(n)
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			if h.Logf != nil {
				h.Logf("archive read error: %v", readErr)
			}
			return
		}
		select {
		case <-ctx.Done():
			if h.Logf != nil {
				h.Logf("archive stream context done: %v", ctx.Err())
			}
			return
		default:
		}
	}
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

	if summaries[0].Index > 0 {
		// New database streaming already delivers the index, so we only need to
		// adjust it into a zero-based base once. This keeps handlers cheap while
		// still supporting legacy databases that lack the precomputed value.
		base = summaries[0].Index - 1
	} else if trimmed := strings.TrimSpace(startAfter); trimmed != "" {
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
		summaries[i].APIURL = trackjson.TrackAPIPath(summaries[i].TrackID)
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

func (h *Handler) respondJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
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
