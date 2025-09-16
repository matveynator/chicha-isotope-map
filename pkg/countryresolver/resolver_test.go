package countryresolver

import "testing"

// TestResolveSampleLocations verifies that representative cities resolve
// to the expected ISO codes.  Covering border regions ensures the
// priority ordering in buildBoxes remains correct.
func TestResolveSampleLocations(t *testing.T) {
	tests := []struct {
		lat, lon float64
		want     string
	}{
		{50.4501, 30.5234, "UA"},   // Kyiv, Ukraine
		{41.7151, 44.8271, "GE"},   // Tbilisi, Georgia
		{40.4093, 49.8671, "AZ"},   // Baku, Azerbaijan
		{40.1792, 44.4991, "AM"},   // Yerevan, Armenia
		{35.4759, 139.5716, "JP"},  // Tokyo area, Japan
		{37.4428, -122.1281, "US"}, // California, USA
		{-43.5322, 172.6089, "NZ"}, // Christchurch, New Zealand
		{1.5100, 103.7172, "SG"},   // Singapore
		{5.6016, -0.1029, "GH"},    // Accra, Ghana
		{37.5446, 126.9264, "KR"},  // Seoul, South Korea
		{25.0439, 121.5290, "TW"},  // Taipei, Taiwan
		{22.3181, 114.1577, "HK"},  // Hong Kong
		{-16.5101, -68.1986, "BO"}, // La Paz, Bolivia
	}
	for _, tc := range tests {
		got, _ := Resolve(tc.lat, tc.lon)
		if got != tc.want {
			t.Errorf("Resolve(%f,%f) = %q, want %q", tc.lat, tc.lon, got, tc.want)
		}
	}
}

// TestResolveInvalid ensures invalid coordinates return an empty code so
// callers can treat them as unknown.
func TestResolveInvalid(t *testing.T) {
	if got, _ := Resolve(91, 0); got != "" {
		t.Fatalf("Resolve(91,0) = %q, want empty", got)
	}
	if got, _ := Resolve(0, 181); got != "" {
		t.Fatalf("Resolve(0,181) = %q, want empty", got)
	}
}

func TestNameFor(t *testing.T) {
	if name := NameFor("ua"); name != "Ukraine" {
		t.Fatalf("NameFor(ua) = %q, want Ukraine", name)
	}
}
