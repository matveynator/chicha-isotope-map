// Package countryresolver provides a lightweight offline mapper from
// coordinates to ISO country codes.  The implementation favours
// simplicity over mathematical perfection, echoing the Go Proverb
// "Clear is better than clever" so operators can audit the logic at a
// glance.
package countryresolver

import (
	"math"
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

// newBox precomputes the rectangle area so caller code stays tidy.
func newBox(code, name string, minLat, maxLat, minLon, maxLon float64) regionBox {
	area := math.Abs(maxLat-minLat) * math.Abs(maxLon-minLon)
	return regionBox{
		code:   code,
		name:   name,
		minLat: minLat,
		maxLat: maxLat,
		minLon: minLon,
		maxLon: maxLon,
		area:   area,
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
	for _, b := range boxes {
		if b.contains(lat, lon) {
			return b.code, b.name
		}
	}
	return "", ""
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
