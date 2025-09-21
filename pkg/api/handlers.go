package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	mux.HandleFunc("/api/tracks/", h.handleTrackData)
	mux.HandleFunc("/api/kml/daily.tar.gz", h.handleArchiveDownload)
}

// handleOverview publishes machine-readable docs so developers understand
// which endpoints to call and how to iterate through data sets.
func (h *Handler) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	totalTracks, err := h.DB.CountTracks(ctx)
	if err != nil {
		http.Error(w, "count tracks", http.StatusInternalServerError)
		return
	}

	overview := struct {
		Disclaimers map[string]string `json:"disclaimers"`
		Endpoints   map[string]any    `json:"endpoints"`
		TotalTracks int64             `json:"totalTracks"`
	}{
		Disclaimers: disclaimerTexts,
		TotalTracks: totalTracks,
		Endpoints: map[string]any{
			"listTracks": map[string]any{
				"method":      "GET",
				"path":        "/api/tracks",
				"query":       []string{"startAfter", "limit"},
				"description": "Returns track summaries sorted alphabetically. Use nextStartAfter to continue pagination.",
			},
			"trackMarkers": map[string]any{
				"method":      "GET",
				"path":        "/api/tracks/{trackID}",
				"query":       []string{"from", "limit", "to"},
				"description": "Streams markers for the requested track. Provide 'from' using the firstID from summaries and increment using nextFromID.",
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

	summaries := make([]database.TrackSummary, 0, limit)
	var lastTrackID string
	for {
		select {
		case <-ctx.Done():
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
			return
		case summary, ok := <-tracksCh:
			if !ok {
				tracksCh = nil
				goto finished
			}
			summaries = append(summaries, summary)
			lastTrackID = summary.TrackID
		}
	}

finished:
	if err := <-errCh; err != nil {
		http.Error(w, "track list error", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("track list error: %v", err)
		}
		return
	}

	var next string
	if len(summaries) == limit {
		next = lastTrackID
	}

	totalTracks, err := h.DB.CountTracks(ctx)
	if err != nil {
		http.Error(w, "count tracks", http.StatusInternalServerError)
		return
	}

	resp := struct {
		StartAfter     string                  `json:"startAfter"`
		Limit          int                     `json:"limit"`
		Tracks         []database.TrackSummary `json:"tracks"`
		NextStartAfter string                  `json:"nextStartAfter,omitempty"`
		TotalTracks    int64                   `json:"totalTracks"`
		Disclaimers    map[string]string       `json:"disclaimers"`
	}{
		StartAfter:     startAfter,
		Limit:          limit,
		Tracks:         summaries,
		NextStartAfter: next,
		TotalTracks:    totalTracks,
		Disclaimers:    disclaimerTexts,
	}

	h.respondJSON(w, resp)
}

// handleTrackData streams markers from a single track using ID ranges.
func (h *Handler) handleTrackData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	trackID := strings.TrimPrefix(r.URL.Path, "/api/tracks/")
	if trackID == "" {
		http.NotFound(w, r)
		return
	}

	summary, err := h.DB.GetTrackSummary(ctx, trackID, h.DBType)
	if err != nil {
		http.Error(w, "summary error", http.StatusInternalServerError)
		return
	}
	if summary.MarkerCount == 0 {
		http.Error(w, "track not found", http.StatusNotFound)
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

	for {
		select {
		case <-ctx.Done():
			http.Error(w, "request cancelled", http.StatusRequestTimeout)
			return
		case marker, ok := <-markersCh:
			if !ok {
				markersCh = nil
				goto drained
			}
			markers = append(markers, marker)
			lastID = marker.ID
		}
	}

drained:
	if err := <-errCh; err != nil {
		http.Error(w, "markers error", http.StatusInternalServerError)
		if h.Logf != nil {
			h.Logf("markers error: %v", err)
		}
		return
	}

	nextFrom := int64(0)
	if lastID > 0 && lastID < summary.LastID {
		nextFrom = lastID + 1
	}

	resp := struct {
		TrackID     string            `json:"trackID"`
		From        int64             `json:"from"`
		To          int64             `json:"to"`
		Limit       int               `json:"limit"`
		NextFrom    int64             `json:"nextFrom,omitempty"`
		LastID      int64             `json:"lastID"`
		Markers     []database.Marker `json:"markers"`
		Disclaimers map[string]string `json:"disclaimers"`
	}{
		TrackID:     trackID,
		From:        from,
		To:          to,
		Limit:       limit,
		NextFrom:    nextFrom,
		LastID:      summary.LastID,
		Markers:     markers,
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
