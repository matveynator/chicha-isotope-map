package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matveynator/chicha-isotope-map/pkg/database"
)

func TestAPIDocsHandlerRejectsHostHeaderMarkup(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/api", nil)
	request.Host = `example.test"><script>alert(1)</script>`
	response := httptest.NewRecorder()

	apiDocsHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if strings.Contains(response.Body.String(), "<script>") {
		t.Fatal("response contains executable Host header markup")
	}
}

func TestAPIDocsHandlerRendersEscapedTemplateModel(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/api", nil)
	response := httptest.NewRecorder()

	apiDocsHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if strings.Contains(body, "__BASE_URL__") || strings.Contains(body, "{{.BaseURL}}") {
		t.Fatal("API documentation contains an unresolved template placeholder")
	}
	if !strings.Contains(body, "http://example.test/api") {
		t.Fatal("API documentation does not contain its resolved API root")
	}
}

func TestValidateShortRedirectTargetRequiresSameOrigin(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://example.test/s/ABC123", nil)

	if target, ok := validateShortRedirectTarget(request, "https://example.test/tracks/1"); !ok || target == "" {
		t.Fatalf("same-origin target rejected: target=%q ok=%v", target, ok)
	}
	for _, target := range []string{
		"https://attacker.test/",
		"javascript:alert(1)",
		"//attacker.test/path",
		"https://user@example.test/path",
	} {
		if validated, ok := validateShortRedirectTarget(request, target); ok {
			t.Fatalf("unsafe target %q accepted as %q", target, validated)
		}
	}
}

func TestRequestOriginRejectsUnsupportedForwardedScheme(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.Header.Set("X-Forwarded-Proto", "javascript")
	if _, err := requestOrigin(request); err == nil {
		t.Fatal("requestOrigin accepted an unsupported forwarded scheme")
	}
}

