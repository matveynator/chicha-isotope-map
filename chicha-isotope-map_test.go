package main

import (
	"context"
	"testing"

	"chicha-isotope-map/pkg/database"
)

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
