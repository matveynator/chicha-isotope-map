package safecastimport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
)

// Logger keeps logging dependency injection minimal so tests can pass a stub
// while production wiring points at log.Printf. We only accept the printf-like
// signature we rely on which mirrors the Go proverb "The bigger the interface,
// the weaker the abstraction".
type Logger func(string, ...any)

// StoreFunc abstracts the marker storage pipeline so the importer can reuse the
// existing upload machinery without creating an import cycle between packages.
// The function matches processAndStoreMarkers from main.go.
type StoreFunc func([]database.Marker, string, *database.Database, string, string, string) (database.Bounds, string, error)

// Start launches a background goroutine that polls the Safecast REST API every
// hour and imports freshly approved bGeigie logs into the local database.  The
// worker is intentionally self-contained: it shares state only through the
// provided channels, following "Don't communicate by sharing memory".
func Start(ctx context.Context, db *database.Database, dbType string, store StoreFunc, logf Logger) {
	if ctx == nil || db == nil || store == nil {
		return
	}
	engine := strings.ToLower(strings.TrimSpace(dbType))
	if engine == "clickhouse" {
		if logf != nil {
			logf("safecast import disabled: clickhouse backend lacks lightweight UPSERT support")
		}
		return
	}

	jobs := make(chan time.Time, 1)
	go schedule(ctx, jobs)
	go worker(ctx, jobs, db, engine, store, logf)
}

// schedule feeds timestamps to the importer worker.  A dedicated goroutine keeps
// tick handling tiny and easy to inspect while allowing the worker to back off
// gracefully when a previous sync is still running.
func schedule(ctx context.Context, jobs chan<- time.Time) {
	defer close(jobs)

	immediate := time.Now().UTC()
	select {
	case <-ctx.Done():
		return
	case jobs <- immediate:
	}

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			select {
			case <-ctx.Done():
				return
			case jobs <- t.UTC():
			default:
				// Worker still busy; drop this tick so we do not pile up work.
			}
		}
	}
}

// worker consumes scheduled timestamps and performs the actual import cycle.
// We bound each run with a timeout to avoid dangling HTTP requests when the
// application is shutting down.
func worker(ctx context.Context, jobs <-chan time.Time, db *database.Database, dbType string, store StoreFunc, logf Logger) {
	// Safecast politely requests that automated clients keep concurrency low.
	// We follow "Don't communicate by sharing memory" by keeping a single
	// worker goroutine and also cap the HTTP transport to a single idle and
	// active connection.  This honours the API guidance while still giving us
	// keep-alive reuse for sequential downloads.
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConns:        1,
		MaxIdleConnsPerHost: 1,
		MaxConnsPerHost:     1,
	}
	client := &http.Client{Timeout: 45 * time.Second, Transport: transport}
	importer := &syncer{db: db, dbType: dbType, store: store, logf: logf, client: client}

	for {
		select {
		case <-ctx.Done():
			return
		case ts, ok := <-jobs:
			if !ok {
				return
			}

			runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			if err := importer.run(runCtx, ts); err != nil && logf != nil {
				logf("safecast sync failed: %v", err)
			}
			cancel()
		}
	}
}

// syncer keeps the state required to import a batch of Safecast datasets.
type syncer struct {
	db     *database.Database
	dbType string
	store  StoreFunc
	logf   Logger
	client *http.Client
}

// statusError keeps HTTP error metadata so the caller can decide whether
// shrinking the time window might help. We intentionally keep the struct tiny
// so it is easy to construct without additional helpers.
type statusError struct {
	code int
	body string
}

func (e *statusError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("unexpected status %d: %s", e.code, e.body)
}

