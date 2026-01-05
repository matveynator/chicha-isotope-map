package atomfast

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
)

// ---------- Constants and shared helpers ----------

const (
	mapPageTemplate      = "http://www.atomfast.net/maps/?lat=34.307144&lng=20.742188&z=2&p=%d"
	trackPageTemplate    = "http://www.atomfast.net/maps/show/%s/"
	trackMarkersTemplate = "http://www.atomfast.net/maps/markers/%s"
	defaultUserAgent     = "Mozilla/5.0 (compatible; ChichaAtomFast/1.0)"
)

var (
	trackIDPattern  = regexp.MustCompile(`/maps/show/([0-9]+)/`)
	stripTags       = regexp.MustCompile(`<[^>]+>`)
	deviceLineRegex = regexp.MustCompile(`(?i)\b(device|model|detector)\b\s*:\s*([^\r\n]+)`)
)

// TrackID keeps AtomFast identifiers distinct from user uploads while still
// preserving the upstream ID for debugging and display.
func TrackID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return "atomfast:" + raw
}

// DeviceInfo describes the hardware associated with a track.
// We keep ID optional because AtomFast may expose only a model string.
type DeviceInfo struct {
	ID    string
	Model string
}

// TrackPayload bundles markers with the device metadata they belong to.
type TrackPayload struct {
	ID      string
	Markers []database.Marker
	Device  DeviceInfo
}

// Client wraps HTTP access so callers can reuse the same transport and user agent.
// This keeps request handling consistent and avoids sprinkling headers in every call.
type Client struct {
	httpClient *http.Client
	userAgent  string
	jitter     *rand.Rand
}

// markerRecord captures the raw AtomFast JSON shape.
// We keep a named type so slices are compatible across helpers.
type markerRecord struct {
	D   float64 `json:"d"`
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
	T   int64   `json:"t"`
}

// NewClient constructs an AtomFast client with conservative timeouts.
// A dedicated rand source avoids global locking while we add jitter between calls.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 25 * time.Second},
		userAgent:  defaultUserAgent,
		jitter:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// FetchPageTrackIDs loads a paginated AtomFast map page and extracts track IDs.
// We keep the parser regex-based because the markup is lightweight and avoids
// pulling in a full HTML parser.
func (c *Client) FetchPageTrackIDs(ctx context.Context, page int) ([]string, error) {
	if page < 1 {
		page = 1
	}
	url := fmt.Sprintf(mapPageTemplate, page)
	body, err := c.fetch(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch map page %d: %w", page, err)
	}

	matches := trackIDPattern.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{})
	ids := make([]string, 0, len(matches))
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

	return ids, nil
}

// FetchTrackPageDevice extracts device metadata from the track HTML page.
// We scan the visible text for "Device:" or "Model:" labels to stay resilient
// to layout changes without HTML-specific parsing dependencies.
func (c *Client) FetchTrackPageDevice(ctx context.Context, id string) (DeviceInfo, error) {
	url := fmt.Sprintf(trackPageTemplate, id)
	body, err := c.fetch(ctx, url)
	if err != nil {
		return DeviceInfo{}, fmt.Errorf("fetch track page %s: %w", id, err)
	}

	text := stripTags.ReplaceAllString(body, "\n")
	text = htmlUnescape(text)

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if match := deviceLineRegex.FindStringSubmatch(line); len(match) == 3 {
			model := strings.TrimSpace(match[2])
			if model == "" {
				continue
			}
			return DeviceInfo{Model: model}, nil
		}
	}

	return DeviceInfo{}, nil
}

