// Package cancerstats streams WHO-derived cancer mortality statistics so the
// map can render country overlays without blocking request handlers.
package cancerstats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
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
	defaultSourceURL       = "https://ghoapi.azureedge.net/api/GHECAUSES?$filter=Dim1%20eq%20'SEX_BTSX'%20and%20Dim2Value%20eq%20'0-49%20years'%20and%20Dim3Value%20eq%20'Malignant%20neoplasms'"
	defaultSourceLabel     = "WHO Global Health Observatory"
)

// Config captures fetcher tunables so the caller can point to a different WHO
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

// DefaultSourceURL exposes the baked-in WHO endpoint for UI attribution.
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
		snapshot, err := fetchSnapshot(ctx, client, cfg.SourceURL)
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

// ===== WHO payload parsing =====

type whoRecord struct {
	SpatialDim   string  `json:"SpatialDim"`
	TimeDim      int     `json:"TimeDim"`
	NumericValue float64 `json:"NumericValue"`
	Value        string  `json:"Value"`
}

func fetchSnapshot(ctx context.Context, client *http.Client, sourceURL string) (Snapshot, error) {
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
		return Snapshot{}, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	snapshot := Snapshot{
		Countries: make(map[string]CountryStat),
		UpdatedAt: time.Now().UTC(),
		Source:    defaultSourceLabel,
		SourceURL: sourceURL,
	}

	if err := decodeWHO(resp.Body, &snapshot); err != nil {
		return Snapshot{}, err
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