func (e *statusError) retryable() bool {
	if e == nil {
		return false
	}
	switch e.code {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// run collects metadata for all approved uploads since the last successful
// import, downloads each missing track, converts the payload into markers, and
// stores them via processAndStoreMarkers.  Errors are logged and skipped so the
// loop can continue processing the remaining datasets.
func (s *syncer) run(ctx context.Context, now time.Time) error {
	if s.db == nil {
		return errors.New("database unavailable")
	}

	// First try the generic cursor table so sources added later resume from the
	// last successful run. Falling back to the legacy Safecast ledger keeps
	// upgrades compatible with previously imported datasets.
	since, err := s.db.ImportCursor(ctx, s.dbType, "safecast")
	if err != nil {
		return fmt.Errorf("query safecast cursor: %w", err)
	}
	if since.IsZero() {
		since, err = s.db.LatestSafecastUpload(ctx, s.dbType)
		if err != nil {
			return fmt.Errorf("query safecast ledger: %w", err)
		}
	}

	// Safecast measurements stretch back well before Fukushima.  Brand new
	// databases therefore rewind to the year 2000 so we do not accidentally
	// skip early logs from legacy deployments.
	if since.IsZero() {
		since = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	} else {
		// Step back slightly to re-check records sharing the same second.
		since = since.Add(-2 * time.Minute)
	}

	until := now.UTC()
	if !until.After(since) {
		until = since.Add(time.Hour)
	}

	const (
		maxWindow  = 30 * 24 * time.Hour
		minWindow  = time.Hour
		backoffMin = 30 * time.Second
		backoffMax = 10 * time.Minute
	)
	window := 14 * 24 * time.Hour
	if window < minWindow {
		window = minWindow
	}
	backoff := backoffMin

	cursor := since
	for cursor.Before(until) {
		chunkEnd := cursor.Add(window)
		if chunkEnd.After(until) {
			chunkEnd = until
		}
		if !chunkEnd.After(cursor) {
			chunkEnd = cursor.Add(minWindow)
		}

		var imports []importSummary
		for {
			var err error
			imports, err = s.collectImports(ctx, cursor, chunkEnd)
			if err != nil {
				var httpErr *statusError
				if errors.As(err, &httpErr) && httpErr.retryable() {
					if window > minWindow {
						window = window / 2
						if window < minWindow {
							window = minWindow
						}
						if s.logf != nil {
							s.logf("safecast sync: reducing window to %s after status %d", window, httpErr.code)
						}
						chunkEnd = cursor.Add(window)
						if chunkEnd.After(until) {
							chunkEnd = until
						}
						if !chunkEnd.After(cursor) {
							chunkEnd = cursor.Add(minWindow)
						}
						backoff = backoffMin
						continue
					}

					wait := backoff
					if s.logf != nil {
						s.logf("safecast sync: waiting %s before retry after status %d", wait, httpErr.code)
					}
					if err := waitContext(ctx, wait); err != nil {
						return err
					}
					if backoff < backoffMax {
						backoff *= 2
						if backoff > backoffMax {
							backoff = backoffMax
						}
					}
					continue
				}
				return err
			}
			backoff = backoffMin
			break
		}

		if len(imports) == 0 {
			if s.logf != nil {
				s.logf("safecast sync: no new imports between %s and %s", cursor.Format(time.RFC3339), chunkEnd.Format(time.RFC3339))
			}
			cursor = chunkEnd
			if window < maxWindow {
				window *= 2
				if window > maxWindow {
					window = maxWindow
				}
			}
			continue
		}

		sort.Slice(imports, func(i, j int) bool {
			if !imports[i].UploadedAt.Equal(imports[j].UploadedAt) {
				return imports[i].UploadedAt.Before(imports[j].UploadedAt)
			}
			return imports[i].ID < imports[j].ID
		})

		latestSeen := cursor
		throttle := time.Duration(0)
		for _, imp := range imports {
			if throttle > 0 {
				if err := waitContext(ctx, throttle); err != nil {
					return err
				}
			}
			throttle = 2 * time.Second

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if imp.UploadedAt.After(latestSeen) {
				latestSeen = imp.UploadedAt
			}

			exists, err := s.db.SafecastImportExists(ctx, s.dbType, imp.ID)
			if err != nil {
				if s.logf != nil {
					s.logf("safecast sync: skip import %d due to existence probe: %v", imp.ID, err)
				}
				continue
			}
			if exists {
				if err := s.db.UpdateImportCursor(ctx, s.dbType, "safecast", imp.UploadedAt); err != nil && s.logf != nil {
					s.logf("safecast sync: cursor update failed: %v", err)
				}
				continue
			}

			if s.logf != nil {
				s.logf("safecast sync: importing %d uploaded %s", imp.ID, imp.UploadedAt.Format(time.RFC3339))
			}

			markers, err := s.downloadAndParse(ctx, imp)
			if err != nil {
				if s.logf != nil {
					s.logf("safecast sync: download %d failed: %v", imp.ID, err)
				}
				continue
			}
			if len(markers) == 0 {
				if s.logf != nil {
					s.logf("safecast sync: import %d had no valid markers", imp.ID)
				}
				continue
			}

			trackID := fmt.Sprintf("SC-%d", imp.ID)
			sourceURL := fmt.Sprintf("https://api.safecast.org/en-US/bgeigie_imports/%d", imp.ID)
			bbox, storedTrackID, err := s.store(markers, trackID, s.db, s.dbType, "safecast", sourceURL)
			if err != nil {
				if s.logf != nil {
					s.logf("safecast sync: store %d failed: %v", imp.ID, err)
				}
				continue
			}
			if s.logf != nil {
				s.logf("safecast sync: stored track %s (bbox %.4f,%.4f⇢%.4f,%.4f)", storedTrackID, bbox.MinLat, bbox.MinLon, bbox.MaxLat, bbox.MaxLon)
			}

			if err := s.db.MarkSafecastImport(ctx, s.dbType, imp.ID, imp.UploadedAt); err != nil {
				if s.logf != nil {
					s.logf("safecast sync: record %d failed: %v", imp.ID, err)
				}
			}
			if err := s.db.UpdateImportCursor(ctx, s.dbType, "safecast", imp.UploadedAt); err != nil && s.logf != nil {
				s.logf("safecast sync: cursor update failed: %v", err)
			}
		}

		nextCursor := latestSeen.Add(time.Second)
		if nextCursor.Before(chunkEnd) {
			nextCursor = chunkEnd
		}
		cursor = nextCursor
		if window < maxWindow {
			window *= 2
			if window > maxWindow {
				window = maxWindow
			}
		}
	}
	return nil
}

// importSummary stores the metadata required to download and categorise a
// single Safecast upload.
type importSummary struct {
	ID         int64
	UploadedAt time.Time
	URLs       []string
}

// collectImports walks Safecast pagination until all uploads in the requested
// interval have been gathered.  We keep the parser resilient by accepting both
// RFC3339 and legacy timestamp formats.
func (s *syncer) collectImports(ctx context.Context, since, until time.Time) ([]importSummary, error) {
	const perPage = 100
	out := make([]importSummary, 0, perPage)
	baseURL := "https://api.safecast.org/en-US/bgeigie_imports"

	for page := 1; ; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
		if err != nil {
			return nil, err
		}
		q := req.URL.Query()
		q.Set("by_status", "done")
		q.Set("status", "approved")
		q.Set("format", "json")
		q.Set("per_page", strconv.Itoa(perPage))
		q.Set("page", strconv.Itoa(page))
		q.Set("uploaded_after", urlTime(since))
		q.Set("uploaded_before", urlTime(until))
		req.URL.RawQuery = q.Encode()
		req.Header.Set("User-Agent", "chicha-isotope-map safecast importer")
		req.Header.Set("Accept", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list imports: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read import page: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return out, &statusError{code: resp.StatusCode, body: truncate(string(body), 240)}
		}

		batch, err := parseImportPage(body)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		out = append(out, batch...)
		if len(batch) < perPage {
			break
		}

		// Space out pagination slightly so we do not hammer the API when a
		// window spans many pages.  The wait honours context cancellation so
		// shutdown stays responsive.
		if err := waitContext(ctx, 1500*time.Millisecond); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// waitContext pauses for the requested duration unless the context is cancelled.
// Using a helper keeps our throttling sites readable while still ensuring we do
// not block shutdown paths.
func waitContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// urlTime renders timestamps in the format expected by the Safecast endpoint,
// e.g. "2024/01/02 15:04:05".
func urlTime(t time.Time) string {
	return t.UTC().Format("2006/01/02 15:04:05")
}

// parseImportPage extracts metadata from a JSON response.  The endpoint returns
// an array so we decode into a slice of lightweight wrappers and inspect common
// URL fields.
func parseImportPage(body []byte) ([]importSummary, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	var payload []map[string]any
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode imports: %w", err)
	}
	out := make([]importSummary, 0, len(payload))
	for _, item := range payload {
		id, err := numericID(item["id"])
		if err != nil {
			continue
		}
		uploadedAt, err := parseUploadedAt(item["uploaded_at"])
		if err != nil {
			continue
		}
		urls := collectCandidateURLs(item)
		if len(urls) == 0 {
			continue
		}
		out = append(out, importSummary{ID: id, UploadedAt: uploadedAt, URLs: urls})
	}
	return out, nil
}

// numericID converts any JSON number/string into an int64.
func numericID(raw any) (int64, error) {
	switch v := raw.(type) {
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, err
		}
		return i, nil
	case float64:
		return int64(v), nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, errors.New("empty")
		}
		i, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, err
		}
		return i, nil
	default:
		return 0, errors.New("unsupported id type")
	}
}

