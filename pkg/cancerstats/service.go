// Package cancerstats streams Global Cancer Observatory statistics so the
// map can render country overlays without blocking request handlers.
package cancerstats

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Logger mirrors log.Printf so callers can inject their own logging without
// importing a concrete logger into this package.
type Logger func(format string, args ...any)

const (
	defaultRefreshInterval = 24 * time.Hour
	defaultHTTPTimeout     = 20 * time.Second
	defaultSourceURL       = "https://gco.iarc.fr/138a2c5a-1b8a-4583-8ec6-91642ad6d28b"
	defaultSourceLabel     = "Global Cancer Observatory (IARC)"
	maxPayloadBytes        = 5 * 1024 * 1024
)

// Config captures fetcher tunables so the caller can point to a different GCO
// endpoint or refresh cadence without editing code.
type Config struct {
	SourceURL       string
	RefreshInterval time.Duration
	HTTPTimeout     time.Duration
}

// CountryStat is a per-country entry for the overlay layer.
type CountryStat struct {
	Country string  `json:"country"`
	Deaths  float64 `json:"deaths"`
	Year    int     `json:"year"`
}

// Snapshot is the API payload consumed by the map layer.
type Snapshot struct {
	Countries  map[string]CountryStat `json:"countries"`
	MinDeaths  float64                `json:"minDeaths"`
	MaxDeaths  float64                `json:"maxDeaths"`
	LatestYear int                    `json:"latestYear"`
	UpdatedAt  time.Time              `json:"updatedAt"`
	Source     string                 `json:"source"`
	SourceURL  string                 `json:"sourceUrl"`
}

// Service owns the current dataset and answers snapshot requests over channels
// to avoid shared-memory synchronization.
type Service struct {
	requests chan snapshotRequest
	stop     chan struct{}
}

type snapshotRequest struct {
	resp chan snapshotResponse
}

type snapshotResponse struct {
	snapshot Snapshot
	err      error
}

type fetchResult struct {
	snapshot Snapshot
	err      error
}

// DefaultSourceURL exposes the baked-in GCO endpoint for UI attribution.
func DefaultSourceURL() string {
	return defaultSourceURL
}

// Start spawns the fetch loop and returns a service that answers snapshot
// requests without exposing internal state.
func Start(ctx context.Context, cfg Config, logf Logger) *Service {
	resolved := applyDefaults(cfg)
	client := &http.Client{Timeout: resolved.HTTPTimeout}
	updates := make(chan fetchResult)
	requests := make(chan snapshotRequest)
	stop := make(chan struct{})

	if logf == nil {
		logf = func(string, ...any) {}
	}

	service := &Service{requests: requests, stop: stop}
	go service.run(ctx, updates)
	go fetchLoop(ctx, client, resolved, updates, logf)
	return service
}

// Snapshot returns the latest cached dataset or an error if none is available.
func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	if s == nil {
		return Snapshot{}, errors.New("cancerstats: service not initialized")
	}
	resp := make(chan snapshotResponse, 1)
	select {
	case s.requests <- snapshotRequest{resp: resp}:
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	case <-s.stop:
		return Snapshot{}, errors.New("cancerstats: service stopped")
	}
	select {
	case reply := <-resp:
		return reply.snapshot, reply.err
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	case <-s.stop:
		return Snapshot{}, errors.New("cancerstats: service stopped")
	}
}

// Stop closes the service to signal background goroutines to exit.
func (s *Service) Stop() {
	if s == nil {
		return
	}
	select {
	case <-s.stop:
		return
	default:
		close(s.stop)
	}
}

// ===== Internal wiring =====

func (s *Service) run(ctx context.Context, updates <-chan fetchResult) {
	var snapshot Snapshot
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			close(s.stop)
			return
		case <-s.stop:
			return
		case req := <-s.requests:
			req.resp <- snapshotResponse{snapshot: snapshot, err: lastErr}
		case res := <-updates:
			if res.err != nil {
				lastErr = res.err
				continue
			}
			snapshot = res.snapshot
			lastErr = nil
		}
	}
}

