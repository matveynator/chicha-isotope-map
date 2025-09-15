package realtime

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"chicha-isotope-map/pkg/database"
)

// devicePayload maps only fields we care about from the Safecast JSON.
// Keeping it small avoids coupling to the full upstream schema.
type devicePayload struct {
	ID    string  `json:"id"`
	Type  string  `json:"type"` // may describe transport (car, walk)
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Time  int64   `json:"time"`
}

// fetch pulls device data once and returns normalized measurements.
func fetch(ctx context.Context, url string) ([]database.RealtimeMeasurement, error) {
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
	now := time.Now().Unix()
	out := make([]database.RealtimeMeasurement, 0, len(raw))
	for _, d := range raw {
		out = append(out, database.RealtimeMeasurement{
			DeviceID:   d.ID,
			Transport:  d.Type,
			Value:      d.Value,
			Unit:       d.Unit,
			Lat:        d.Lat,
			Lon:        d.Lon,
			MeasuredAt: d.Time,
			FetchedAt:  now,
		})
	}
	return out, nil
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
		for {
			data, err := fetch(ctx, url)
			if err != nil {
				logf("realtime fetch error: %v", err)
			} else {
				// Log how many devices were returned to understand coverage.
				logf("realtime fetch: devices %d", len(data))
				for _, m := range data {
					select {
					case <-ctx.Done():
						close(measurements)
						return
					case measurements <- m:
					}
				}
				reports <- len(data)
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