// parseUploadedAt normalises timestamps provided as either string or nested JSON.
func parseUploadedAt(raw any) (time.Time, error) {
	switch v := raw.(type) {
	case string:
		ts := parseTimeFlexible(v)
		if ts.IsZero() {
			return time.Time{}, errors.New("invalid timestamp")
		}
		return ts, nil
	case map[string]any:
		if text, ok := v["uploaded_at"].(string); ok {
			ts := parseTimeFlexible(text)
			if ts.IsZero() {
				return time.Time{}, errors.New("invalid timestamp")
			}
			return ts, nil
		}
	}
	return time.Time{}, errors.New("missing uploaded_at")
}

// collectCandidateURLs scans JSON fields for likely download locations.  Safecast
// exposes several redundant fields; gathering all of them increases our odds of
// finding an accessible file without hard-coding every variant.
func collectCandidateURLs(item map[string]any) []string {
	candidates := make([]string, 0, 6)
	add := func(raw any) {
		s, ok := raw.(string)
		if !ok {
			return
		}
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			return
		}
		if !strings.HasPrefix(trimmed, "http") {
			return
		}
		candidates = append(candidates, trimmed)
	}

	for _, key := range []string{"csv_url", "gdrive_url", "url", "download_url", "data_file", "data_file_url", "s3_url", "s3_file_url", "log_url", "original_log_url"} {
		if val, ok := item[key]; ok {
			add(val)
		}
	}

	if files, ok := item["files"].([]any); ok {
		for _, f := range files {
			if m, ok := f.(map[string]any); ok {
				add(m["url"])
			}
		}
	}

	seen := make(map[string]struct{}, len(candidates))
	uniq := candidates[:0]
	for _, u := range candidates {
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		uniq = append(uniq, u)
	}
	return uniq
}

