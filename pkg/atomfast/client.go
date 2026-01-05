package atomfast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
)

// Config captures AtomFast endpoints and parsing defaults so callers can
// keep the loader configuration in one place without sprinkling magic
// values throughout the code.
type Config struct {
	BaseURL         string
	PageLimit       int
	UserAgent       string
	TrackPageFormat string
	Timeout         time.Duration
}

// DeviceInfo carries the best-effort device identity extracted from AtomFast.
// We keep both ID and Model so callers can build a stable identifier while
// preserving the raw model string for user-facing display.
type DeviceInfo struct {
	ID    string
	Model string
}

// TrackData bundles the parsed markers with any device metadata discovered in
// the AtomFast payloads.
type TrackData struct {
	TrackID string
	Markers []database.Marker
	Device  DeviceInfo
}

// Client fetches AtomFast track lists and marker payloads using only standard
// libraries so it stays portable across the supported Go runtimes.
type Client struct {
	baseURL         string
	pageLimit       int
	userAgent       string
	trackPageFormat string
	client          *http.Client
	rng             *rand.Rand
	userAgents      []string
	platforms       []string
	languages       []string
}

// NewClient builds a Client while applying defensive defaults so the caller
// can pass a zero-value Config when they want the canonical AtomFast endpoints.
func NewClient(cfg Config) *Client {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "http://www.atomfast.net"
	}
	limit := cfg.PageLimit
	if limit <= 0 {
		limit = 20
	}
	agent := strings.TrimSpace(cfg.UserAgent)
	if agent == "" {
		agent = "Mozilla/5.0 (compatible; ChichaIsotopeMap/1.0; +https://github.com/matveynator/chicha-isotope-map)"
	}
	pageFormat := strings.TrimSpace(cfg.TrackPageFormat)
	if pageFormat == "" {
		pageFormat = base + "/maps/show/%s/"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return &Client{
		baseURL:         base,
		pageLimit:       limit,
		userAgent:       agent,
		trackPageFormat: pageFormat,
		client: &http.Client{
			Timeout: timeout,
		},
		rng:        rng,
		userAgents: defaultUserAgents(),
		platforms:  defaultPlatforms(),
		languages:  defaultLanguages(),
	}
}

// FetchRecentIDs pulls one page of recent AtomFast track IDs.
// The payload format is not documented publicly, so we parse generically
// and search for any list of objects containing an identifier field.
func (c *Client) FetchRecentIDs(ctx context.Context, page int) ([]string, error) {
	if page <= 0 {
		page = 1
	}
	endpoint := fmt.Sprintf("%s/maps/recent_list/?p=%d&lim=%d", c.baseURL, page, c.pageLimit)
	body, err := c.fetchBody(ctx, endpoint, true)
	if err != nil {
		return nil, err
	}
	doc, err := decodeJSON(body)
	if err == nil {
		if ids := extractTrackIDs(doc); len(ids) > 0 {
			return ids, nil
		}
	}
	ids := extractTrackIDsFromText(string(body))
	return ids, nil
}

// FetchTrack downloads the AtomFast marker JSON for a given track ID and
// enriches it with any device metadata found in JSON or HTML payloads.
func (c *Client) FetchTrack(ctx context.Context, trackID string) (TrackData, error) {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return TrackData{}, errors.New("atomfast track id empty")
	}
	endpoint := fmt.Sprintf("%s/maps/markers/%s", c.baseURL, trackID)
	doc, err := c.fetchJSON(ctx, endpoint)
	if err != nil {
		return TrackData{}, err
	}
	markers, err := extractMarkers(doc)
	if err != nil {
		return TrackData{}, err
	}
	device := extractDeviceInfo(doc)
	if device.Model == "" && device.ID == "" {
		if pageDevice, pageErr := c.fetchDeviceFromTrackPage(ctx, trackID); pageErr == nil {
			device = pageDevice
		}
	}
	return TrackData{TrackID: trackID, Markers: markers, Device: device}, nil
}

