package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
)

// devicePayload maps only fields we care about from the Safecast JSON.
// Keeping it small avoids coupling to the full upstream schema.
type devicePayload struct {
	ID      string  `json:"id"`
	Type    string  `json:"type"` // may describe transport (car, walk)
	Value   float64 `json:"value"`
	Unit    string  `json:"unit"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Time    int64   `json:"time"`
	Country string  `json:"country"` // optional; fall back to bbox lookup
}

// fetch pulls device data once.
// Returning raw payload keeps this function simple and lets callers derive
// both measurements and summaries without another pass.
func fetch(ctx context.Context, url string) ([]devicePayload, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var raw []devicePayload
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// bbox defines a coarse country boundary.
// We keep only a few major regions to avoid heavyweight geo libraries.
type bbox struct {
	code           string
	minLat, maxLat float64
	minLon, maxLon float64
}

var countryBoxes = []bbox{
	{"JP", 30, 46, 129, 146},  // Japan
	{"US", 24, 49, -125, -66}, // Continental USA
	{"RU", 41, 82, 19, 180},   // Rough Russia mainland
	{"EU", 36, 71, -25, 40},   // Generic Europe box
}

// countryFor returns a rough country code for given coordinates.
// Unknown points fall back to "??".
func countryFor(lat, lon float64) string {
	for _, b := range countryBoxes {
		if lat >= b.minLat && lat <= b.maxLat && lon >= b.minLon && lon <= b.maxLon {
			return b.code
		}
	}
	return "??"
}

// Start launches background workers that keep the live table updated.
// We poll every 15 minutes as requested by Safecast to avoid flooding.
// Two goroutines communicate over a channel; no mutex is needed.
// logf defines where progress messages are written.
func Start(ctx context.Context, db *database.Database, dbType string, logf func(string, ...any)) {
	const url = "https://tt.safecast.org/devices"
	const pollInterval = 15 * time.Minute

	if logf == nil {
		logf = log.Printf
	}

	// Announce poller start once so operators know interval and source.
	logf("realtime poller start: url=%s interval=%s", url, pollInterval)

	measurements := make(chan database.RealtimeMeasurement)
	reports := make(chan int)

	// DB writer goroutine.
	// It counts successes and errors per batch and logs once per report.
	go func() {
		var stored, errs int
		var lastErr error
		for {
			select {
			case <-ctx.Done():
				return
			case m := <-measurements:
				if err := db.InsertRealtimeMeasurement(m, dbType); err != nil {
					errs++
					lastErr = err
				} else {
					stored++
				}
			case n := <-reports:
				if errs > 0 {
					logf("realtime poll: devices %d stored %d errors %d last=%v next=%s", n, stored, errs, lastErr, pollInterval)
				} else {
					logf("realtime poll: devices %d stored %d next=%s", n, stored, pollInterval)
				}
				stored, errs, lastErr = 0, 0, nil
			}
		}
	}()

	// Fetcher goroutine runs once immediately and then every pollInterval.
	// Running the first fetch before waiting on the ticker gives operators
	// instant feedback after startup, embodying "Make interfaces easy to
	// use correctly".
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		// prevIDs remembers devices from prior fetch to compute add/remove counts.
		prevIDs := make(map[string]struct{})

		for {
			data, err := fetch(ctx, url)
			if err != nil {
				logf("realtime fetch error: %v", err)
			} else {
				// Log how many devices were returned to understand coverage.
				logf("realtime fetch: devices %d", len(data))

				// Summaries are computed while forwarding measurements to DB.
				stats := make(map[string]struct {
					sum   float64
					count int
				})
				curr := make(map[string]struct{})
				now := time.Now().Unix()

				for _, d := range data {
					curr[d.ID] = struct{}{}
					c := d.Country
					if c == "" {
						c = countryFor(d.Lat, d.Lon)
					}
					s := stats[c]
					s.sum += d.Value
					s.count++
					stats[c] = s

					m := database.RealtimeMeasurement{
						DeviceID:   d.ID,
						Transport:  d.Type,
						Value:      d.Value,
						Unit:       d.Unit,
						Lat:        d.Lat,
						Lon:        d.Lon,
						MeasuredAt: d.Time,
						FetchedAt:  now,
					}
					select {
					case <-ctx.Done():
						close(measurements)
						return
					case measurements <- m:
					}
				}
				reports <- len(data)

				// Calculate device churn since last poll.
				added, removed := 0, 0
				for id := range curr {
					if _, ok := prevIDs[id]; !ok {
						added++
					}
				}
				for id := range prevIDs {
					if _, ok := curr[id]; !ok {
						removed++
					}
				}
				prevIDs = curr

				// Build country summary lines.
				parts := make([]string, 0, len(stats))
				for country, s := range stats {
					avg := s.sum / float64(s.count)
					parts = append(parts, fmt.Sprintf("%s:%d avg=%.2f", country, s.count, avg))
				}
				sort.Strings(parts)

				// Prepare example map link using the first device.
				example := ""
				if len(data) > 0 {
					e := data[0]
					example = fmt.Sprintf("https://www.openstreetmap.org/?mlat=%f&mlon=%f&zoom=12", e.Lat, e.Lon)
				}

				logf("realtime summary: %s added=%d removed=%d example=%s", strings.Join(parts, " "), added, removed, example)

				// Promote stale device histories to normal tracks once a day.
				go db.PromoteStaleRealtime(time.Now().Add(-24*time.Hour).Unix(), dbType)
			}
			select {
			case <-ctx.Done():
				close(measurements)
				return
			case <-ticker.C:
			}
		}
	}()
}