// downloadAndParse iterates through candidate URLs until a payload successfully
// converts into markers.  Returning the first successful parse keeps the worker
// lean while still tolerating transient CDN errors or permission issues.
func (s *syncer) downloadAndParse(ctx context.Context, imp importSummary) ([]database.Marker, error) {
	for _, u := range imp.URLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "chicha-isotope-map safecast importer")
		resp, err := s.client.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		if resp.StatusCode != http.StatusOK {
			continue
		}

		markers := parseMarkers(body, u)
		if len(markers) == 0 {
			continue
		}
		return markers, nil
	}
	return nil, fmt.Errorf("no downloadable artifact for %d", imp.ID)
}

// parseMarkers inspects the source URL extension and chooses a parsing strategy
// suitable for the payload.  We currently support CSV and raw bGeigie logs.
func parseMarkers(body []byte, sourceURL string) []database.Marker {
	ext := strings.ToLower(filepathExt(sourceURL))
	switch ext {
	case ".csv":
		return parseCSVMarkers(body)
	case ".log", ".txt":
		return parseLogMarkers(body)
	case ".json":
		return parseJSONMarkers(body)
	default:
		trimmed := bytes.TrimSpace(body)
		if bytes.HasPrefix(trimmed, []byte("$BNRDD")) {
			return parseLogMarkers(trimmed)
		}
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			if markers := parseJSONMarkers(trimmed); len(markers) > 0 {
				return markers
			}
		}
		lowered := bytes.ToLower(trimmed)
		if bytes.Contains(lowered, []byte("latitude")) && bytes.Contains(lowered, []byte("longitude")) {
			return parseCSVMarkers(trimmed)
		}
	}
	return nil
}