// fetchJSON performs a JSON request with a friendly user-agent header so the
// AtomFast endpoints treat the loader as a normal browser session.
func (c *Client) fetchJSON(ctx context.Context, endpoint string) (any, error) {
	body, err := c.fetchBody(ctx, endpoint, true)
	if err != nil {
		return nil, err
	}
	doc, err := decodeJSON(body)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// fetchDeviceFromTrackPage retrieves the HTML page for a track and scrapes
// device hints using lightweight regex parsing to avoid extra dependencies.
func (c *Client) fetchDeviceFromTrackPage(ctx context.Context, trackID string) (DeviceInfo, error) {
	if c.trackPageFormat == "" {
		return DeviceInfo{}, errors.New("track page format not configured")
	}
	endpoint := fmt.Sprintf(c.trackPageFormat, trackID)
	body, err := c.fetchBody(ctx, endpoint, false)
	if err != nil {
		return DeviceInfo{}, err
	}
	return extractDeviceInfoFromHTML(string(body)), nil
}

// fetchBody loads a URL payload while emulating a browser session, keeping
// AtomFast's anti-bot filters satisfied without external dependencies.
func (c *Client) fetchBody(ctx context.Context, endpoint string, wantJSON bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if wantJSON {
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
	} else {
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	}
	if c.baseURL != "" {
		req.Header.Set("Referer", c.baseURL+"/maps/")
	}
	c.applyRandomHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("atomfast http %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// applyRandomHeaders rotates a pool of browser-like headers so successive
// requests avoid identical fingerprints without relying on external deps.
func (c *Client) applyRandomHeaders(req *http.Request) {
	if req == nil {
		return
	}
	userAgent := c.userAgent
	if len(c.userAgents) > 0 {
		userAgent = c.userAgents[c.rng.Intn(len(c.userAgents))]
	}
	platform := ""
	if len(c.platforms) > 0 {
		platform = c.platforms[c.rng.Intn(len(c.platforms))]
	}
	lang := ""
	if len(c.languages) > 0 {
		lang = c.languages[c.rng.Intn(len(c.languages))]
	}

	req.Header.Set("User-Agent", userAgent)
	if platform != "" {
		req.Header.Set("Sec-CH-UA-Platform", fmt.Sprintf("%q", platform))
		req.Header.Set("Sec-CH-UA-Mobile", "?0")
	}
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("X-Request-Id", randomToken(c.rng, 12))
}

// randomToken returns a short random string so every request carries a unique
// header value even when the browser signature repeats.
func randomToken(rng *rand.Rand, length int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	if length <= 0 {
		length = 8
	}
	out := make([]byte, length)
	for i := range out {
		out[i] = letters[rng.Intn(len(letters))]
	}
	return string(out)
}

// defaultUserAgents keeps a small, curated pool of desktop user agents so
// loader requests look like real browsers without pretending to be exotic.
func defaultUserAgents() []string {
	return []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 11.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 13_6) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_2) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.3 Safari/605.1.15",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Windows NT 11.0; Win64; x64; rv:122.0) Gecko/20100101 Firefox/122.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14.2; rv:121.0) Gecko/20100101 Firefox/121.0",
	}
}

// defaultPlatforms lists operating system tokens aligned with the UA pool.
func defaultPlatforms() []string {
	return []string{
		"Windows",
		"Windows",
		"Windows",
		"macOS",
		"macOS",
		"Linux",
		"Linux",
		"Windows",
		"Windows",
		"macOS",
	}
}

// defaultLanguages rotates common Accept-Language values to add variability
// without triggering locale-specific content changes in AtomFast.
func defaultLanguages() []string {
	return []string{
		"en-US,en;q=0.9",
		"en-GB,en;q=0.9",
		"en-US,en;q=0.8,ru;q=0.6",
		"en-US,en;q=0.8,uk;q=0.5",
		"en-US,en;q=0.8,de;q=0.5",
		"en-US,en;q=0.8,fr;q=0.5",
		"en-US,en;q=0.8,es;q=0.5",
		"en-US,en;q=0.8,it;q=0.5",
		"en-US,en;q=0.8,pl;q=0.5",
		"en-US,en;q=0.8,tr;q=0.5",
	}
}

// -----------------------------
// JSON parsing helpers
// -----------------------------

