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
// provided coordinate.  When no polygon matches we return empty strings so
// callers may fall back to "unknown" labels.
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

// resolvePoint walks the packed R-tree and keeps the smallest containing
// polygon.  Preferring the tiniest area means enclaves and islands win over
// their host countries without manual priority lists.
func resolvePoint(lat, lon float64) (string, string) {
	// We ask the dataset coordinator for the cached polygons so the heavy
	// JSON parsing only happens once per process and never during package
	// init, keeping startup friendly to go test/go vet.
	snapshot := ensureDatasetReady()

	candidateCh := make(chan candidate)
	done := make(chan struct{})

	go func() {
		defer close(candidateCh)
		streamCandidates(snapshot.index, lat, lon, candidateCh, done)
	}()

	defer close(done)

	bestArea := math.MaxFloat64
	bestCode := ""
	bestName := ""

	for {
		select {
		case cand, ok := <-candidateCh:
			if !ok {
				return bestCode, bestName
			}
			country := snapshot.countries[cand.countryIndex]
			poly := country.polygons[cand.polygonIndex]
			if !poly.contains(lat, lon) {
				continue
			}
			if poly.area < bestArea {
				bestArea = poly.area
				bestCode = country.code
				bestName = country.name
			}
		}
	}
}

// NameFor returns the stored English country name for a code.  The lookup
// is forgiving with input case so external callers can pass arbitrary user
// data.
func NameFor(code string) string {
	if code == "" {
		return ""
	}
	// Lazily request the dataset so command-line tools do not pay the
	// parsing cost unless NameFor is actually used during the run.
	snapshot := ensureDatasetReady()
	upper := strings.ToUpper(code)
	if name, ok := snapshot.names[upper]; ok {
		return name
	}
	return ""
}

// buildNameIndex constructs a map from ISO code to English name once during
// package initialisation.  Doing this eagerly avoids locking at runtime,
// keeping Resolve cheap for every caller.
func buildNameIndex(list []country) map[string]string {
	out := make(map[string]string, len(list))
	for _, c := range list {
		if c.code == "" || c.name == "" {
			continue
		}
		if _, ok := out[c.code]; !ok {
			out[c.code] = c.name
		}
	}
	return out
}