// parseJSONMarkers decodes arrays or wrapper objects that expose Safecast style
// measurement data. We treat a value as µSv/h unless unit metadata explicitly
// states counts so the map honours upstream semantics.
func parseJSONMarkers(data []byte) []database.Marker {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var payload any
	if err := dec.Decode(&payload); err != nil {
		return nil
	}

	records := flattenJSON(payload)
	markers := make([]database.Marker, 0, len(records))

	for _, record := range records {
		lat := floatFromAny(getField(record, "latitude", "lat"))
		lon := floatFromAny(getField(record, "longitude", "lon", "lng"))
		if lat == 0 && lon == 0 {
			continue
		}

		ts := timeFromAny(getField(record, "captured_at", "timestamp", "created_at", "measured_at", "time"))
		if ts.IsZero() {
			continue
		}

		dose := floatFromAny(getField(record, "usvh", "usv_per_h", "micro_sv", "microsieverts_per_hour"))
		unitName := unitNameFromRecord(record)
		if dose <= 0 {
			value := floatFromAny(getField(record, "value", "radiation", "dose"))
			switch {
			case value <= 0:
			case strings.Contains(unitName, "cpm"):
				dose = convertCPMToMicroSv(value)
			case strings.Contains(unitName, "cps"):
				dose = convertCPSToMicroSv(value)
			case unitName == "":
				dose = value
			case strings.Contains(unitName, "sv"):
				dose = value
			}
		}
		if dose <= 0 {
			if cpm := floatFromAny(getField(record, "cpm")); cpm > 0 {
				dose = convertCPMToMicroSv(cpm)
			} else if cps := floatFromAny(getField(record, "cps")); cps > 0 {
				dose = convertCPSToMicroSv(cps)
			}
		}
		if dose <= 0 {
			continue
		}

		countRate := floatFromAny(getField(record, "cps"))
		if countRate <= 0 {
			if cpm := floatFromAny(getField(record, "cpm")); cpm > 0 {
				countRate = cpm / 60.0
			} else {
				value := floatFromAny(getField(record, "value"))
				switch {
				case value <= 0:
				case strings.Contains(unitName, "cps"):
					countRate = value
				case strings.Contains(unitName, "cpm"):
					countRate = value / 60.0
				}
			}
		}

		markers = append(markers, database.Marker{
			DoseRate:  dose,
			Date:      ts.Unix(),
			Lon:       lon,
			Lat:       lat,
			CountRate: countRate,
			Zoom:      0,
			Speed:     0,
		})
	}
	return markers
}