func extractTrackIDs(doc any) []string {
	var ids []string
	seen := make(map[string]struct{})
	for _, id := range findTrackIDs(doc) {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func findTrackIDs(doc any) []string {
	switch val := doc.(type) {
	case []any:
		return idsFromSlice(val)
	case map[string]any:
		for _, key := range []string{"data", "list", "tracks", "items", "results"} {
			if raw, ok := val[key]; ok {
				if ids := findTrackIDs(raw); len(ids) > 0 {
					return ids
				}
			}
		}
		return idsFromMap(val)
	default:
		return nil
	}
}

func idsFromSlice(items []any) []string {
	ids := make([]string, 0, len(items))
	for _, entry := range items {
		switch typed := entry.(type) {
		case map[string]any:
			if id := parseTrackID(typed); id != "" {
				ids = append(ids, id)
			}
		case string:
			ids = append(ids, strings.TrimSpace(typed))
		case float64:
			ids = append(ids, strconv.FormatInt(int64(typed), 10))
		}
	}
	return ids
}

func idsFromMap(m map[string]any) []string {
	if id := parseTrackID(m); id != "" {
		return []string{id}
	}
	return nil
}

func parseTrackID(m map[string]any) string {
	for _, key := range []string{"id", "track_id", "trackId", "trackID", "uid"} {
		if raw, ok := m[key]; ok {
			if id := stringify(raw); id != "" {
				return id
			}
		}
	}
	return ""
}

func decodeJSON(body []byte) (any, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, errors.New("atomfast empty payload")
	}
	var doc any
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func extractMarkers(doc any) ([]database.Marker, error) {
	entries := findMarkerEntries(doc, 0)
	if len(entries) == 0 {
		return nil, errors.New("atomfast: no markers found")
	}
	markers := make([]database.Marker, 0, len(entries))
	for _, entry := range entries {
		marker, ok := parseMarker(entry)
		if !ok {
			continue
		}
		markers = append(markers, marker)
	}
	if len(markers) == 0 {
		return nil, errors.New("atomfast: markers parsed empty")
	}
	return markers, nil
}

func findMarkerEntries(doc any, depth int) []map[string]any {
	if depth > 4 {
		return nil
	}
	switch val := doc.(type) {
	case []any:
		entries := collectMarkerEntries(val)
		if len(entries) > 0 {
			return entries
		}
		for _, entry := range val {
			if nested := findMarkerEntries(entry, depth+1); len(nested) > 0 {
				return nested
			}
		}
	case map[string]any:
		for _, key := range []string{"markers", "points", "data", "records", "track"} {
			if raw, ok := val[key]; ok {
				if nested := findMarkerEntries(raw, depth+1); len(nested) > 0 {
					return nested
				}
			}
		}
		for _, raw := range val {
			if nested := findMarkerEntries(raw, depth+1); len(nested) > 0 {
				return nested
			}
		}
	}
	return nil
}

func collectMarkerEntries(items []any) []map[string]any {
	entries := make([]map[string]any, 0, len(items))
	for _, entry := range items {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if hasCoordinates(m) {
			entries = append(entries, m)
		}
	}
	return entries
}

func hasCoordinates(m map[string]any) bool {
	_, latOk := readFloat(m, "lat", "latitude")
	_, lonOk := readFloat(m, "lon", "lng", "longitude")
	return latOk && lonOk
}

func parseMarker(m map[string]any) (database.Marker, bool) {
	lat, latOk := readFloat(m, "lat", "latitude")
	lon, lonOk := readFloat(m, "lon", "lng", "longitude")
	if !latOk || !lonOk {
		return database.Marker{}, false
	}
	timestamp := readTimestamp(m)
	if timestamp == 0 {
		return database.Marker{}, false
	}
	dose, _ := readFloat(m, "doseRate", "dose_rate", "dose", "sv", "usvh", "svh")
	countRate, _ := readFloat(m, "countRate", "count_rate", "cps", "cpm")
	speed, _ := readFloat(m, "speed", "speed_ms", "speedMS")
	altitude, altitudeOk := readFloat(m, "altitude", "alt", "height")
	temperature, temperatureOk := readFloat(m, "temperature", "temp", "temp_c")
	humidity, humidityOk := readFloat(m, "humidity", "hum", "humidity_percent")
	detector := readString(m, "detector", "sensor", "device")
	tube := readString(m, "tube")
	radiation := readString(m, "radiation", "channels")

	marker := database.Marker{
		DoseRate:         dose,
		Date:             timestamp,
		Lon:              lon,
		Lat:              lat,
		CountRate:        countRate,
		Speed:            speed,
		Altitude:         altitude,
		Detector:         strings.TrimSpace(detector),
		Radiation:        strings.TrimSpace(radiation),
		AltitudeValid:    altitudeOk,
		Zoom:             0,
		Temperature:      temperature,
		Humidity:         humidity,
		Tube:             strings.TrimSpace(tube),
		TemperatureValid: temperatureOk,
		HumidityValid:    humidityOk,
	}
	return marker, true
}

func readTimestamp(m map[string]any) int64 {
	if raw, ok := readInt64(m, "time", "timestamp", "date", "datetime", "ts"); ok {
		return normalizeTimestamp(raw)
	}
	if text := readString(m, "time", "timestamp", "date", "datetime", "ts"); text != "" {
		if ts, ok := parseTimeString(text); ok {
			return ts
		}
	}
	return 0
}

func normalizeTimestamp(ts int64) int64 {
	if ts > 1_000_000_000_000 {
		return ts / 1000
	}
	return ts
}

func parseTimeString(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		return normalizeTimestamp(n), true
	}
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	for _, layout := range formats {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts.Unix(), true
		}
	}
	return 0, false
}