func applyDefaults(cfg Config) Config {
	if strings.TrimSpace(cfg.SourceURL) == "" {
		cfg.SourceURL = defaultSourceURL
	}
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = defaultRefreshInterval
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = defaultHTTPTimeout
	}
	return cfg
}

func fetchLoop(ctx context.Context, client *http.Client, cfg Config, updates chan<- fetchResult, logf Logger) {
	fetchOnce := func() {
		snapshot, err := fetchSnapshot(ctx, client, cfg.SourceURL, 0)
		if err != nil {
			logf("cancerstats: fetch failed: %v", err)
		}
		select {
		case updates <- fetchResult{snapshot: snapshot, err: err}:
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
			logf("cancerstats: update channel blocked; dropping snapshot")
		}
	}

	fetchOnce()
	ticker := time.NewTicker(cfg.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fetchOnce()
		}
	}
}

// ===== Payload parsing =====

type whoRecord struct {
	SpatialDim   string  `json:"SpatialDim"`
	TimeDim      int     `json:"TimeDim"`
	NumericValue float64 `json:"NumericValue"`
	Value        string  `json:"Value"`
}

func fetchSnapshot(ctx context.Context, client *http.Client, sourceURL string, depth int) (Snapshot, error) {
	if depth > 2 {
		return Snapshot{}, fmt.Errorf("api discovery exceeded depth for %s", sourceURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return Snapshot{}, fmt.Errorf("source %s status %d: %s", sourceURL, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxPayloadBytes))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read payload from %s: %w", sourceURL, err)
	}

	if looksLikeHTML(resp.Header.Get("Content-Type"), payload) {
		apiURL := discoverGCOAPIURL(sourceURL, payload)
		if apiURL == "" {
			return Snapshot{}, fmt.Errorf("source %s returned HTML without API link", sourceURL)
		}
		return fetchSnapshot(ctx, client, apiURL, depth+1)
	}

	snapshot := Snapshot{
		Countries: make(map[string]CountryStat),
		UpdatedAt: time.Now().UTC(),
		Source:    defaultSourceLabel,
		SourceURL: sourceURL,
	}

	if err := decodePayload(payload, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode payload from %s: %w", sourceURL, err)
	}

	min := math.Inf(1)
	max := math.Inf(-1)
	latestYear := 0
	for _, stat := range snapshot.Countries {
		if stat.Deaths < min {
			min = stat.Deaths
		}
		if stat.Deaths > max {
			max = stat.Deaths
		}
		if stat.Year > latestYear {
			latestYear = stat.Year
		}
	}

	if len(snapshot.Countries) == 0 {
		min = 0
		max = 0
	}
	if min == math.Inf(1) {
		min = 0
	}
	if max == math.Inf(-1) {
		max = 0
	}

	snapshot.MinDeaths = min
	snapshot.MaxDeaths = max
	snapshot.LatestYear = latestYear

	return snapshot, nil
}

func looksLikeHTML(contentType string, payload []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return true
	}
	trimmed := bytes.TrimSpace(payload)
	return len(trimmed) > 0 && trimmed[0] == '<'
}

func discoverGCOAPIURL(sourceURL string, payload []byte) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`https://gco\\.iarc\\.fr/api[^\"'\\s]+`),
		regexp.MustCompile(`/api[^\"'\\s]+`),
	}
	text := string(payload)
	for _, pattern := range patterns {
		if match := pattern.FindString(text); match != "" {
			if strings.HasPrefix(match, "http") {
				return match
			}
			base, err := url.Parse(sourceURL)
			if err != nil {
				return strings.TrimRight(strings.TrimSpace(sourceURL), "/") + match
			}
			rel, err := url.Parse(match)
			if err != nil {
				return strings.TrimRight(strings.TrimSpace(sourceURL), "/") + match
			}
			return base.ResolveReference(rel).String()
		}
	}
	return ""
}

func decodePayload(payload []byte, snapshot *Snapshot) error {
	if err := decodeWHO(bytes.NewReader(payload), snapshot); err == nil {
		return nil
	}
	if err := decodeGCO(payload, snapshot); err == nil {
		return nil
	}
	return errors.New("unknown payload format")
}