// flattenJSON unwraps common envelope structures such as {"measurements": [...]}
// so downstream parsing only deals with individual measurement objects.
func flattenJSON(payload any) []map[string]any {
	switch v := payload.(type) {
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]any:
		for _, key := range []string{"measurements", "data", "results", "items", "entries"} {
			if nested, ok := valueCaseInsensitive(v, key); ok {
				switch t := nested.(type) {
				case []any:
					return flattenJSON(t)
				case map[string]any:
					return flattenJSON(t)
				}
			}
		}
		return []map[string]any{v}
	default:
		return nil
	}
}

// getField retrieves the first matching key ignoring case differences.
func getField(record map[string]any, keys ...string) any {
	for _, key := range keys {
		if val, ok := valueCaseInsensitive(record, key); ok {
			return val
		}
	}
	return nil
}

// unitNameFromRecord extracts a lower-cased unit hint so callers can apply the
// right conversion factors without guessing.
func unitNameFromRecord(record map[string]any) string {
	for _, key := range []string{"unit", "unit_name", "unitlabel", "unit_symbol"} {
		val, ok := valueCaseInsensitive(record, key)
		if !ok {
			continue
		}
		if nested, ok := val.(map[string]any); ok {
			if s := unitNameFromRecord(nested); s != "" {
				return s
			}
		}
		if s := strings.ToLower(stringFromAny(val)); s != "" {
			return s
		}
	}
	return ""
}

// valueCaseInsensitive mirrors sqlc helpers: we compare lower-cased keys so we
// can tolerate upstream schema tweaks without expanding every branch.
func valueCaseInsensitive(record map[string]any, key string) (any, bool) {
	if record == nil {
		return nil, false
	}
	want := strings.ToLower(strings.TrimSpace(key))
	for k, v := range record {
		if strings.ToLower(strings.TrimSpace(k)) == want {
			return v, true
		}
	}
	return nil, false
}

// floatFromAny converts arbitrary JSON primitives to float64, keeping behaviour
// consistent with parseFloat for string inputs.
func floatFromAny(v any) float64 {
	switch val := v.(type) {
	case json.Number:
		if f, err := val.Float64(); err == nil {
			return f
		}
		if i, err := val.Int64(); err == nil {
			return float64(i)
		}
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case uint64:
		return float64(val)
	case string:
		return parseFloat(val)
	case []byte:
		return parseFloat(string(val))
	}
	return 0
}

// stringFromAny keeps string extraction centralised so we do not repeat trim
// logic across callers.
func stringFromAny(v any) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case []byte:
		return strings.TrimSpace(string(val))
	case json.Number:
		return strings.TrimSpace(val.String())
	case fmt.Stringer:
		return strings.TrimSpace(val.String())
	}
	return ""
}

// timeFromAny mirrors parseTimeFlexible but also understands JSON numbers and
// nested timestamp containers.
func timeFromAny(v any) time.Time {
	switch val := v.(type) {
	case time.Time:
		if val.IsZero() {
			return time.Time{}
		}
		return val.UTC()
	case string:
		return parseTimeFlexible(val)
	case json.Number:
		if i, err := val.Int64(); err == nil && i > 0 {
			return time.Unix(i, 0).UTC()
		}
		if f, err := val.Float64(); err == nil && f > 0 {
			return time.Unix(int64(f), 0).UTC()
		}
	case float64:
		if val > 0 {
			return time.Unix(int64(val), 0).UTC()
		}
	case float32:
		if val > 0 {
			return time.Unix(int64(val), 0).UTC()
		}
	case int64:
		if val > 0 {
			return time.Unix(val, 0).UTC()
		}
	case int:
		if val > 0 {
			return time.Unix(int64(val), 0).UTC()
		}
	case map[string]any:
		for _, key := range []string{"time", "timestamp", "captured_at"} {
			if nested, ok := valueCaseInsensitive(val, key); ok {
				if ts := timeFromAny(nested); !ts.IsZero() {
					return ts
				}
			}
		}
	}
	return time.Time{}
}

