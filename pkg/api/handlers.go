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
	Cache   *ResponseCache
}

// NewHandler constructs a Handler with sane defaults.
// Logf is optional; pass nil if logging is not required.
func NewHandler(db *database.Database, dbType string, archive *jsonarchive.Generator, limiter *RateLimiter, logf func(string, ...any)) *Handler {
	return &Handler{
		DB:      db,
		DBType:  dbType,
		Archive: archive,
		Limiter: limiter,
		Logf:    logf,
		Cache:   NewResponseCache(24 * time.Hour),
	}
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
	mux.HandleFunc("/api/shorten", h.handleShorten)
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

// handleShorten issues or finalizes short URLs for the current map view. We
// keep the logic explicit instead of clever so operators can audit it easily,
// following the proverb "Clear is better than clever".
func (h *Handler) handleShorten(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.DB == nil || h.DB.DB == nil {
		http.Error(w, "short links unavailable", http.StatusServiceUnavailable)
		return
	}

	permit, ok := h.acquirePermit(w, r, RequestGeneral)
	if !ok {
		return
	}
	if permit != nil {
		defer permit.Release()
	}

	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	var payload struct {
		URL    string `json:"url"`
		Code   string `json:"code"`
		Commit bool   `json:"commit"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	raw := strings.TrimSpace(payload.URL)
	if raw == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}

	scheme := requestScheme(r)
	host := strings.TrimSpace(r.Host)
	if host == "" {
		http.Error(w, "missing host", http.StatusBadRequest)
		return
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}
	parsed.Fragment = ""

	var target string
	if parsed.Scheme == "" && parsed.Host == "" {
		if !strings.HasPrefix(parsed.Path, "/") {
			http.Error(w, "relative path must start with /", http.StatusBadRequest)
			return
		}
		base := &url.URL{Scheme: scheme, Host: host}
		target = base.ResolveReference(parsed).String()
	} else {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			http.Error(w, "unsupported scheme", http.StatusBadRequest)
			return
		}
		if !strings.EqualFold(strings.TrimSpace(parsed.Host), host) {
			http.Error(w, "foreign host rejected", http.StatusBadRequest)
			return
		}
		target = parsed.String()
	}

	if len(target) > 4096 {
		http.Error(w, "url too long", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var (
		code   string
		stored bool
	)

	if payload.Commit {
		code, err = h.DB.PersistShortLink(ctx, target, payload.Code, time.Now().UTC(), 0)
		stored = (err == nil)
	} else {
		code, stored, err = h.DB.PreviewShortLink(ctx, target, 0)
	}
	if err != nil {
		http.Error(w, "short link unavailable", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("shorten failed for %q: %v", target, err)
		}
		return
	}

	shortURL := fmt.Sprintf("%s://%s/s/%s", scheme, host, code)

	w.Header().Set("Cache-Control", "no-store")
	h.respondJSON(w, map[string]any{
		"code":   code,
		"short":  shortURL,
		"target": target,
		"stored": stored,
	})
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
	// Cache the overview briefly so COUNT(DISTINCT) queries are avoided on every request while keeping data fresh.
	data, err := h.cachedJSONWithTTL(ctx, "overview", time.Minute, func(ctx context.Context) ([]byte, error) {
		totalTracks, latestTrackID, err := h.latestTrackInfo(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, newAPIError(http.StatusRequestTimeout, "request cancelled", "overview latest track info", err)
			}
			return nil, newAPIError(http.StatusInternalServerError, "count tracks", "overview latest track info", err)
		}

		endpoints := map[string]any{
			"listTracks": map[string]any{
				"method":      "GET",
				"path":        "/api/tracks",
				"description": "Returns all track summaries sorted alphabetically. Each summary exposes an index and api URL.",
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
				"description": "Lists all tracks that contain markers within the given year.",
			},
			"tracksByMonth": map[string]any{
				"method":      "GET",
				"path":        "/api/tracks/months/{year}/{month}",
				"description": "Lists all tracks for a calendar month without pagination.",
			},
		}
		if h.Archive != nil {
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

		payload, encErr := encodeJSON(overview)
		if encErr != nil {
			return nil, newAPIError(http.StatusInternalServerError, "encode json", "encode overview response", encErr)
		}
		return payload, nil
	})
	if err != nil {
		h.handleCacheError(w, "overview", err)
		return
	}

	writeJSONBytes(w, data)
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
	data, err := h.cachedJSON(ctx, "tracks:list", func(ctx context.Context) ([]byte, error) {
		return h.buildTracksListJSON(ctx)
	})
	if err != nil {
		h.handleCacheError(w, "track list", err)
		return
	}

	writeJSONBytes(w, data)
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

	cacheKey := fmt.Sprintf("tracks:year:%d", year)
	data, err := h.cachedJSON(ctx, cacheKey, func(ctx context.Context) ([]byte, error) {
		return h.buildTracksByYearJSON(ctx, year, start, end)
	})
	if err != nil {
		h.handleCacheError(w, fmt.Sprintf("tracks by year %d", year), err)
		return
	}

	writeJSONBytes(w, data)
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

	cacheKey := fmt.Sprintf("tracks:month:%04d-%02d", year, month)
	data, err := h.cachedJSON(ctx, cacheKey, func(ctx context.Context) ([]byte, error) {
		return h.buildTracksByMonthJSON(ctx, year, month, start, end)
	})
	if err != nil {
		h.handleCacheError(w, fmt.Sprintf("tracks by month %04d-%02d", year, month), err)
		return
	}

	writeJSONBytes(w, data)
}

func (h *Handler) serveTrackData(w http.ResponseWriter, r *http.Request, trackID string) {
	ctx := r.Context()

	data, err := h.cachedJSON(ctx, "track:data:"+trackID, func(ctx context.Context) ([]byte, error) {
		return h.buildTrackDataJSON(ctx, trackID)
	})
	if err != nil {
		h.handleCacheError(w, fmt.Sprintf("track %s", trackID), err)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", trackjson.SafeCIMFilename(trackID)))
	writeJSONBytes(w, data)
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

	// Allow extra headroom so archive discovery keeps working even when large builds take longer than 30 seconds.
	fetchCtx, cancelFetch := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancelFetch()

	info, err := h.Archive.Fetch(fetchCtx)
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

	streamCtx := r.Context()
	var streamCancel context.CancelFunc
	// Stretch the streaming deadline based on the on-disk size and throttle so downloads finish instead of aborting mid-transfer.
	if stat.Size() > 0 && archiveThrottleRateBytes > 0 {
		estimated := time.Duration(stat.Size()) * time.Second / time.Duration(archiveThrottleRateBytes)
		if estimated < time.Minute {
			estimated = time.Minute
		}
		estimated += 30 * time.Second
		streamCtx, streamCancel = context.WithTimeout(streamCtx, estimated)
		defer streamCancel()
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
			case <-streamCtx.Done():
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
		case <-streamCtx.Done():
			if h.Logf != nil {
				h.Logf("archive stream context done: %v", streamCtx.Err())
			}
			return
		default:
		}
	}
}

// =====================
// Cached JSON builders
// =====================

func (h *Handler) buildTracksListJSON(ctx context.Context) ([]byte, error) {
	tracksCh, errCh := h.DB.StreamTrackSummaries(ctx, "", 0, h.DBType)
	summaries, _, err := collectTrackSummaries(ctx, tracksCh, errCh, 0)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, newAPIError(http.StatusRequestTimeout, "request cancelled", "", err)
		}
		return nil, newAPIError(http.StatusInternalServerError, "track list error", "track list error", err)
	}
	if _, err := h.finalizeSummaries(ctx, "", summaries); err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "track index error", "track index error", err)
	}
	totalTracks, latestTrackID, err := h.latestTrackInfo(ctx)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "count tracks", "count tracks", err)
	}
	resp := struct {
		Tracks        []database.TrackSummary `json:"tracks"`
		TotalTracks   int64                   `json:"totalTracks"`
		LatestTrackID string                  `json:"latestTrackID,omitempty"`
		Disclaimers   map[string]string       `json:"disclaimers"`
	}{
		Tracks:        summaries,
		TotalTracks:   totalTracks,
		LatestTrackID: latestTrackID,
		Disclaimers:   trackjson.Disclaimers,
	}
	data, err := encodeJSON(resp)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "encode json", "encode tracks list", err)
	}
	return data, nil
}

func (h *Handler) buildTracksByYearJSON(ctx context.Context, year int, start, end time.Time) ([]byte, error) {
	tracksCh, errCh := h.DB.StreamTrackSummariesByDateRange(ctx, "", 0, start.Unix(), end.Unix(), h.DBType)
	summaries, _, err := collectTrackSummaries(ctx, tracksCh, errCh, 0)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, newAPIError(http.StatusRequestTimeout, "request cancelled", "", err)
		}
		return nil, newAPIError(http.StatusInternalServerError, "track list error", fmt.Sprintf("tracks by year %d", year), err)
	}
	if _, err := h.finalizeSummaries(ctx, "", summaries); err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "track index error", fmt.Sprintf("tracks by year %d index error", year), err)
	}
	rangeTotal, err := h.DB.CountTracksInRange(ctx, start.Unix(), end.Unix(), h.DBType)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "count tracks", fmt.Sprintf("tracks by year %d range count", year), err)
	}
	totalTracks, latestTrackID, err := h.latestTrackInfo(ctx)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "count tracks", fmt.Sprintf("tracks by year %d total", year), err)
	}
	resp := struct {
		Year          int                     `json:"year"`
		RangeStart    int64                   `json:"rangeStart"`
		RangeEnd      int64                   `json:"rangeEnd"`
		Tracks        []database.TrackSummary `json:"tracks"`
		RangeTotal    int64                   `json:"rangeTotal"`
		TotalTracks   int64                   `json:"totalTracks"`
		LatestTrackID string                  `json:"latestTrackID,omitempty"`
		Disclaimers   map[string]string       `json:"disclaimers"`
	}{
		Year:          year,
		RangeStart:    start.Unix(),
		RangeEnd:      end.Unix(),
		Tracks:        summaries,
		RangeTotal:    rangeTotal,
		TotalTracks:   totalTracks,
		LatestTrackID: latestTrackID,
		Disclaimers:   trackjson.Disclaimers,
	}
	data, err := encodeJSON(resp)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "encode json", fmt.Sprintf("encode tracks by year %d", year), err)
	}
	return data, nil
}

func (h *Handler) buildTracksByMonthJSON(ctx context.Context, year, month int, start, end time.Time) ([]byte, error) {
	tracksCh, errCh := h.DB.StreamTrackSummariesByDateRange(ctx, "", 0, start.Unix(), end.Unix(), h.DBType)
	summaries, _, err := collectTrackSummaries(ctx, tracksCh, errCh, 0)
	if err != nil {
		label := fmt.Sprintf("tracks by month %04d-%02d", year, month)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, newAPIError(http.StatusRequestTimeout, "request cancelled", "", err)
		}
		return nil, newAPIError(http.StatusInternalServerError, "track list error", label, err)
	}
	if _, err := h.finalizeSummaries(ctx, "", summaries); err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "track index error", fmt.Sprintf("tracks by month %04d-%02d index error", year, month), err)
	}
	rangeTotal, err := h.DB.CountTracksInRange(ctx, start.Unix(), end.Unix(), h.DBType)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "count tracks", fmt.Sprintf("tracks by month %04d-%02d range count", year, month), err)
	}
	totalTracks, latestTrackID, err := h.latestTrackInfo(ctx)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "count tracks", fmt.Sprintf("tracks by month %04d-%02d total", year, month), err)
	}
	resp := struct {
		Year          int                     `json:"year"`
		Month         int                     `json:"month"`
		RangeStart    int64                   `json:"rangeStart"`
		RangeEnd      int64                   `json:"rangeEnd"`
		Tracks        []database.TrackSummary `json:"tracks"`
		RangeTotal    int64                   `json:"rangeTotal"`
		TotalTracks   int64                   `json:"totalTracks"`
		LatestTrackID string                  `json:"latestTrackID,omitempty"`
		Disclaimers   map[string]string       `json:"disclaimers"`
	}{
		Year:          year,
		Month:         month,
		RangeStart:    start.Unix(),
		RangeEnd:      end.Unix(),
		Tracks:        summaries,
		RangeTotal:    rangeTotal,
		TotalTracks:   totalTracks,
		LatestTrackID: latestTrackID,
		Disclaimers:   trackjson.Disclaimers,
	}
	data, err := encodeJSON(resp)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "encode json", fmt.Sprintf("encode tracks by month %04d-%02d", year, month), err)
	}
	return data, nil
}

func (h *Handler) buildTrackDataJSON(ctx context.Context, trackID string) ([]byte, error) {
	summary, err := h.DB.GetTrackSummary(ctx, trackID, h.DBType)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "summary error", fmt.Sprintf("track %s summary error", trackID), err)
	}
	if summary.MarkerCount == 0 {
		return nil, newAPIError(http.StatusNotFound, "track not found", "", nil)
	}

	trackIndex, err := h.DB.CountTrackIDsUpTo(ctx, trackID, h.DBType)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "track index error", fmt.Sprintf("track %s index error", trackID), err)
	}

	capHint := 0
	if summary.MarkerCount > 0 {
		if summary.MarkerCount > 4096 {
			capHint = 4096
		} else {
			capHint = int(summary.MarkerCount)
		}
	}
	markers := make([]trackjson.MarkerPayload, 0, capHint)

	markersCh, errCh := h.DB.StreamMarkersByTrackRange(ctx, trackID, summary.FirstID, summary.LastID, 0, h.DBType)
	for marker := range markersCh {
		payload, _ := trackjson.MakeMarkerPayload(marker)
		markers = append(markers, payload)
	}

	if err := <-errCh; err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, newAPIError(http.StatusRequestTimeout, "request cancelled", "", err)
		}
		return nil, newAPIError(http.StatusInternalServerError, "markers error", fmt.Sprintf("track %s markers error", trackID), err)
	}

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

	data, err := encodeJSON(resp)
	if err != nil {
		return nil, newAPIError(http.StatusInternalServerError, "encode json", fmt.Sprintf("encode track %s", trackID), err)
	}
	return data, nil
}

// cachedJSONWithTTL lets handlers fine-tune cache freshness without spawning additional goroutines.
func (h *Handler) cachedJSONWithTTL(ctx context.Context, key string, ttl time.Duration, loader func(context.Context) ([]byte, error)) ([]byte, error) {
	if h.Cache == nil {
		return loader(ctx)
	}
	data, err := h.Cache.GetWithTTL(ctx, key, ttl, loader)
	if err != nil {
		if errors.Is(err, errCacheDisabled) || errors.Is(err, errCacheStopped) || errors.Is(err, errNoLoader) {
			return loader(ctx)
		}
		return nil, err
	}
	return data, nil
}

func (h *Handler) cachedJSON(ctx context.Context, key string, loader func(context.Context) ([]byte, error)) ([]byte, error) {
	return h.cachedJSONWithTTL(ctx, key, 0, loader)
}

func (h *Handler) handleCacheError(w http.ResponseWriter, label string, err error) {
	if err == nil {
		return
	}
	if apiErr, ok := err.(*apiError); ok {
		h.respondAPIError(w, apiErr)
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		http.Error(w, "request cancelled", http.StatusRequestTimeout)
		return
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
	if h.Logf != nil {
		h.Logf("%s: %v", label, err)
	}
}

func (h *Handler) respondAPIError(w http.ResponseWriter, apiErr *apiError) {
	if apiErr == nil {
		return
	}
	http.Error(w, apiErr.Message, apiErr.Status)
	if h.Logf != nil && apiErr.Err != nil {
		if strings.TrimSpace(apiErr.LogMessage) != "" {
			h.Logf("%s: %v", apiErr.LogMessage, apiErr.Err)
		}
	}
}

func encodeJSON(payload any) ([]byte, error) {
	buf, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(buf, '\n'), nil
}

func writeJSONBytes(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

type apiError struct {
	Status     int
	Message    string
	LogMessage string
	Err        error
}

func (e *apiError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func newAPIError(status int, message, logMessage string, err error) *apiError {
	return &apiError{Status: status, Message: message, LogMessage: logMessage, Err: err}
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
	capHint := limit
	if capHint <= 0 {
		capHint = 1024
	}
	summaries := make([]database.TrackSummary, 0, capHint)
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
	data, err := encodeJSON(payload)
	if err != nil {
		http.Error(w, "encode json", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("encode json: %v", err)
		}
		return
	}
	writeJSONBytes(w, data)
}

// requestScheme inspects headers and TLS state so generated URLs reuse the
// scheme the client used to reach us.
func requestScheme(r *http.Request) string {
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto != "" {
		return strings.ToLower(proto)
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
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