func readFloat(m map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if raw, ok := m[key]; ok {
			switch v := raw.(type) {
			case float64:
				if !math.IsNaN(v) {
					return v, true
				}
			case int:
				return float64(v), true
			case int64:
				return float64(v), true
			case json.Number:
				if f, err := v.Float64(); err == nil {
					return f, true
				}
			case string:
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					return f, true
				}
			}
		}
	}
	return 0, false
}

func readInt64(m map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		if raw, ok := m[key]; ok {
			switch v := raw.(type) {
			case float64:
				return int64(v), true
			case int:
				return int64(v), true
			case int64:
				return v, true
			case json.Number:
				if n, err := v.Int64(); err == nil {
					return n, true
				}
			case string:
				if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
					return n, true
				}
			}
		}
	}
	return 0, false
}

func readString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := m[key]; ok {
			if s := stringify(raw); s != "" {
				return s
			}
		}
	}
	return ""
}

func stringify(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func extractDeviceInfo(doc any) DeviceInfo {
	info := DeviceInfo{}
	if m, ok := doc.(map[string]any); ok {
		info = deviceFromMap(m)
		if info.ID != "" || info.Model != "" {
			return info
		}
		return searchDeviceInfo(m, 0)
	}
	return info
}

func deviceFromMap(m map[string]any) DeviceInfo {
	model := readString(m, "device", "device_name", "deviceName", "device_model", "deviceModel", "model")
	id := readString(m, "device_id", "deviceId", "id", "serial", "sn")
	if model == "" {
		if nested, ok := m["device"].(map[string]any); ok {
			model = readString(nested, "model", "name", "type")
			if id == "" {
				id = readString(nested, "id", "device_id", "serial", "sn")
			}
		}
	}
	return DeviceInfo{ID: strings.TrimSpace(id), Model: strings.TrimSpace(model)}
}

func searchDeviceInfo(node any, depth int) DeviceInfo {
	if depth > 4 {
		return DeviceInfo{}
	}
	switch val := node.(type) {
	case map[string]any:
		if info := deviceFromMap(val); info.ID != "" || info.Model != "" {
			return info
		}
		for _, raw := range val {
			if info := searchDeviceInfo(raw, depth+1); info.ID != "" || info.Model != "" {
				return info
			}
		}
	case []any:
		for _, raw := range val {
			if info := searchDeviceInfo(raw, depth+1); info.ID != "" || info.Model != "" {
				return info
			}
		}
	}
	return DeviceInfo{}
}

func extractDeviceInfoFromHTML(html string) DeviceInfo {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:device|detector|model|device model|модель|устройство)[^:]*:\s*([^<\n]+)`),
		regexp.MustCompile(`(?i)(?:device|detector|model|модель|устройство)\s*</[^>]+>\s*([^<\n]+)`),
	}
	for _, re := range patterns {
		if match := re.FindStringSubmatch(html); len(match) > 1 {
			model := strings.TrimSpace(match[1])
			if model != "" {
				return DeviceInfo{Model: model}
			}
		}
	}
	return DeviceInfo{}
}

func extractTrackIDsFromText(text string) []string {
	ids := make([]string, 0, 32)
	seen := make(map[string]struct{})
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\"id\"\\s*:\\s*\"?(\\d+)\"?`),
		regexp.MustCompile(`data-id\\s*=\\s*\"(\\d+)\"`),
		regexp.MustCompile(`track(?:_|\\s)?id\\s*[:=]\\s*\"?(\\d+)\"?`),
	}
	for _, re := range patterns {
		matches := re.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			id := strings.TrimSpace(match[1])
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}