// parseCSVMarkers extracts markers from CSV exports.  We only look at a subset
// of common columns (latitude, longitude, captured_at, cps/cpm/usvh) so the
// function stays robust even when the upstream schema picks up new fields.
func parseCSVMarkers(data []byte) []database.Marker {
	r := csv.NewReader(bytes.NewReader(data))
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		return nil
	}
	if len(header) == 0 {
		return nil
	}

	type colIdx struct {
		lat, lon, cpm, cps, usvh, captured int
	}
	idx := colIdx{lat: -1, lon: -1, cpm: -1, cps: -1, usvh: -1, captured: -1}

	for i, name := range header {
		lower := strings.ToLower(strings.TrimSpace(name))
		switch {
		case strings.Contains(lower, "latitude"):
			idx.lat = i
		case strings.Contains(lower, "longitude"):
			idx.lon = i
		case strings.Contains(lower, "cpm"):
			idx.cpm = i
		case strings.Contains(lower, "cps"):
			idx.cps = i
		case strings.Contains(lower, "usv") || strings.Contains(lower, "sievert"):
			idx.usvh = i
		case strings.Contains(lower, "captured") || strings.Contains(lower, "created_at") || strings.Contains(lower, "time"):
			idx.captured = i
		}
	}

	if idx.lat == -1 || idx.lon == -1 || idx.captured == -1 {
		return nil
	}

	markers := make([]database.Marker, 0, 1024)
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			break
		}
		if len(record) <= idx.lon || len(record) <= idx.lat || len(record) <= idx.captured {
			continue
		}

		lat := parseFloat(record[idx.lat])
		lon := parseFloat(record[idx.lon])
		if lat == 0 && lon == 0 {
			continue
		}

		ts := parseTimeFlexible(record[idx.captured])
		if ts.IsZero() {
			continue
		}

		var dose float64
		if idx.usvh >= 0 && idx.usvh < len(record) {
			dose = parseFloat(record[idx.usvh])
		}
		if dose <= 0 && idx.cpm >= 0 && idx.cpm < len(record) {
			dose = convertCPMToMicroSv(parseFloat(record[idx.cpm]))
		}
		if dose <= 0 && idx.cps >= 0 && idx.cps < len(record) {
			dose = convertCPSToMicroSv(parseFloat(record[idx.cps]))
		}
		if dose <= 0 {
			continue
		}

		countRate := 0.0
		if idx.cps >= 0 && idx.cps < len(record) {
			countRate = parseFloat(record[idx.cps])
		}
		if countRate <= 0 && idx.cpm >= 0 && idx.cpm < len(record) {
			countRate = parseFloat(record[idx.cpm]) / 60.0
		}

		markers = append(markers, database.Marker{
			DoseRate:  dose,
			Date:      ts.Unix(),
			Lon:       lon,
			Lat:       lat,
			CountRate: countRate,
			Zoom:      0,
			Speed:     0,
		})
	}
	return markers
}

// parseLogMarkers consumes raw bGeigie log files and reuses the same dose
// conversion heuristics as the upload handler.
func parseLogMarkers(data []byte) []database.Marker {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	const cpmPerMicroSv = 334.0
	markers := make([]database.Marker, 0, 1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "$BNRDD") {
			continue
		}
		if i := strings.IndexByte(line, '*'); i != -1 {
			line = line[:i]
		}
		parts := strings.Split(line, ",")
		if len(parts) < 11 {
			continue
		}

		ts := parseLogTimestamp(parts)
		if ts == 0 {
			continue
		}

		var lat, lon float64
		if len(parts) >= 11 && strings.Contains(parts[2], "T") {
			lat = parseDMM(parts[7], parts[8], 2)
			lon = parseDMM(parts[9], parts[10], 3)
		} else if len(parts) >= 8 {
			lat = parseBGeigieCoord(parts[6])
			lon = parseBGeigieCoord(parts[7])
		}
		if lat == 0 && lon == 0 {
			continue
		}

		cpm := parseFloat(parts[3])
		cps := parseFloat(parts[4])
		dose := 0.0
		if cpm > 0 {
			dose = cpm / cpmPerMicroSv
		} else if cps > 0 {
			dose = (cps * 60.0) / cpmPerMicroSv
		}
		if dose <= 0 {
			continue
		}

		countRate := cps
		if countRate == 0 && cpm > 0 {
			countRate = cpm / 60.0
		}

		markers = append(markers, database.Marker{
			DoseRate:  dose,
			Date:      ts,
			Lon:       lon,
			Lat:       lat,
			CountRate: countRate,
			Zoom:      0,
			Speed:     0,
		})
	}
	return markers
}