func TestMapHandlerRendersConfiguredOSMVectorStylesWithoutCARTO(t *testing.T) {
	previousLightStyleURL := *osmVectorLightStyleURL
	previousDarkStyleURL := *osmVectorDarkStyleURL
	previousTranslations := translations
	t.Cleanup(func() {
		*osmVectorLightStyleURL = previousLightStyleURL
		*osmVectorDarkStyleURL = previousDarkStyleURL
		translations = previousTranslations
	})

	*osmVectorLightStyleURL = "https://maps.example/light.json"
	*osmVectorDarkStyleURL = "https://maps.example/dark.json"
	translations = map[string]map[string]string{"en": {}}

	request := httptest.NewRequest("GET", "http://example.test/?theme=dark", nil)
	response := httptest.NewRecorder()
	mapHandler(response, request)

	if response.Code != 200 {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	responseBody := response.Body.String()
	for _, expected := range []string{
		`light: "https://maps.example/light.json"`,
		`dark: "https://maps.example/dark.json"`,
		`new L.OSMBaseLayer`,
		`/static/maplibre-gl.js`,
		`/static/leaflet-maplibre-gl.js`,
	} {
		if !strings.Contains(responseBody, expected) {
			t.Fatalf("rendered map does not contain %q", expected)
		}
	}
	if strings.Contains(responseBody, "cartocdn.com") {
		t.Fatal("rendered map still references CARTO tiles")
	}
}

func TestLicenseHandlerServesMapLibreNotices(t *testing.T) {
	for _, licenseCode := range []string{"maplibre", "maplibre-leaflet"} {
		request := httptest.NewRequest("GET", "http://example.test/licenses/"+licenseCode, nil)
		response := httptest.NewRecorder()
		licenseHandler(response, request)

		if response.Code != 200 {
			t.Fatalf("%s status = %d, want 200", licenseCode, response.Code)
		}
		if !strings.Contains(response.Body.String(), "Permission") {
			t.Fatalf("%s notice does not contain its license grant", licenseCode)
		}
	}
}

func TestFastMergeMarkersByZoomKeepsHighestDoseRepresentative(t *testing.T) {
	markers := []database.Marker{
		{ID: 1, Lat: 10.000000, Lon: 20.000000, DoseRate: 0.08, Date: 100, TrackID: "low"},
		{ID: 2, Lat: 10.000001, Lon: 20.000001, DoseRate: 0.42, Date: 90, TrackID: "high", Detector: "detector-a"},
	}

	merged := fastMergeMarkersByZoom(markers, 10, radiusForZoom(10))
	if len(merged) != 1 {
		t.Fatalf("merged len = %d, want 1", len(merged))
	}
	got := merged[0]
	if got.DoseRate != 0.42 {
		t.Fatalf("dose = %v, want high dose", got.DoseRate)
	}
	if got.TrackID != "high" {
		t.Fatalf("trackID = %q, want high marker track", got.TrackID)
	}
	if got.Detector != "detector-a" {
		t.Fatalf("detector = %q, want representative detector", got.Detector)
	}
	if got.Zoom != 10 {
		t.Fatalf("zoom = %d, want 10", got.Zoom)
	}
}

func TestFastMergeMarkersByZoomTieBreaksByLatestDate(t *testing.T) {
	markers := []database.Marker{
		{ID: 1, Lat: 10.000000, Lon: 20.000000, DoseRate: 0.42, Date: 100, TrackID: "older"},
		{ID: 2, Lat: 10.000001, Lon: 20.000001, DoseRate: 0.42, Date: 200, TrackID: "newer"},
	}

	merged := fastMergeMarkersByZoom(markers, 10, radiusForZoom(10))
	if len(merged) != 1 {
		t.Fatalf("merged len = %d, want 1", len(merged))
	}
	if merged[0].TrackID != "newer" {
		t.Fatalf("trackID = %q, want latest marker", merged[0].TrackID)
	}
}

func collectAggregateMarkers(t *testing.T, markers []database.Marker) []database.Marker {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	base := make(chan database.Marker, len(markers))
	out := aggregateMarkers(ctx, base, nil, 10)
	for _, marker := range markers {
		base <- marker
	}
	close(base)

	var got []database.Marker
	for marker := range out {
		got = append(got, marker)
	}
	return got
}

func TestAggregateMarkersEmitsReplacementWinnersInInputOrder(t *testing.T) {
	got := collectAggregateMarkers(t, []database.Marker{
		{ID: 1, Lat: 10.000000, Lon: 20.000000, DoseRate: 0.08, Date: 100, TrackID: "low"},
		{ID: 2, Lat: 10.000001, Lon: 20.000001, DoseRate: 0.42, Date: 120, TrackID: "high"},
	})

	if len(got) != 2 {
		t.Fatalf("aggregated len = %d, want 2 replacement emissions", len(got))
	}
	if got[0].DoseRate != 0.08 || got[1].DoseRate != 0.42 {
		t.Fatalf("aggregated doses = %v, %v; want input-order low then high", got[0].DoseRate, got[1].DoseRate)
	}
	if got[0].AggregateKey == "" || got[0].AggregateKey != got[1].AggregateKey {
		t.Fatalf("aggregate keys = %q, %q; want stable replacement key", got[0].AggregateKey, got[1].AggregateKey)
	}
}

func TestAggregateMarkersPreservesInputOrderAcrossCells(t *testing.T) {
	got := collectAggregateMarkers(t, []database.Marker{
		{ID: 1, Lat: 20.000000, Lon: 20.000000, DoseRate: 0.20, Date: 100, TrackID: "first"},
		{ID: 2, Lat: 10.000000, Lon: 10.000000, DoseRate: 0.30, Date: 120, TrackID: "second"},
	})

	if len(got) != 2 {
		t.Fatalf("aggregated len = %d, want 2", len(got))
	}
	if got[0].TrackID != "first" || got[1].TrackID != "second" {
		t.Fatalf("aggregated order = %q, %q; want input order", got[0].TrackID, got[1].TrackID)
	}
	if got[0].AggregateKey == "" || got[1].AggregateKey == "" || got[0].AggregateKey == got[1].AggregateKey {
		t.Fatalf("aggregate keys = %q, %q; want distinct stable keys", got[0].AggregateKey, got[1].AggregateKey)
	}
}

func TestMapMarkerStreamZoomUsesRequestedZoom(t *testing.T) {
	if got := mapMarkerStreamZoom(8); got != 8 {
		t.Fatalf("map stream zoom = %d, want requested zoom", got)
	}
}

func TestAggregateMarkersSuppressesWeakerLaterMarkerPerCell(t *testing.T) {
	got := collectAggregateMarkers(t, []database.Marker{
		{ID: 1, Lat: 10.000000, Lon: 20.000000, DoseRate: 0.42, Date: 100, TrackID: "high"},
		{ID: 2, Lat: 10.000001, Lon: 20.000001, DoseRate: 0.08, Date: 120, TrackID: "low"},
	})

	if len(got) != 1 {
		t.Fatalf("aggregated len = %d, want 1", len(got))
	}
	if got[0].DoseRate != 0.42 || got[0].TrackID != "high" || got[0].AggregateKey == "" {
		t.Fatalf("aggregated marker = %+v, want high dose marker with aggregate key", got[0])
	}
}

func TestGenerateSerialNumberReturnsDistinctCompactIdentifiers(t *testing.T) {
	const generatedCount = 128
	identifiers := make(map[string]struct{}, generatedCount)

	for range generatedCount {
		identifier := GenerateSerialNumber()
		if len(identifier) != 10 {
			t.Fatalf("identifier length = %d, want 10", len(identifier))
		}
		if _, exists := identifiers[identifier]; exists {
			t.Fatalf("duplicate identifier %q", identifier)
		}
		identifiers[identifier] = struct{}{}
	}
}
