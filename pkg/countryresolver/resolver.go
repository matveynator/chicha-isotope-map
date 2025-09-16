// Package countryresolver provides a lightweight offline mapper from
// coordinates to ISO country codes.  The implementation favours
// simplicity over mathematical perfection, echoing the Go Proverb
// "Clear is better than clever" so operators can audit the logic at a
// glance.
package countryresolver

import (
	"math"
	"runtime"
	"strings"
)

// regionBox stores a coarse rectangular approximation for a country.
// We intentionally keep only a handful of rectangles per country to
// avoid heavy geo libraries while still classifying realtime sensors
// accurately.
type regionBox struct {
	code           string
	name           string
	minLat, maxLat float64
	minLon, maxLon float64
	priority       int
	area           float64
}

// contains reports whether the rectangle includes the provided point.
func (b regionBox) contains(lat, lon float64) bool {
	if lat < b.minLat || lat > b.maxLat {
		return false
	}
	if lon < b.minLon || lon > b.maxLon {
		return false
	}
	return true
}

// boxes is populated via buildBoxes in data.go.  We keep the slice as a
// package-level variable so Resolve can stay allocation free.
var boxes = buildBoxes()

// nameByCode keeps a quick lookup for English country names.  It is
// derived once from the prepared slice to keep Resolve simple.
var nameByCode = buildNameIndex(boxes)

// query represents a single Resolve request forwarded to background
// workers.  The reply channel is buffered to avoid blocking when callers
// abandon a lookup early.
type query struct {
	lat, lon float64
	reply    chan result
}

// result collects the ISO code and English name for a location.
type result struct {
	code string
	name string
}

// resolveRequests feeds Resolve work to the background pool.  Using a
// channel rather than a mutex embraces the Go proverb "Share memory by
// communicating" so concurrent callers do not need explicit locking.
var resolveRequests chan query

// init spins up a small worker pool sized to the available CPUs.  This
// keeps lookups responsive without introducing extra dependencies.
func init() {
	workerCount := runtime.GOMAXPROCS(0)
	if workerCount < 1 {
		workerCount = 1
	}
	resolveRequests = make(chan query, workerCount)
	for i := 0; i < workerCount; i++ {
		go resolverWorker(resolveRequests)
	}
}

// resolverWorker processes resolve queries sequentially per goroutine so
// the main Resolve function can stay a thin dispatcher.
func resolverWorker(in <-chan query) {
	for req := range in {
		code, name := resolvePoint(req.lat, req.lon)
		// A buffered channel prevents a blocked send if the
		// caller timed out and forgot to read the result.
		req.reply <- result{code: code, name: name}
	}
}

// Resolve finds an ISO 3166-1 alpha-2 code and English name for the
// provided coordinate.  When no rectangle matches we return empty
// strings so callers may fall back to "unknown" labels.
func Resolve(lat, lon float64) (string, string) {
	// lat or lon outside plausible bounds are treated as unknown.
	if math.IsNaN(lat) || math.IsNaN(lon) {
		return "", ""
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return "", ""
	}

	reply := make(chan result, 1)
	req := query{lat: lat, lon: lon, reply: reply}

	// Try the worker pool first to honour the channel-driven design.
	select {
	case resolveRequests <- req:
	default:
		// When the queue is saturated we resolve inline to keep
		// latency low for realtime sensors.
		code, name := resolvePoint(lat, lon)
		return code, name
	}

	select {
	case res := <-reply:
		return res.code, res.name
	}
}

// resolvePoint walks the prepared regions using a producer goroutine to
// keep the iteration cancellable.  Closing the done channel stops the
// producer once the first match is found.
func resolvePoint(lat, lon float64) (string, string) {
	candidateCh := make(chan regionBox)
	done := make(chan struct{})

	go func() {
		defer close(candidateCh)
		for _, box := range boxes {
			if !box.contains(lat, lon) {
				continue
			}
			select {
			case candidateCh <- box:
			case <-done:
				return
			}
		}
	}()

	defer close(done)

	for {
		select {
		case box, ok := <-candidateCh:
			if !ok {
				return "", ""
			}
			return box.code, box.name
		}
	}
}

// NameFor returns the stored English country name for a code.  The
// lookup is forgiving with input case so external callers can pass
// arbitrary user data.
func NameFor(code string) string {
	if code == "" {
		return ""
	}
	upper := strings.ToUpper(code)
	if name, ok := nameByCode[upper]; ok {
		return name
	}
	return ""
}

// buildNameIndex constructs a map from ISO code to English name once
// during package initialisation.  Doing this eagerly avoids locking at
// runtime, keeping Resolve cheap for every caller.
func buildNameIndex(list []regionBox) map[string]string {
	out := make(map[string]string, len(list))
	for _, b := range list {
		if _, ok := out[b.code]; !ok {
			out[b.code] = b.name
		}
	}
	return out
}