// parseLogTimestamp interprets both ISO8601 and legacy timestamp formats present
// in bGeigie logs.
func parseLogTimestamp(parts []string) int64 {
	if len(parts) >= 3 && strings.Contains(parts[2], "T") {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[2])); err == nil {
			return t.Unix()
		}
	}
	if len(parts) >= 6 {
		if t := parseTimeFlexible(parts[1] + " " + parts[2]); !t.IsZero() {
			return t.Unix()
		}
	}
	return 0
}

// parseTimeFlexible attempts several timestamp layouts commonly used by
// Safecast exports to keep the importer tolerant of format drifts.
func parseTimeFlexible(raw string) time.Time {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.000Z07:00",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, candidate); err == nil {
			return ts
		}
	}
	if unix, err := strconv.ParseInt(candidate, 10, 64); err == nil {
		return time.Unix(unix, 0)
	}
	return time.Time{}
}

// parseFloat trims the string and converts it to float64, returning zero when
// parsing fails.
func parseFloat(raw string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0
	}
	return f
}

// convertCPMToMicroSv and convertCPSToMicroSv mirror the constants used across
// the uploader so ingested tracks remain consistent.
func convertCPMToMicroSv(cpm float64) float64 {
	if cpm <= 0 {
		return 0
	}
	return cpm / 334.0
}

func convertCPSToMicroSv(cps float64) float64 {
	if cps <= 0 {
		return 0
	}
	return (cps * 60.0) / 334.0
}

// parseDMM and parseBGeigieCoord mirror helpers from main so the importer keeps
// GPS decoding identical.  We inline the minimal variants to avoid import cycles.
func parseDMM(val, hemi string, degDigits int) float64 {
	val = strings.TrimSpace(val)
	hemi = strings.TrimSpace(hemi)
	if val == "" {
		return 0
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0
	}
	deg := int(f / 100.0)
	minutes := f - float64(deg*100)
	d := float64(deg) + minutes/60.0
	switch strings.ToUpper(hemi) {
	case "S", "W":
		d = -d
	}
	if degDigits == 2 {
		if d < -90 || d > 90 {
			return 0
		}
	} else {
		if d < -180 || d > 180 {
			return 0
		}
	}
	return d
}

func parseBGeigieCoord(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	r := s[len(s)-1]
	if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
		base := s[:len(s)-1]
		v, err := strconv.ParseFloat(base, 64)
		if err != nil {
			return 0
		}
		switch strings.ToUpper(string(r)) {
		case "S", "W":
			return -v
		default:
			return v
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// filepathExt extracts the suffix without allocating via url.Parse for common
// well-formed URLs.  We still fall back to full parsing for complex cases.
func filepathExt(source string) string {
	if idx := strings.Index(source, "?"); idx >= 0 {
		source = source[:idx]
	}
	if dot := strings.LastIndex(source, "."); dot >= 0 && dot+1 < len(source) {
		return source[dot:]
	}
	if u, err := url.Parse(source); err == nil {
		return path.Ext(u.Path)
	}
	return ""
}

// truncate limits verbose HTTP bodies in logs.  The helper stays tiny so the
// caller can include contextual prefixes without additional allocations.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