// FetchTrackMarkers retrieves the track JSON payload and parses markers plus device info.
// Device details are optional, so we always return markers even if no device is found.
func (c *Client) FetchTrackMarkers(ctx context.Context, id string) ([]database.Marker, DeviceInfo, error) {
	url := fmt.Sprintf(trackMarkersTemplate, id)
	body, err := c.fetch(ctx, url)
	if err != nil {
		return nil, DeviceInfo{}, fmt.Errorf("fetch track markers %s: %w", id, err)
	}

	markers, device, err := ParseMarkers([]byte(body))
	if err != nil {
		return nil, DeviceInfo{}, fmt.Errorf("parse track markers %s: %w", id, err)
	}
	return markers, device, nil
}

// SleepWithJitter pauses between requests while still honoring context cancellations.
// A jittered delay reduces the chance of appearing like a bot, while select keeps
// us responsive to shutdown requests.
func (c *Client) SleepWithJitter(ctx context.Context, minDelay, maxDelay time.Duration) {
	if maxDelay < minDelay {
		maxDelay = minDelay
	}
	if minDelay <= 0 {
		return
	}
	delta := maxDelay - minDelay
	wait := minDelay
	if delta > 0 {
		wait += time.Duration(c.jitter.Int63n(int64(delta)))
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// ---------- Parsing helpers ----------

// ParseMarkers supports both the raw array format and object-wrapped payloads.
// We accept the object format because AtomFast sometimes attaches metadata.
func ParseMarkers(data []byte) ([]database.Marker, DeviceInfo, error) {
	var rawMarkers []markerRecord
	if err := json.Unmarshal(data, &rawMarkers); err == nil {
		return recordsToMarkers(rawMarkers), DeviceInfo{}, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, DeviceInfo{}, fmt.Errorf("parse AtomFast payload: %w", err)
	}

	markers, err := parseWrappedMarkers(obj)
	if err != nil {
		return nil, DeviceInfo{}, err
	}

	device := parseDeviceFromJSON(obj)
	return markers, device, nil
}

func parseWrappedMarkers(obj map[string]json.RawMessage) ([]database.Marker, error) {
	keys := []string{"markers", "data", "points"}
	for _, key := range keys {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		var records []markerRecord
		if err := json.Unmarshal(raw, &records); err != nil {
			continue
		}
		return recordsToMarkers(records), nil
	}

	return nil, fmt.Errorf("AtomFast payload missing markers")
}

func parseDeviceFromJSON(obj map[string]json.RawMessage) DeviceInfo {
	keys := []string{"device", "device_id", "deviceId", "device_model", "deviceModel", "model", "detector"}
	for _, key := range keys {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		device := parseDeviceRaw(raw)
		if device.ID != "" || device.Model != "" {
			return device
		}
	}
	return DeviceInfo{}
}

func parseDeviceRaw(raw json.RawMessage) DeviceInfo {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		asString = strings.TrimSpace(asString)
		if asString != "" {
			return DeviceInfo{ID: asString, Model: asString}
		}
	}

	var asObj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asObj); err != nil {
		return DeviceInfo{}
	}
	device := DeviceInfo{}
	for key, value := range asObj {
		switch strings.ToLower(key) {
		case "id", "device_id", "deviceid":
			var id string
			if err := json.Unmarshal(value, &id); err == nil {
				device.ID = strings.TrimSpace(id)
			}
		case "model", "device_model", "devicemodel", "name":
			var model string
			if err := json.Unmarshal(value, &model); err == nil {
				device.Model = strings.TrimSpace(model)
			}
		}
	}
	if device.ID == "" && device.Model != "" {
		device.ID = device.Model
	}
	return device
}

func recordsToMarkers(records []markerRecord) []database.Marker {
	markers := make([]database.Marker, 0, len(records))
	for _, r := range records {
		markers = append(markers, database.Marker{
			DoseRate:  r.D,
			CountRate: r.D, // AtomFast stores cps in the same field
			Lat:       r.Lat,
			Lon:       r.Lng,
			Date:      r.T / 1000,
		})
	}
	return markers
}

func (c *Client) fetch(ctx context.Context, url string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func htmlUnescape(input string) string {
	replacer := strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"", "&#39;", "'")
	return replacer.Replace(input)
}
