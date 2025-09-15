package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"chicha-isotope-map/pkg/database"
)

// ttDevice represents a subset of fields from https://tt.safecast.org/devices
// The upstream schema includes various counters; we primarily use lnd_7318u as CPM when available.
type ttDevice struct {
	DeviceURN      string   `json:"device_urn"`
	DeviceClass    string   `json:"device_class"`
	DeviceSN       string   `json:"device_sn,omitempty"`
	Device         int      `json:"device"`
	WhenCaptured   string   `json:"when_captured"`
	LocLat         float64  `json:"loc_lat"`
	LocLon         float64  `json:"loc_lon"`
	LND7318u       *float64 `json:"lnd_7318u,omitempty"`
	ServiceTransport string `json:"service_transport,omitempty"`
	ServiceUploaded  string `json:"service_uploaded,omitempty"`
}

// fetch pulls device data once and returns normalized measurements.
func fetch(ctx context.Context, url string) ([]database.RealtimeMeasurement, error) {
	// Use a client with a timeout so a bad upstream cannot stall the goroutine.
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "chicha-isotope-map/realtime (+https://safecast.org)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read a small snippet to help debugging (avoid huge bodies)
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, errors.New("unexpected status: " + resp.Status + ", body: " + string(b))
	}

	// Optional: check content-type when provided.
	if ct := resp.Header.Get("Content-Type"); ct != "" &&
		!(ct == "application/json" || ct == "application/json; charset=utf-8" || ct == "application/json; charset=UTF-8") {
		// Not fatal, but log a warning — upstream sometimes serves variations.
		log.Printf("realtime fetch: warning: unexpected Content-Type %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var raw []ttDevice
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	out := make([]database.RealtimeMeasurement, 0, len(raw))
	for _, d := range raw {
		// Take CPM if present; skip devices without a usable value.
		var val float64
		var unit string
		if d.LND7318u != nil {
			val = *d.LND7318u
			unit = "cpm"
		} else {
			continue
		}

		// Parse RFC3339 time; if invalid, try service_uploaded; if still invalid, fallback to now.
		var measured int64
		if t, err := time.Parse(time.RFC3339, d.WhenCaptured); err == nil {
			measured = t.Unix()
		} else if d.ServiceUploaded != "" {
			if t2, err2 := time.Parse(time.RFC3339, d.ServiceUploaded); err2 == nil {
				measured = t2.Unix()
			} else {
				measured = now
			}
		} else {
			measured = now
		}

		// Use DeviceURN as the stable ID; fall back to numeric device id.
		devID := d.DeviceURN
		if devID == "" {
			devID = "device:" + strconv.Itoa(d.Device)
		}

		out = append(out, database.RealtimeMeasurement{
			DeviceID:   devID,
			Transport:  d.DeviceClass,
			Value:      val,
			Unit:       unit,
			Lat:        d.LocLat,
			Lon:        d.LocLon,
			MeasuredAt: measured,
			FetchedAt:  now,
		})
	}
	log.Printf("realtime fetch: parsed %d devices", len(out))
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

	log.Printf("realtime: start (interval=%s, url=%s)", pollInterval, url)

	// DB writer goroutine.
	// It counts successes and errors per batch and logs once per report.
	go func() {
		var stored, errs int
		var lastErr error
		for {
			select {
			case <-ctx.Done():
				return
			case m, ok := <-measurements:
				if !ok {
					// channel closed by fetcher — graceful shutdown
					return
				}
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

		// Immediate fetch on startup so we don't wait pollInterval for first data.
		doFetch := func() {
			data, err := fetch(ctx, url)
			if err != nil {
				log.Printf("realtime fetch: %v", err)
				return
			}
			for _, m := range data {
				select {
				case <-ctx.Done():
					close(measurements)
					return
				case measurements <- m:
				}
			}
			// Always report so the writer logs the batch outcome (can be 0 devices).
			reports <- len(data)
		}

		// First run immediately
		doFetch()

		for {
			select {
			case <-ctx.Done():
				close(measurements)
				return
			case <-ticker.C:
				doFetch()
			}
		}
	}()
}