func decodeWHO(reader io.Reader, snapshot *Snapshot) error {
	decoder := json.NewDecoder(reader)

	tok, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode token: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim.String() != "{" {
		return errors.New("unexpected JSON payload")
	}

	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode key: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("unexpected JSON key")
		}
		if key != "value" {
			var discard json.RawMessage
			if err := decoder.Decode(&discard); err != nil {
				return fmt.Errorf("discard %s: %w", key, err)
			}
			continue
		}
		if tok, err := decoder.Token(); err != nil || tok.(json.Delim).String() != "[" {
			return errors.New("expected value array")
		}

		for decoder.More() {
			var rec whoRecord
			if err := decoder.Decode(&rec); err != nil {
				return fmt.Errorf("decode record: %w", err)
			}
			code := strings.TrimSpace(rec.SpatialDim)
			if code == "" || len(code) < 3 {
				continue
			}
			value := rec.NumericValue
			if value == 0 && rec.Value != "" && rec.Value != "0" {
				parsed, err := strconv.ParseFloat(strings.TrimSpace(rec.Value), 64)
				if err == nil {
					value = parsed
				}
			}
			if value <= 0 {
				continue
			}
			existing, ok := snapshot.Countries[code]
			if !ok || rec.TimeDim > existing.Year {
				snapshot.Countries[code] = CountryStat{
					Country: code,
					Deaths:  value,
					Year:    rec.TimeDim,
				}
			}
		}

		if tok, err := decoder.Token(); err != nil || tok.(json.Delim).String() != "]" {
			return errors.New("value array not closed")
		}
	}

	if tok, err := decoder.Token(); err != nil || tok.(json.Delim).String() != "}" {
		return errors.New("payload not closed")
	}

	return nil
}

func decodeGCO(payload []byte, snapshot *Snapshot) error {
	var root any
	if err := json.Unmarshal(payload, &root); err != nil {
		return err
	}
	records := findFirstArray(root)
	if len(records) == 0 {
		return errors.New("gco payload missing data array")
	}
	for _, item := range records {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		code := findStringField(record, []string{
			"iso3", "iso_a3", "iso3_code", "country_code", "country_iso3", "countryIso3", "country",
		})
		code = strings.ToUpper(strings.TrimSpace(code))
		if len(code) < 3 {
			continue
		}
		value := findFloatField(record, []string{
			"deaths", "value", "mortality", "count", "number", "cases",
		})
		if value <= 0 {
			continue
		}
		year := int(findFloatField(record, []string{"year", "time", "date"}))
		existing, ok := snapshot.Countries[code]
		if !ok || year > existing.Year {
			snapshot.Countries[code] = CountryStat{
				Country: code,
				Deaths:  value,
				Year:    year,
			}
		}
	}
	if len(snapshot.Countries) == 0 {
		return errors.New("gco payload contained no country records")
	}
	return nil
}

func findFirstArray(root any) []any {
	switch data := root.(type) {
	case []any:
		return data
	case map[string]any:
		keys := []string{"data", "value", "values", "records", "items", "dataset", "result"}
		for _, key := range keys {
			if arr, ok := data[key].([]any); ok {
				return arr
			}
		}
		for _, value := range data {
			if arr := findFirstArray(value); len(arr) > 0 {
				return arr
			}
		}
	}
	return nil
}

func findStringField(record map[string]any, keys []string) string {
	for _, key := range keys {
		if value, ok := record[key]; ok {
			switch typed := value.(type) {
			case string:
				return typed
			case fmt.Stringer:
				return typed.String()
			}
		}
	}
	return ""
}

func findFloatField(record map[string]any, keys []string) float64 {
	for _, key := range keys {
		if value, ok := record[key]; ok {
			switch typed := value.(type) {
			case float64:
				return typed
			case float32:
				return float64(typed)
			case int:
				return float64(typed)
			case int64:
				return float64(typed)
			case json.Number:
				if parsed, err := typed.Float64(); err == nil {
					return parsed
				}
			case string:
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
					return parsed
				}
			}
		}
	}
	return 0
}
